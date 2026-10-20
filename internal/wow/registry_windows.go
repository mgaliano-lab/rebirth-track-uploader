//go:build windows

package wow

import "golang.org/x/sys/windows/registry"

// El instalador de Battle.net deja la ruta en el registro.
func registryRoots() []string {
	var out []string
	for _, k := range []string{`SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`, `SOFTWARE\Blizzard Entertainment\World of Warcraft`} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, k, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if p, _, err := key.GetStringValue("InstallPath"); err == nil && p != "" {
			// InstallPath apunta a la carpeta de una versión (…\World of Warcraft\_retail_\).
			out = append(out, parentIfFlavor(p))
		}
		key.Close()
	}
	return out
}
