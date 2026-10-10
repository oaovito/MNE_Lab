// Command mnelab-release prepares signed release manifests.
//
//	mnelab-release keygen -out signing.key       (prints the public key)
//	mnelab-release manifest -version 1.2.0 -base-url URL [-prev old.json] -out mnelab-releases.json dist/*
//	mnelab-release sign -key-env MNELAB_SIGNING_KEY mnelab-releases.json
//	mnelab-release verify -pub KEY mnelab-releases.json
//
// The private key never belongs in the repository: keep it offline or in
// the CI secret store.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "manifest":
		err = manifest(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mnelab-release keygen|manifest|sign|verify ...")
	os.Exit(2)
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "file for the private key (base64 seed)")
	fs.Parse(args)
	if *out == "" {
		return errors.New("-out is required")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	// Exclusive creation checks the path atomically, including dangling
	// symlinks. Stat followed by WriteFile could overwrite a concurrent key
	// or follow an existing link into an unintended destination.
	file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite an existing key: %w", err)
		}
		return err
	}
	complete := false
	defer func() {
		if !complete {
			file.Close()
			os.Remove(*out)
		}
	}()
	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func loadKey(env, file string) (ed25519.PrivateKey, error) {
	var s string
	switch {
	case env != "":
		s = os.Getenv(env)
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("signing key missing or invalid")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	env := fs.String("key-env", "", "environment variable holding the key")
	file := fs.String("key", "", "file holding the key")
	fs.Parse(args)
	key, err := loadKey(*env, *file)
	if err != nil {
		return err
	}
	for _, p := range fs.Args() {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if _, err := update.Parse(raw); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if err := os.WriteFile(p+".sig", []byte(update.Sign(key, raw)+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pub := fs.String("pub", "", "public key(s), comma-separated")
	fs.Parse(args)
	update.PublicKeys = *pub
	for _, p := range fs.Args() {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(p + ".sig")
		if err != nil {
			return err
		}
		m, err := update.Verify(raw, sig)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		fmt.Printf("%s: valid, %d releases\n", p, len(m.Releases))
	}
	return nil
}

var assetName = regexp.MustCompile(`^(?:mnelab|MNE-Lab)-([0-9][^-]*(?:-[a-z]+\.[0-9]+)?)-(windows|linux|darwin)-(amd64|386|arm64)(?:-(update|portable))?\.(zip|exe|tar\.gz)$`)

func manifest(args []string) error {
	fs := flag.NewFlagSet("manifest", flag.ExitOnError)
	version := fs.String("version", "", "release version")
	build := fs.Int("build", 0, "build number")
	channel := fs.String("channel", "stable", "channel")
	schema := fs.Int("schema", 1, "data schema written by this build")
	minSchema := fs.Int("min-schema", 1, "oldest data schema it migrates from")
	baseURL := fs.String("base-url", "", "download URL prefix of the assets")
	prev := fs.String("prev", "", "previous manifest to extend")
	out := fs.String("out", "mnelab-releases.json", "output file")
	notesEN := fs.String("notes-en", "", "release notes (English)")
	notesPT := fs.String("notes-pt", "", "release notes (Portuguese)")
	notesES := fs.String("notes-es", "", "release notes (Spanish)")
	security := fs.Bool("security", false, "security release")
	fs.Parse(args)
	if *version == "" || !strings.HasPrefix(*baseURL, "https://") {
		return errors.New("-version and an https -base-url are required")
	}
	m := update.Manifest{Format: update.ManifestFormat}
	if *prev != "" {
		raw, err := os.ReadFile(*prev)
		if err != nil {
			return err
		}
		if m, err = update.Parse(raw); err != nil {
			return err
		}
	}
	r := update.Release{Version: *version, Build: *build, Channel: *channel, Date: time.Now().UTC(), Schema: *schema, MinSchema: *minSchema, Security: *security,
		Notes: map[string]string{}}
	for k, v := range map[string]string{"en": *notesEN, "pt-BR": *notesPT, "es": *notesES} {
		if v != "" {
			r.Notes[k] = v
		}
	}
	for _, p := range fs.Args() {
		name := filepath.Base(p)
		g := assetName.FindStringSubmatch(name)
		if g == nil || g[1] != *version {
			continue
		}
		kind := g[4]
		if kind == "" {
			kind = "temporary"
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		r.Assets = append(r.Assets, update.Asset{OS: g[2], Arch: g[3], Kind: kind, URL: strings.TrimRight(*baseURL, "/") + "/" + name, SHA256: hex.EncodeToString(h.Sum(nil)), Size: n})
	}
	if len(r.Assets) == 0 {
		return errors.New("no assets matched the version")
	}
	sort.Slice(r.Assets, func(i, j int) bool { return r.Assets[i].URL < r.Assets[j].URL })
	var rels []update.Release
	for _, old := range m.Releases {
		if update.Compare(old.Version, *version) != 0 {
			rels = append(rels, old)
		}
	}
	m.Releases = append(rels, r)
	sort.Slice(m.Releases, func(i, j int) bool { return update.Compare(m.Releases[i].Version, m.Releases[j].Version) > 0 })
	m.Generated = time.Now().UTC()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
}
