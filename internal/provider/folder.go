package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oaovito/mne_lab/internal/atomicfile"
)

// Folder is the desktop-app folder transport.
type Folder struct {
	id     string
	client string // the desktop app's folder
	root   string // <client folder>/<MNE Lab subfolder>
}

// NewFolder returns a folder transport rooted at the client folder.
func NewFolder(id, clientFolder string) *Folder {
	return &Folder{id: id, client: clientFolder, root: filepath.Join(clientFolder, appSubfolder(id))}
}

func appSubfolder(id string) string {
	if id == OneDrive {
		return filepath.Join("Apps", "MNE Lab")
	}
	return "MNE Lab"
}

func (f *Folder) ID() string        { return f.id }
func (f *Folder) Transport() string { return TransportFolder }
func (f *Folder) Info() AccountInfo { return AccountInfo{} }

// Root returns the storage folder.
func (f *Folder) Root() string { return f.root }

func (f *Folder) path(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if name == "" {
		return f.root, nil
	}
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("provider: invalid name %q", name)
	}
	return filepath.Join(f.root, clean), nil
}

// CheckConnection verifies the folder exists and is writable with a
// write/read/delete probe.
func (f *Folder) CheckConnection(ctx context.Context) error {
	if _, err := os.Stat(f.client); err != nil {
		return ErrUnavailable
	}
	if err := os.MkdirAll(f.root, 0o700); err != nil {
		return ErrUnavailable
	}
	probe := filepath.Join(f.root, ".mnelab-probe")
	want := []byte("probe")
	if err := os.WriteFile(probe, want, 0o600); err != nil {
		return ErrUnavailable
	}
	got, err := os.ReadFile(probe)
	os.Remove(probe)
	if err != nil || string(got) != string(want) {
		return ErrUnavailable
	}
	return nil
}

func (f *Folder) List(ctx context.Context, dir string) ([]Object, error) {
	p, err := f.path(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	var out []Object
	prefix := strings.TrimSuffix(dir, "/")
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".prev") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		if prefix != "" {
			name = prefix + "/" + name
		}
		out = append(out, Object{Name: name, Size: info.Size(), Modified: info.ModTime().UTC(),
			Revision: fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())})
	}
	return out, nil
}

func (f *Folder) Read(ctx context.Context, name string) ([]byte, error) {
	p, err := f.path(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	return b, nil
}

func (f *Folder) Write(ctx context.Context, name string, data []byte) (Object, error) {
	p, err := f.path(name)
	if err != nil {
		return Object{}, err
	}
	if err := atomicfile.WriteFile(p, data, 0o600); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no space") {
			return Object{}, ErrQuota
		}
		return Object{}, ErrUnavailable
	}
	os.Remove(p + ".prev") // the cloud keeps history; avoid syncing duplicates
	sum := sha256.Sum256(data)
	return Object{Name: name, Size: int64(len(data)), Revision: hex.EncodeToString(sum[:8])}, nil
}

func (f *Folder) Delete(ctx context.Context, name string) error {
	p, err := f.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrUnavailable
	}
	return nil
}

func (f *Folder) Disconnect(ctx context.Context) error { return nil }

func (f *Folder) Credentials() ([]byte, error) {
	return json.Marshal(map[string]string{"transport": TransportFolder, "folder": f.client})
}

// DetectFolders finds the folders of installed official desktop clients.
// Only folder locations are examined; no file outside MNE Lab's own
// subfolder is ever read.
func DetectFolders() map[string]string {
	out := map[string]string{}
	home, _ := os.UserHomeDir()
	exists := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.IsDir()
	}
	// Microsoft OneDrive
	for _, env := range []string{"OneDriveConsumer", "OneDrive", "OneDriveCommercial"} {
		if v := os.Getenv(env); v != "" && exists(v) {
			out[OneDrive] = v
			break
		}
	}
	if _, ok := out[OneDrive]; !ok && home != "" && exists(filepath.Join(home, "OneDrive")) {
		out[OneDrive] = filepath.Join(home, "OneDrive")
	}
	switch runtime.GOOS {
	case "windows":
		if home != "" && exists(filepath.Join(home, "iCloudDrive")) {
			out[ICloud] = filepath.Join(home, "iCloudDrive")
		}
		for c := 'D'; c <= 'Z'; c++ {
			p := string(c) + `:\My Drive`
			if exists(p) {
				out[Google] = p
				break
			}
		}
		if _, ok := out[Google]; !ok && home != "" && exists(filepath.Join(home, "Google Drive")) {
			out[Google] = filepath.Join(home, "Google Drive")
		}
	case "darwin":
		if home != "" {
			if p := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs"); exists(p) {
				out[ICloud] = p
			}
			if m, _ := filepath.Glob(filepath.Join(home, "Library", "CloudStorage", "GoogleDrive-*", "My Drive")); len(m) > 0 {
				out[Google] = m[0]
			}
			if m, _ := filepath.Glob(filepath.Join(home, "Library", "CloudStorage", "OneDrive-*")); len(m) > 0 {
				out[OneDrive] = m[0]
			}
		}
	}
	return out
}
