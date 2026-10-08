// Package version identifies the running build. Release builds set these
// values with -ldflags "-X github.com/oaovito/mne_lab/internal/version.Version=...".
package version

var (
	// Version is the semantic version of the build.
	Version = "0.1.0-dev"
	// Channel is stable, beta or dev.
	Channel = "dev"
	// Commit is the source revision.
	Commit = ""
	// Date is the build date (RFC 3339).
	Date = ""
)

// Name is the application's visual name.
const Name = "MNE Lab"

// Schema is the data schema version written in exports and packages.
const Schema = 1

// String returns "MNE Lab <version>".
func String() string { return Name + " " + Version }
