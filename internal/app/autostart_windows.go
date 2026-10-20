//go:build windows

package app

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentPackageFullName = kernel32.NewProc("GetCurrentPackageFullName")
	procGetTickCount64            = kernel32.NewProc("GetTickCount64")
)

// StartedAtLogin: en la versión de la Store la tarea de inicio no puede pasar --minimized, así que
// si Windows arrancó hace menos de 10 minutos se asume que la abrió el inicio de sesión.
func StartedAtLogin() bool {
	if !Packaged() {
		return false
	}
	ms, _, _ := procGetTickCount64.Call()
	return ms < 10*60*1000
}

// Packaged dice si la app corre como paquete MSIX (instalada desde la Microsoft Store).
// Ahí el inicio con Windows lo maneja la tarea de inicio del paquete (Configuración › Aplicaciones
// › Inicio), no la clave Run del registro.
func Packaged() bool {
	if procGetCurrentPackageFullName.Find() != nil {
		return false
	}
	var n uint32
	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
	const appmodelErrorNoPackage = 15700
	return r != appmodelErrorNoPackage
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runName = "RebirthTrackUploader"

// Iniciar con Windows: entrada en HKCU\...\Run (solo para este usuario, sin permisos de admin).
func AutostartEnabled() bool {
	if Packaged() {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runName)
	return err == nil
}

func SetAutostart(on bool) error {
	if Packaged() {
		return errors.New("instalada desde la Microsoft Store: el inicio con Windows se activa en Configuración › Aplicaciones › Inicio")
	}
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
