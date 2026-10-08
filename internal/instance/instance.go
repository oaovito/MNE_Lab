// Package instance keeps one MNE Lab running per data folder. A second
// launch hands over to the running one (which shows its window) instead of
// opening the same encrypted data twice.
package instance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ErrRunning means another process holds the lock.
var ErrRunning = errors.New("instance.already_running")

// Info is published by the running instance so a second launch can ask it
// to show its window. The token only allows that single request.
type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// Lock is a held instance lock.
type Lock struct {
	f    *os.File
	path string
}

// Acquire takes the lock file in dir.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "instance.lock")
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, ErrRunning
	}
	return &Lock{f: f, path: p}, nil
}

// Publish records how to reach this instance.
func (l *Lock) Publish(info Info) error {
	b, _ := json.Marshal(info)
	return os.WriteFile(filepath.Join(filepath.Dir(l.path), "instance.json"), b, 0o600)
}

// Release frees the lock.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	os.Remove(filepath.Join(filepath.Dir(l.path), "instance.json"))
	unlockFile(l.f)
	l.f.Close()
	l.f = nil
}

// Running reads the published info of the running instance.
func Running(dir string) (Info, error) {
	var i Info
	b, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		return i, err
	}
	return i, json.Unmarshal(b, &i)
}

// Held reports whether some process currently holds the lock in dir.
func Held(dir string) bool {
	l, err := Acquire(dir)
	if err != nil {
		return errors.Is(err, ErrRunning)
	}
	l.unlockOnly()
	return false
}

func (l *Lock) unlockOnly() {
	unlockFile(l.f)
	l.f.Close()
}
