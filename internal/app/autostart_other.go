//go:build !windows

package app

func AutostartEnabled() bool     { return false }
func SetAutostart(on bool) error { return nil }
