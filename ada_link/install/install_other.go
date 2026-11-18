//go:build !windows

// Package install installs a MyGo application for the current Windows user.
// Elsewhere the calls report that installing is unsupported, so that apps
// using it still build and run on other platforms.
package install

import (
	"errors"
	"os"
	"path/filepath"
)

// Shortcut is one Start Menu entry, and optionally a desktop one.
type Shortcut struct {
	Name        string
	Exe         string
	Description string
	Desktop     bool
}

// Config describes an app for Install and Uninstall.
type Config struct {
	Name               string
	Version            string
	Publisher          string
	Dir                string
	Exes               map[string]string
	Host               string
	UninstallArgs      []string
	QuietUninstallArgs []string
	Shortcuts          []Shortcut
	Key                string
}

// Result is what Install did.
type Result struct {
	Dir       string
	Shortcuts []string
}

// ErrUnsupported is returned by Install and Uninstall off Windows.
var ErrUnsupported = errors.New("install: installing is supported on Windows only")

// DefaultDir returns where an app named name would install.
func DefaultDir(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), name)
	}
	return filepath.Join(home, ".local", "share", name)
}

// Status reports that the app is not installed.
func Status(Config) (installed bool, dir string) { return false, "" }

// Install reports that installing is Windows-only.
func Install(Config) (Result, error) { return Result{}, ErrUnsupported }

// Uninstall reports that uninstalling is Windows-only.
func Uninstall(Config) (bool, error) { return false, ErrUnsupported }

// Confirm cannot ask on a platform without the installer.
func Confirm(string, string) bool { return false }

// Inform does nothing.
func Inform(string, string) {}

// Warn does nothing.
func Warn(string, string) {}
