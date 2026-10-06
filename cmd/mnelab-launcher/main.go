// Command mnelab-launcher is "MNE Lab.exe" at the root of a Portable USB
// install. It starts the selected build from app/<version>/, returns to the
// previous build if a new one repeatedly fails to start, and exits so the
// launcher itself never stays locked while MNE Lab runs.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/update"
)

func main() {
	if err := run(); err != nil {
		fail(err.Error())
		os.Exit(1)
	}
}

func run() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	root := filepath.Dir(self)
	if _, err := os.Stat(filepath.Join(root, paths.PortableMarker)); err != nil {
		return fmt.Errorf("This folder is not a complete MNE Lab portable installation (%s is missing).", paths.PortableMarker)
	}
	builds := filepath.Join(root, "app")
	exe, version, err := update.Resolve(builds)
	if err != nil {
		return fmt.Errorf("No MNE Lab build was found in %s.", builds)
	}
	args := append([]string{"--root", root, "--build", version}, os.Args[1:]...)
	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	detach(cmd)
	return cmd.Start()
}
