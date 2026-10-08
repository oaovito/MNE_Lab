package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CloudKitAPI is the CloudKit Web Services base URL (overridable in tests).
var CloudKitAPI = "https://api.apple-cloudkit.com"

// CloudKitCallbackPort is the fixed loopback port registered as the
// "Sign In Callback" URL of the CloudKit API token
// (http://127.0.0.1:47613/).
var CloudKitCallbackPort = 47613

// cloudKitChunk keeps each record well under CloudKit's 1 MB record limit.
const cloudKitChunk = 700 * 1024

// ICloudAPI stores files as records of type MNEFile in the private
// database of the MNE Lab CloudKit container, which Apple scopes to the
// signed-in user and to this application. It uses CloudKit Web Services,
// Apple's official cross-platform API; the Apple Account password is
// entered only on Apple's own sign-in page.
//
// Container schema required (CloudKit Dashboard): record type MNEFile with
// fields path (String, queryable), parent (String, queryable), data
// (Bytes), part (Int64), parts (Int64), size (Int64).
type ICloudAPI struct {
	cfg  Config
	mu   sync.Mutex
	web  string // ckWebAuthToken, rotated on every round trip
	info AccountInfo
}

func (c *ICloudAPI) base() string {
	return fmt.Sprintf("%s/database/1/%s/%s/private", CloudKitAPI, url.PathEscape(c.cfg.CloudKitContainer), url.PathEscape(c.cfg.CloudKitEnv))
}

// ConnectICloud runs the CloudKit sign-in: the first request returns a
// redirect URL to Apple's sign-in page; after sign-in Apple calls the
// loopback callback with a web auth token.
func ConnectICloud(ctx context.Context, c Config, open Opener) (*ICloudAPI, error) {
	if c.CloudKitContainer == "" || c.CloudKitAPIToken == "" {
		return nil, ErrNotConfigured
	}
	ic := &ICloudAPI{cfg: c}
	u := ic.base() + "/users/caller?ckAPIToken=" + url.QueryEscape(c.CloudKitAPIToken)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, ErrOffline
	}
	var r struct {
		Code     string `json:"serverErrorCode"`
		Redirect string `json:"redirectURL"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r)
	resp.Body.Close()
	if r.Redirect == "" {
		return nil, ErrUnavailable
	}
	tok, err := awaitCloudKitToken(ctx, r.Redirect, open)
	if err != nil {
		return nil, err
	}
	ic.web = tok
	if err := ic.CheckConnection(ctx); err != nil {
		return nil, err
	}
	return ic, nil
}

func awaitCloudKitToken(ctx context.Context, signIn string, open Opener) (string, error) {
	type res struct {
		tok string
		err error
	}
	done := make(chan res, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Addr: fmt.Sprintf("127.0.0.1:%d", CloudKitCallbackPort),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			tok := r.URL.Query().Get("ckWebAuthToken")
			if tok == "" {
				fmt.Fprintf(w, callbackPage, "#fca5a5", errMark, "Not connected", "Sign-in was not completed. Return to MNE Lab to try again.")
				select {
				case done <- res{err: ErrCanceled}:
				default:
				}
				return
			}
			fmt.Fprintf(w, callbackPage, "#5eead4", okMark, "Connected", "You can close this tab and return to MNE Lab.")
			select {
			case done <- res{tok: tok}:
			default:
			}
		})}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	defer srv.Close()
	if err := open(signIn); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case <-ctx.Done():
		return "", ErrCanceled
	case err := <-errc:
		return "", err
	case r := <-done:
		return r.tok, r.err
	}
}

// RestoreICloud rebuilds a connection from stored credentials.
func RestoreICloud(c Config, creds []byte) (*ICloudAPI, error) {
	var s struct {
		Web  string      `json:"web"`
		Info AccountInfo `json:"info"`
	}
	if err := json.Unmarshal(creds, &s); err != nil {
		return nil, err
	}
	return &ICloudAPI{cfg: c, web: s.Web, info: s.Info}, nil
}

func (c *ICloudAPI) ID() string        { return ICloud }
func (c *ICloudAPI) Transport() string { return TransportAPI }
func (c *ICloudAPI) Info() AccountInfo { return c.info }

func (c *ICloudAPI) Credentials() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return json.Marshal(map[string]any{"transport": TransportAPI, "web": c.web, "info": c.info})
}

// call performs one authenticated round trip and rotates the web auth
// token, which CloudKit invalidates after each use.
func (c *ICloudAPI) call(ctx context.Context, op string, body any) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.web == "" {
		return nil, ErrUnauthorized
	}
	b, _ := json.Marshal(body)
	u := c.base() + "/" + op + "?ckAPIToken=" + url.QueryEscape(c.cfg.CloudKitAPIToken) + "&ckWebAuthToken=" + url.QueryEscape(c.web)
	method := "POST"
	if body == nil {
		method = "GET"
		b = nil
	}
	req, _ := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		if IsNetworkError(err) {
			return nil, ErrOffline
		}
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	var rot struct {
		Web string `json:"ckWebAuthToken"`
	}
	if json.Unmarshal(rb, &rot) == nil && rot.Web != "" {
		c.web = rot.Web
	} else if h := resp.Header.Get("X-Apple-CloudKit-Web-Auth-Token"); h != "" {
		c.web = h
	}
	switch {
	case resp.StatusCode == 421 || resp.StatusCode == 401:
		c.web = ""
		return nil, ErrUnauthorized
	case resp.StatusCode >= 300:
		return nil, apiError(resp.StatusCode, string(rb))
	}
	return rb, nil
}

func (c *ICloudAPI) CheckConnection(ctx context.Context) error {
	_, err := c.call(ctx, "users/caller", nil)
	return err
}

func recordName(path string, part int) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s-%d", hex.EncodeToString(h[:16]), part)
}

type ckField struct {
	Value any    `json:"value"`
	Type  string `json:"type,omitempty"`
}

type ckRecord struct {
	RecordName string             `json:"recordName"`
	RecordType string             `json:"recordType,omitempty"`
	Fields     map[string]ckField `json:"fields,omitempty"`
	ChangeTag  string             `json:"recordChangeTag,omitempty"`
	Modified   *struct {
		Timestamp int64 `json:"timestamp"`
	} `json:"modified,omitempty"`
	ServerErrorCode string `json:"serverErrorCode,omitempty"`
}

func dirOf(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[:i]
	}
	return ""
}

func (c *ICloudAPI) List(ctx context.Context, dir string) ([]Object, error) {
	dir = strings.Trim(dir, "/")
	var out []Object
	marker := ""
	for {
		q := map[string]any{
			"query": map[string]any{"recordType": "MNEFile", "filterBy": []map[string]any{
				{"fieldName": "parent", "comparator": "EQUALS", "fieldValue": map[string]any{"value": dir, "type": "STRING"}},
				{"fieldName": "part", "comparator": "EQUALS", "fieldValue": map[string]any{"value": 0, "type": "INT64"}},
			}},
			"desiredKeys":  []string{"path", "size"},
			"resultsLimit": 200,
		}
		if marker != "" {
			q["continuationMarker"] = marker
		}
		b, err := c.call(ctx, "records/query", q)
		if err != nil {
			return nil, err
		}
		var r struct {
			Records []ckRecord `json:"records"`
			Marker  string     `json:"continuationMarker"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		for _, rec := range r.Records {
			p, _ := rec.Fields["path"].Value.(string)
			size, _ := rec.Fields["size"].Value.(float64)
			o := Object{Name: p, Size: int64(size), Revision: rec.ChangeTag}
			if rec.Modified != nil {
				o.Modified = time.UnixMilli(rec.Modified.Timestamp).UTC()
			}
			out = append(out, o)
		}
		if r.Marker == "" {
			return out, nil
		}
		marker = r.Marker
	}
}

func (c *ICloudAPI) lookup(ctx context.Context, names []string) ([]ckRecord, error) {
	var recs []map[string]string
	for _, n := range names {
		recs = append(recs, map[string]string{"recordName": n})
	}
	b, err := c.call(ctx, "records/lookup", map[string]any{"records": recs})
	if err != nil {
		return nil, err
	}
	var r struct {
		Records []ckRecord `json:"records"`
	}
	return r.Records, json.Unmarshal(b, &r)
}

func (c *ICloudAPI) Read(ctx context.Context, name string) ([]byte, error) {
	first, err := c.lookup(ctx, []string{recordName(name, 0)})
	if err != nil {
		return nil, err
	}
	if len(first) == 0 || first[0].ServerErrorCode != "" {
		return nil, ErrNotFound
	}
	parts := 1
	if v, ok := first[0].Fields["parts"].Value.(float64); ok && v > 1 {
		parts = int(v)
	}
	recs := first
	if parts > 1 {
		var names []string
		for i := 1; i < parts; i++ {
			names = append(names, recordName(name, i))
		}
		rest, err := c.lookup(ctx, names)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rest...)
	}
	var out bytes.Buffer
	for _, rec := range recs {
		if rec.ServerErrorCode != "" {
			return nil, ErrUnavailable
		}
		s, _ := rec.Fields["data"].Value.(string)
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, err
		}
		out.Write(b)
	}
	return out.Bytes(), nil
}

func (c *ICloudAPI) Write(ctx context.Context, name string, data []byte) (Object, error) {
	parts := (len(data) + cloudKitChunk - 1) / cloudKitChunk
	if parts == 0 {
		parts = 1
	}
	var ops []map[string]any
	for i := 0; i < parts; i++ {
		chunk := data[i*cloudKitChunk : min((i+1)*cloudKitChunk, len(data))]
		ops = append(ops, map[string]any{"operationType": "forceReplace", "record": ckRecord{
			RecordName: recordName(name, i), RecordType: "MNEFile",
			Fields: map[string]ckField{
				"path":   {Value: name, Type: "STRING"},
				"parent": {Value: dirOf(name), Type: "STRING"},
				"part":   {Value: i, Type: "INT64"},
				"parts":  {Value: parts, Type: "INT64"},
				"size":   {Value: len(data), Type: "INT64"},
				"data":   {Value: base64.StdEncoding.EncodeToString(chunk), Type: "BYTES"},
			}}})
	}
	b, err := c.call(ctx, "records/modify", map[string]any{"operations": ops, "atomic": true})
	if err != nil {
		return Object{}, err
	}
	var r struct {
		Records []ckRecord `json:"records"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Object{}, err
	}
	for _, rec := range r.Records {
		if rec.ServerErrorCode != "" {
			if strings.Contains(rec.ServerErrorCode, "QUOTA") {
				return Object{}, ErrQuota
			}
			return Object{}, ErrUnavailable
		}
	}
	tag := ""
	if len(r.Records) > 0 {
		tag = r.Records[0].ChangeTag
	}
	return Object{Name: name, Size: int64(len(data)), Revision: tag}, nil
}

func (c *ICloudAPI) Delete(ctx context.Context, name string) error {
	first, err := c.lookup(ctx, []string{recordName(name, 0)})
	if err != nil {
		return err
	}
	if len(first) == 0 || first[0].ServerErrorCode != "" {
		return nil
	}
	parts := 1
	if v, ok := first[0].Fields["parts"].Value.(float64); ok && v > 1 {
		parts = int(v)
	}
	var ops []map[string]any
	for i := 0; i < parts; i++ {
		ops = append(ops, map[string]any{"operationType": "forceDelete", "record": map[string]string{"recordName": recordName(name, i)}})
	}
	_, err = c.call(ctx, "records/modify", map[string]any{"operations": ops})
	return err
}

// Disconnect forgets the web auth token. CloudKit tokens expire on their
// own; the user can also sign out of the app at appleid.apple.com.
func (c *ICloudAPI) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	c.web = ""
	c.mu.Unlock()
	return nil
}
