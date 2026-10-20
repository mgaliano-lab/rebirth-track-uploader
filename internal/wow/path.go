package wow

import (
	"path/filepath"
	"strings"
)

func parentIfFlavor(p string) string {
	p = filepath.Clean(p)
	base := filepath.Base(p)
	if strings.HasPrefix(base, "_") && strings.HasSuffix(base, "_") {
		return filepath.Dir(p)
	}
	return p
}
