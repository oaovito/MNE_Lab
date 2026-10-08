package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Build-time configuration of official application registrations. Values
// are injected with -ldflags -X by the release pipeline and are never
// committed. A build without them still offers the folder transport.
var (
	GoogleClientID     = ""
	GoogleClientSecret = "" // Google's "installed app" secret; not confidential per Google, still kept out of the repository
	MicrosoftClientID  = ""
	CloudKitContainer  = ""
	CloudKitAPIToken   = ""
	CloudKitEnv        = "production"
)

// Config is the resolved registration configuration.
type Config struct {
	GoogleClientID     string `json:"googleClientId"`
	GoogleClientSecret string `json:"googleClientSecret"`
	MicrosoftClientID  string `json:"microsoftClientId"`
	CloudKitContainer  string `json:"cloudKitContainer"`
	CloudKitAPIToken   string `json:"cloudKitApiToken"`
	CloudKitEnv        string `json:"cloudKitEnvironment"`
}

// LoadConfig merges build-time values with an optional local file
// (data/providers.json) used by development builds.
func LoadConfig(dataDir string) Config {
	c := Config{GoogleClientID, GoogleClientSecret, MicrosoftClientID, CloudKitContainer, CloudKitAPIToken, CloudKitEnv}
	if b, err := os.ReadFile(filepath.Join(dataDir, "providers.json")); err == nil {
		var f Config
		if json.Unmarshal(b, &f) == nil {
			if f.GoogleClientID != "" {
				c.GoogleClientID, c.GoogleClientSecret = f.GoogleClientID, f.GoogleClientSecret
			}
			if f.MicrosoftClientID != "" {
				c.MicrosoftClientID = f.MicrosoftClientID
			}
			if f.CloudKitContainer != "" {
				c.CloudKitContainer, c.CloudKitAPIToken = f.CloudKitContainer, f.CloudKitAPIToken
				if f.CloudKitEnv != "" {
					c.CloudKitEnv = f.CloudKitEnv
				}
			}
		}
	}
	return c
}

// APIAvailable reports whether the API transport of a provider is
// configured in this build.
func (c Config) APIAvailable(id string) bool {
	switch id {
	case Google:
		return c.GoogleClientID != ""
	case OneDrive:
		return c.MicrosoftClientID != ""
	case ICloud:
		return c.CloudKitContainer != "" && c.CloudKitAPIToken != ""
	}
	return false
}
