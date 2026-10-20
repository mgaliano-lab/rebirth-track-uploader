//go:build windows

package app

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runName = "RebirthTrackUploader"

// Iniciar con Windows: entrada en HKCU\...\Run (solo para este usuario, sin permisos de admin).
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runName)
	return err == nil
}

func SetAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runName, `"`+exe+`" --minimized`)
}
