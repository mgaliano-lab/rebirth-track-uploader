package scans

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Match es un archivo encontrado para un patrón, dentro de la carpeta de una versión de WoW.
type Match struct {
	Rel     string // con «/», con los nombres tal como están en disco
	Path    string // absoluta
	Size    int64
	ModTime time.Time
}

// Tope de entradas que se miran por carpeta (una carpeta enorme no frena la app).
const maxDirEntries = 5000

// Find busca los archivos de un patrón dentro de dir sin salir de ahí: recorre tramo a tramo (sin
// filepath.Glob, que interpretaría comodines del propio dir), no sigue enlaces simbólicos ni
// junctions, y solo devuelve archivos comunes con extensión de datos. Sin distinguir mayúsculas,
// como Windows.
func Find(dir, pattern string) ([]Match, error) {
	if err := PathProblem(pattern); err != nil {
		return nil, err
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, nil // la carpeta no existe (todavía)
	}
	base, _ = filepath.Abs(base)
	segs := strings.Split(pattern, "/")
	type node struct{ abs, rel string }
	level := []node{{base, ""}}
	for i, seg := range segs {
		last := i == len(segs)-1
		var next []node
		for _, n := range level {
			entries, err := os.ReadDir(n.abs)
			if err != nil {
				continue
			}
			if len(entries) > maxDirEntries {
				entries = entries[:maxDirEntries]
			}
			for _, e := range entries {
				if ok, _ := filepath.Match(strings.ToLower(seg), strings.ToLower(e.Name())); !ok {
					continue
				}
				t := e.Type()
				if (last && !t.IsRegular()) || (!last && t&fs.ModeType != fs.ModeDir) {
					continue // enlaces, junctions y lo que no sea archivo/carpeta común
				}
				rel := e.Name()
				if n.rel != "" {
					rel = n.rel + "/" + e.Name()
				}
				next = append(next, node{filepath.Join(n.abs, e.Name()), rel})
			}
		}
		level = next
		if len(level) == 0 {
			return nil, nil
		}
	}
	var out []Match
	for _, n := range level {
		if len(out) >= MaxMatches {
			break
		}
		if !HasDataExt(n.rel) || !Inside(base, n.abs) {
			continue
		}
		st, err := os.Lstat(n.abs)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		out = append(out, Match{Rel: n.rel, Path: n.abs, Size: st.Size(), ModTime: st.ModTime()})
	}
	return out, nil
}

// Inside: p (resolviendo enlaces) está dentro de base.
func Inside(base, p string) bool {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, real)
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ErrTooBig: el archivo pasa el tope del relevamiento.
type ErrTooBig struct{ Size, Max int64 }

func (e ErrTooBig) Error() string { return "supera el tamaño máximo" }

// Read lee el archivo con el tope del relevamiento (y el duro de la app), aunque crezca mientras se lee.
func Read(m Match, max int64) ([]byte, error) {
	max = min(max, MaxBytes)
	if m.Size > max {
		return nil, ErrTooBig{m.Size, max}
	}
	f, err := os.Open(m.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrTooBig{int64(len(b)), max}
	}
	return b, nil
}
