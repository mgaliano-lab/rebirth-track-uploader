//go:build !windows

package app

import "os"

func AutostartEnabled() bool     { return false }
func SetAutostart(on bool) error { return nil }

// Packaged: fuera de Windows no hay Store; REBIRTH_TRACK_FAKE_STORE=1 la simula para probar el panel.
func Packaged() bool       { return os.Getenv("REBIRTH_TRACK_FAKE_STORE") == "1" }
func StartedAtLogin() bool { return false }
