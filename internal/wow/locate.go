package wow

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Normalize lleva a la carpeta raíz de WoW lo que haya elegido el usuario: si eligió una versión
// (…\_retail_) o algo de adentro (…\_retail_\Logs), sube hasta la raíz.
func Normalize(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	for i := 0; i < 3; i++ {
		base := strings.ToLower(filepath.Base(p))
		if base == "logs" || base == "interface" || base == "wtf" || base == "addons" {
			p = filepath.Dir(p)
			continue
		}
		break
	}
	return parentIfFlavor(p)
}

// Locate busca instalaciones de WoW en los lugares habituales de todos los discos (y un nivel de
// carpetas en la raíz de cada uno). Devuelve las raíces que tienen al menos una versión, la que
// más versiones tiene primero.
func Locate() []string {
	seen := map[string]bool{}
	var found []string
	try := func(p string) {
		p = filepath.Clean(p)
		key := strings.ToLower(p)
		if seen[key] {
			return
		}
		seen[key] = true
		if _, ins := Detect(p); len(ins) > 0 {
			found = append(found, p)
		}
	}
	for _, r := range Roots("") {
		try(r)
	}
	const name = "World of Warcraft"
	if runtime.GOOS == "windows" {
		for d := 'C'; d <= 'Z'; d++ {
			drive := string(d) + `:\`
			if !isDir(drive) {
				continue
			}
			for _, sub := range []string{"", `Program Files (x86)`, `Program Files`, "Games", "Juegos", "Blizzard", `Battle.net`, `Program Files (x86)\Blizzard`} {
				try(filepath.Join(drive, sub, name))
			}
			// Un nivel: X:\<cualquier carpeta>\World of Warcraft
			if entries, err := os.ReadDir(drive); err == nil {
				for _, e := range entries {
					if e.IsDir() && !strings.HasPrefix(e.Name(), "$") {
						try(filepath.Join(drive, e.Name(), name))
					}
				}
			}
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		try(filepath.Join(home, "Applications", name))
		try(filepath.Join(home, "Games", name))
	}
	// La que más versiones tiene primero.
	count := func(p string) int { _, ins := Detect(p); return len(ins) }
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && count(found[j]) > count(found[j-1]); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}
	return found
}
