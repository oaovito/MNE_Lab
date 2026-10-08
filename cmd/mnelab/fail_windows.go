package main

import (
	"golang.org/x/sys/windows"
)

func fail(err error) {
	title, _ := windows.UTF16PtrFromString("MNE Lab")
	msg, _ := windows.UTF16PtrFromString("MNE Lab could not start.\n\n" + err.Error())
	windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONERROR)
}
