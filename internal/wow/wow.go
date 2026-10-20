// Package wow encuentra la instalación de World of Warcraft y sus versiones (retail, Classic…).
package wow

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// Install es una versión del juego dentro de la carpeta de WoW (ej. _anniversary_).
type Install struct {
	Flavor string `json:"flavor"`
	Label  string `json:"label"`
	Dir    string `json:"dir"`
}

var Labels = map[string]string{
	"_retail_":      "Retail",
	"_classic_era_": "Era / WoW Forever",
	"_anniversary_": "Classic Anniversary",
	"_classic_":     "Classic (MoP)",
}

// Roots devuelve las carpetas candidatas a "World of Warcraft", en orden de preferencia.
func Roots(override string) []string {
	if override != "" {
		return []string{override}
	}
	var out []string
	out = append(out, registryRoots()...)
	switch runtime.GOOS {
	case "windows":
		for _, d := range []string{`C:\Program Files (x86)`, `C:\Program Files`, `D:\`, `D:\Games`, `E:\`, `E:\Games`} {
			out = append(out, filepath.Join(d, "World of Warcraft"))
		}
	case "darwin":
		out = append(out, "/Applications/World of Warcraft")
	}
	return out
}

// Detect lista las versiones instaladas (carpetas _x_ con Interface, Logs o WTF).
func Detect(override string) (root string, installs []Install) {
	for _, r := range Roots(override) {
		var found []Install
		for flavor, label := range Labels {
			dir := filepath.Join(r, flavor)
			if isDir(filepath.Join(dir, "Interface")) || isDir(filepath.Join(dir, "WTF")) || isDir(filepath.Join(dir, "Logs")) {
				found = append(found, Install{Flavor: flavor, Label: label, Dir: dir})
			}
		}
		if len(found) > 0 {
			sort.Slice(found, func(i, j int) bool { return found[i].Flavor < found[j].Flavor })
			return r, found
		}
	}
	return "", nil
}

func (i Install) LogsDir() string   { return filepath.Join(i.Dir, "Logs") }
func (i Install) AddOnsDir() string { return filepath.Join(i.Dir, "Interface", "AddOns") }

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
