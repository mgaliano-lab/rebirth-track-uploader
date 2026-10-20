//go:build windows

package app

import (
	"io"
	"os"
	"os/exec"
	"syscall"
)

const detached = 0x00000008 | 0x00000200 // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP

func applyUpdate(mode, path string) error {
	if mode == "installer" {
		// El instalador (por usuario, sin admin) reemplaza la app y la vuelve a abrir minimizada,
		// respetando el inicio con Windows y el acceso directo tal como estaban.
		cmd := exec.Command(path, "/S")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached}
		return cmd.Start()
	}
	// Portable: Windows deja renombrar el .exe en uso; el nuevo ocupa su lugar.
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	os.Remove(exe + ".old")
	if err := os.Rename(exe, exe+".old"); err != nil {
		return err
	}
	if err := copyFile(path, exe); err != nil {
		os.Rename(exe+".old", exe)
		return err
	}
	cmd := exec.Command(exe, "--after-update", "--minimized")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached}
	return cmd.Start()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
