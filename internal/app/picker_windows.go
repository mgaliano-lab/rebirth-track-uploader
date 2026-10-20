//go:build windows

package app

import (
	"os/exec"
	"strings"
	"syscall"
)

// pickFolder abre el selector de carpetas de Windows (por PowerShell, sin ventana de consola).
// Devuelve "" si el usuario cancela.
func pickFolder(start string) (string, error) {
	esc := strings.ReplaceAll(start, "'", "''")
	script := `[Console]::OutputEncoding=[Text.Encoding]::UTF8;` +
		`Add-Type -AssemblyName System.Windows.Forms;` +
		`$d=New-Object System.Windows.Forms.FolderBrowserDialog;` +
		`$d.Description='Elegí la carpeta "World of Warcraft" (la que tiene _retail_, _anniversary_, etc.)';` +
		`$d.ShowNewFolderButton=$false;` +
		`if('` + esc + `' -ne ''){$d.SelectedPath='` + esc + `'};` +
		`$f=New-Object System.Windows.Forms.Form -Property @{TopMost=$true;ShowInTaskbar=$false};` +
		`if($d.ShowDialog($f) -eq 'OK'){[Console]::Out.Write($d.SelectedPath)}`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
