package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCloudKit keeps MNEFile records and rotates the web auth token on
// every call, as CloudKit Web Services does.
type fakeCloudKit struct {
	mu      sync.Mutex
	token   int
	records map[string]map[string]ckField
	calls   []string
}

func (f *fakeCloudKit) current() string { return fmt.Sprintf("web-%d", f.token) }

func (f *fakeCloudKit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op := r.URL.Path[strings.Index(r.URL.Path, "/private/")+len("/private/"):]
	f.calls = append(f.calls, op)
	if r.URL.Query().Get("ckAPIToken") != "api-token" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.Query().Get("ckWebAuthToken") != f.current() {
		w.WriteHeader(421)
		json.NewEncoder(w).Encode(map[string]string{"serverErrorCode": "AUTHENTICATION_REQUIRED", "redirectURL": "https://sign-in.example/"})
		return
	}
	f.token++
	out := map[string]any{"ckWebAuthToken": f.current()}
	var body struct {
		Records    []ckRecord `json:"records"`
		Operations []struct {
			Type   string   `json:"operationType"`
			Record ckRecord `json:"record"`
		} `json:"operations"`
		Query struct {
			Filters []struct {
				Field string  `json:"fieldName"`
				Value ckField `json:"fieldValue"`
			} `json:"filterBy"`
		} `json:"query"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	switch op {
	case "users/caller":
		out["userRecordName"] = "_user"
	case "records/modify":
		var recs []ckRecord
		for _, o := range body.Operations {
			if o.Type == "forceDelete" {
				delete(f.records, o.Record.RecordName)
			} else {
				f.records[o.Record.RecordName] = o.Record.Fields
			}
			recs = append(recs, ckRecord{RecordName: o.Record.RecordName, ChangeTag: "t"})
		}
		out["records"] = recs
	case "records/lookup":
		var recs []ckRecord
		for _, q := range body.Records {
			if fields, ok := f.records[q.RecordName]; ok {
				recs = append(recs, ckRecord{RecordName: q.RecordName, Fields: fields})
			} else {
				recs = append(recs, ckRecord{RecordName: q.RecordName, ServerErrorCode: "NOT_FOUND"})
			}
		}
		out["records"] = recs
	case "records/query":
		parent := body.Query.Filters[0].Value.Value
		var recs []ckRecord
		for name, fields := range f.records {
			if fields["parent"].Value == parent && fmt.Sprint(fields["part"].Value) == "0" {
				recs = append(recs, ckRecord{RecordName: name, Fields: map[string]ckField{"path": fields["path"], "size": fields["size"]}})
			}
		}
		out["records"] = recs
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(out)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func withCloudKit(t *testing.T) (*fakeCloudKit, Config) {
	t.Helper()
	f := &fakeCloudKit{records: map[string]map[string]ckField{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api, port, client := CloudKitAPI, CloudKitCallbackPort, HTTPClient
	CloudKitAPI, CloudKitCallbackPort, HTTPClient = srv.URL, freePort(t), srv.Client()
	t.Cleanup(func() { CloudKitAPI, CloudKitCallbackPort, HTTPClient = api, port, client })
	return f, Config{CloudKitContainer: "iCloud.test", CloudKitAPIToken: "api-token", CloudKitEnv: "development"}
}

// callback plays the browser after Apple's sign-in: stray requests first,
// then the redirect carrying the token.
func callback(t *testing.T, f *fakeCloudKit, stray bool) Opener {
	return func(signIn string) error {
		if signIn != "https://sign-in.example/" {
			t.Errorf("sign-in page %q", signIn)
		}
		go func() {
			base := fmt.Sprintf("http://127.0.0.1:%d", CloudKitCallbackPort)
			if stray {
				for _, u := range []string{base + "/favicon.ico", base + "/other?ckWebAuthToken=forged"} {
					for i := 0; i < 50; i++ {
						if resp, err := http.Get(u); err == nil {
							resp.Body.Close()
							break
						}
						time.Sleep(20 * time.Millisecond)
					}
				}
				if resp, err := http.Post(base+"/?ckWebAuthToken=forged", "text/plain", nil); err == nil {
					resp.Body.Close()
				}
			}
			f.mu.Lock()
			tok := f.current()
			f.mu.Unlock()
			for i := 0; i < 50; i++ { // until the listener is up
				if resp, err := http.Get(base + "/?ckWebAuthToken=" + tok); err == nil {
					resp.Body.Close()
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
		return nil
	}
}

func TestICloudSignInAndFiles(t *testing.T) {
	f, cfg := withCloudKit(t)
	ctx := context.Background()
	c, err := ConnectICloud(ctx, cfg, callback(t, f, true))
	if err != nil {
		t.Fatal(err)
	}
	small := []byte("record version")
	large := bytes.Repeat([]byte("0123456789"), cloudKitChunk/5) // two parts
	for name, data := range map[string][]byte{"accounts/a/r/x.rec": small, "accounts/a/b/y.blob": large} {
		if o, err := c.Write(ctx, name, data); err != nil || o.Size != int64(len(data)) {
			t.Fatalf("write %s: %+v %v", name, o, err)
		}
		if got, err := c.Read(ctx, name); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("read %s: %d bytes, %v", name, len(got), err)
		}
	}
	if objs, err := c.List(ctx, "accounts/a/r"); err != nil || len(objs) != 1 || objs[0].Name != "accounts/a/r/x.rec" || objs[0].Size != int64(len(small)) {
		t.Fatalf("list: %+v %v", objs, err)
	}
	// A part left by another version is not joined silently.
	f.mu.Lock()
	f.records[recordName("accounts/a/b/y.blob", 1)]["path"] = ckField{Value: "accounts/a/b/other.blob", Type: "STRING"}
	f.mu.Unlock()
	if _, err := c.Read(ctx, "accounts/a/b/y.blob"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("foreign part accepted: %v", err)
	}
	if err := c.Delete(ctx, "accounts/a/r/x.rec"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(ctx, "accounts/a/r/x.rec"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted file read: %v", err)
	}
	// The rotated token is what the credentials keep; a restored
	// connection continues with it.
	creds, err := c.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	r, err := RestoreICloud(cfg, creds)
	if err != nil || r.CheckConnection(ctx) != nil {
		t.Fatalf("restored connection: %v", err)
	}
	// A token CloudKit no longer accepts asks for sign-in again.
	if err := c.CheckConnection(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale token not reported as signed out")
	}
}

func TestICloudSignInIgnoresStrayRequestsAndNeedsConfig(t *testing.T) {
	f, cfg := withCloudKit(t)
	if _, err := ConnectICloud(context.Background(), Config{}, callback(t, f, false)); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing container: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Only stray requests arrive: the sign-in stays open until cancelled.
	open := func(string) error {
		go func() {
			base := fmt.Sprintf("http://127.0.0.1:%d", CloudKitCallbackPort)
			for _, u := range []string{base + "/favicon.ico", base + "/x?ckWebAuthToken=forged"} {
				for i := 0; i < 50; i++ {
					if resp, err := http.Get(u); err == nil {
						resp.Body.Close()
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			cancel()
		}()
		return nil
	}
	if _, err := ConnectICloud(ctx, cfg, open); !errors.Is(err, ErrCanceled) {
		t.Fatalf("stray request completed or failed the sign-in: %v", err)
	}
}
