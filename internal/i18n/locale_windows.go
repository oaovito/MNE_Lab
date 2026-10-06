//go:build windows

package i18n

import (
	"syscall"
	"unsafe"
)

func systemLocale() string {
	k := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	buf := make([]uint16, 85)
	r, _, _ := k.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
