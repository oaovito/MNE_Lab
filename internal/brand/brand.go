// Package brand holds the official MNE Lab icon in the formats the
// executable needs at run time. The masters are in assets/brand.
package brand

import (
	_ "embed"
	"runtime"
)

//go:embed tray.ico
var trayICO []byte

//go:embed tray.png
var trayPNG []byte

// Icon256 is the application icon (PNG, 256 px).
//
//go:embed icon-256.png
var Icon256 []byte

// TrayIcon returns the tray icon in the format of this operating system.
func TrayIcon() []byte {
	if runtime.GOOS == "windows" {
		return trayICO
	}
	return trayPNG
}
