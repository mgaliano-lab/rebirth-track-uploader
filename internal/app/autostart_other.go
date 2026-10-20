//go:build !windows

package app

func AutostartEnabled() bool     { return false }
func SetAutostart(on bool) error { return nil }
func Packaged() bool             { return false }
func StartedAtLogin() bool       { return false }
