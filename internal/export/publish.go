package export

import "os"

// A hard link also provides an atomic exclusive claim. If neither it nor
// the platform's exclusive rename works, fail without replacing any file.
func linkNoReplace(from, to string) error {
	if err := os.Link(from, to); err != nil {
		return err
	}
	// Once the complete final file exists, failure to remove the temporary
	// name cannot invalidate it. CleanPartials can remove that name later.
	_ = os.Remove(from)
	return nil
}
