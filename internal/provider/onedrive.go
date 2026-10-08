package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Microsoft endpoints (overridable in tests). "consumers" covers personal
// Microsoft accounts with OneDrive; "common" would also allow work accounts.
var (
	MicrosoftAuthURL  = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	MicrosoftTokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	GraphAPI          = "https://graph.microsoft.com/v1.0"
)

// OneDriveScopes: the app folder only (Apps/MNE Lab), a refresh token, and
// the basic profile to show which account is connected.
var OneDriveScopes = []string{"Files.ReadWrite.AppFolder", "User.Read", "offline_access"}

// OneDriveAPI stores files in the OneDrive app folder through Microsoft
// Graph (special folder "approot").
type OneDriveAPI struct {
	ts   *tokenSource
	info AccountInfo
}

func msOAuth(c Config) OAuthConfig {
	return OAuthConfig{AuthURL: MicrosoftAuthURL, TokenURL: MicrosoftTokenURL, ClientID: c.MicrosoftClientID,
		Scopes: OneDriveScopes, RedirectHost: "localhost"}
}

// ConnectOneDrive signs the user in through the Microsoft identity platform.
func ConnectOneDrive(ctx context.Context, c Config, open Opener) (*OneDriveAPI, error) {
	cfg := msOAuth(c)
	tok, err := Authorize(ctx, cfg, open)
	if err != nil {
		return nil, err
	}
	o := &OneDriveAPI{ts: &tokenSource{cfg: cfg, tok: tok}}
	return o, o.loadInfo(ctx)
}

// RestoreOneDrive rebuilds a connection from stored credentials.
func RestoreOneDrive(c Config, creds []byte) (*OneDriveAPI, error) {
	var s struct {
		Token Token       `json:"token"`
		Info  AccountInfo `json:"info"`
	}
	if err := json.Unmarshal(creds, &s); err != nil {
		return nil, err
	}
	return &OneDriveAPI{ts: &tokenSource{cfg: msOAuth(c), tok: s.Token}, info: s.Info}, nil
}

func (o *OneDriveAPI) ID() string        { return OneDrive }
func (o *OneDriveAPI) Transport() string { return TransportAPI }
func (o *OneDriveAPI) Info() AccountInfo { return o.info }

func (o *OneDriveAPI) Credentials() ([]byte, error) {
	return json.Marshal(map[string]any{"transport": TransportAPI, "token": o.ts.snapshot(), "info": o.info})
}

func itemPath(name string) string {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (o *OneDriveAPI) do(ctx context.Context, method, u string, body []byte, ctype string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := o.ts.token(ctx)
		if attempt == 1 {
			tok, err = o.ts.forceRefresh(ctx)
		}
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, _ := http.NewRequestWithContext(ctx, method, u, rd)
		req.Header.Set("Authorization", "Bearer "+tok)
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		resp, err := HTTPClient.Do(req)
		if err != nil {
			if IsNetworkError(err) {
				return nil, ErrOffline
			}
			return nil, err
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if resp.StatusCode == 401 && attempt == 0 {
			continue
		}
		if resp.StatusCode >= 300 {
			return nil, apiError(resp.StatusCode, string(b))
		}
		return b, rerr
	}
	return nil, ErrUnauthorized
}

func (o *OneDriveAPI) loadInfo(ctx context.Context) error {
	b, err := o.do(ctx, "GET", GraphAPI+"/me?$select=displayName,mail,userPrincipalName", nil, "")
	if err != nil {
		return err
	}
	var me struct {
		DisplayName string `json:"displayName"`
		Mail        string `json:"mail"`
		UPN         string `json:"userPrincipalName"`
	}
	json.Unmarshal(b, &me)
	email := me.Mail
	if email == "" {
		email = me.UPN
	}
	o.info = AccountInfo{DisplayName: me.DisplayName, MaskedEmail: MaskEmail(email)}
	return nil
}

func (o *OneDriveAPI) CheckConnection(ctx context.Context) error {
	_, err := o.do(ctx, "GET", GraphAPI+"/me/drive/special/approot?$select=id", nil, "")
	return err
}

func (o *OneDriveAPI) List(ctx context.Context, dir string) ([]Object, error) {
	u := GraphAPI + "/me/drive/special/approot/children"
	if d := strings.Trim(dir, "/"); d != "" {
		u = GraphAPI + "/me/drive/special/approot:/" + itemPath(d) + ":/children"
	}
	u += "?$select=name,size,eTag,lastModifiedDateTime,file&$top=999"
	prefix := strings.Trim(dir, "/")
	var out []Object
	for u != "" {
		b, err := o.do(ctx, "GET", u, nil, "")
		if errors.Is(err, ErrNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var r struct {
			Next  string `json:"@odata.nextLink"`
			Value []struct {
				Name     string    `json:"name"`
				Size     int64     `json:"size"`
				ETag     string    `json:"eTag"`
				Modified time.Time `json:"lastModifiedDateTime"`
				File     *struct{} `json:"file"`
			} `json:"value"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		for _, it := range r.Value {
			if it.File == nil {
				continue
			}
			name := it.Name
			if prefix != "" {
				name = prefix + "/" + it.Name
			}
			out = append(out, Object{Name: name, Size: it.Size, Revision: it.ETag, Modified: it.Modified})
		}
		u = r.Next
	}
	return out, nil
}

func (o *OneDriveAPI) Read(ctx context.Context, name string) ([]byte, error) {
	return o.do(ctx, "GET", GraphAPI+"/me/drive/special/approot:/"+itemPath(name)+":/content", nil, "")
}

func (o *OneDriveAPI) Write(ctx context.Context, name string, data []byte) (Object, error) {
	b, err := o.do(ctx, "PUT", GraphAPI+"/me/drive/special/approot:/"+itemPath(name)+":/content", data, "application/octet-stream")
	if err != nil {
		return Object{}, err
	}
	var it struct {
		Size     int64     `json:"size"`
		ETag     string    `json:"eTag"`
		Modified time.Time `json:"lastModifiedDateTime"`
	}
	json.Unmarshal(b, &it)
	return Object{Name: name, Size: it.Size, Revision: it.ETag, Modified: it.Modified}, nil
}

func (o *OneDriveAPI) Delete(ctx context.Context, name string) error {
	_, err := o.do(ctx, "DELETE", GraphAPI+"/me/drive/special/approot:/"+itemPath(name), nil, "")
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Disconnect forgets the tokens. Microsoft offers no public revocation
// endpoint for these tokens; the interface tells the user where to remove
// the app's access in their Microsoft account.
func (o *OneDriveAPI) Disconnect(ctx context.Context) error {
	o.ts.clear()
	return nil
}
