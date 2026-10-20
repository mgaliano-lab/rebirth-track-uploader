//go:build !windows

package app

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// pickFolder: en macOS (solo para probar la app) usa el selector del sistema.
func pickFolder(start string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("el selector de carpetas solo está en Windows: escribí la ruta")
	}
	out, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "Elegí la carpeta World of Warcraft")`).Output()
	if err != nil {
		return "", nil // cancelado
	}
	return strings.TrimSpace(string(out)), nil
}
