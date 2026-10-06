//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

func systemLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
