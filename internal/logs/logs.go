// Package logs lee los registros de combate que escribe WoW (Logs/WoWCombatLog*.txt) desde donde
// quedó la última vez y se queda SOLO con los tramos de bandas: lo que el juego registró entre un
// ZONE_CHANGE a una instancia con dificultad de banda y el siguiente ZONE_CHANGE a otro lado.
// Todo lo demás (mundo abierto, mazmorras, JcJ) se descarta en la PC y nunca se sube.
package logs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Chunk son las líneas nuevas de un archivo listas para subir.
type Chunk struct {
	Data        []byte // líneas completas (con \n), solo de bandas
	Header      []byte // última línea COMBAT_LOG_VERSION vista (se manda al principio de cada sesión)
	Lines       int
	OffsetStart int64
	OffsetEnd   int64 // hasta acá quedó leído el archivo
	InRaid      bool  // estado al final del tramo (para seguir en la próxima lectura)
	Zone        string
	Zones       []string // bandas que aparecen en el tramo
	Skipped     int      // líneas descartadas por no ser de banda
}

// File es un archivo de log encontrado en la carpeta Logs.
type File struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

var nameRe = regexp.MustCompile(`^WoWCombatLog[\w.-]*\.txt$`)

func List(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, File{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.Before(out[j].ModTime) })
	return out, nil
}

// MaxChunk limita lo que se lee de una vez (un lote) para no cargar archivos enormes en memoria.
const MaxChunk = 24 << 20

// Read lee desde offset hasta la última línea completa (como mucho MaxChunk bytes) y filtra.
func Read(path string, offset int64, inRaid bool, zone string, raidDifficulties map[int]bool) (Chunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return Chunk{}, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return Chunk{}, err
	}
	buf := make([]byte, MaxChunk)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Chunk{}, err
	}
	buf = buf[:n]
	// Solo líneas completas: la última puede estar a medio escribir.
	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		return Chunk{OffsetStart: offset, OffsetEnd: offset, InRaid: inRaid, Zone: zone}, nil
	}
	buf = buf[:last+1]
	ch := Filter(buf, inRaid, zone, raidDifficulties)
	ch.OffsetStart = offset
	ch.OffsetEnd = offset + int64(len(buf))
	return ch, nil
}

// ReadRange relee exactamente [offset, end) (límite de línea ya confirmado por el sitio) para
// recuperar el estado de zona en ese punto.
func ReadRange(path string, offset, end int64, inRaid bool, zone string, raidDifficulties map[int]bool) (Chunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return Chunk{}, err
	}
	defer f.Close()
	buf := make([]byte, end-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return Chunk{}, err
	}
	ch := Filter(buf, inRaid, zone, raidDifficulties)
	ch.OffsetStart, ch.OffsetEnd = offset, end
	return ch, nil
}

// Filter recorre las líneas y conserva las de bandas (más el encabezado COMBAT_LOG_VERSION).
func Filter(buf []byte, inRaid bool, zone string, raidDifficulties map[int]bool) Chunk {
	var out bytes.Buffer
	ch := Chunk{}
	seen := map[string]bool{}
	for len(buf) > 0 {
		i := bytes.IndexByte(buf, '\n')
		line := buf[:i+1]
		buf = buf[i+1:]
		ev := event(line)
		keep := inRaid
		switch ev {
		case "COMBAT_LOG_VERSION":
			// Versión del formato y si el modo avanzado está activo (sin datos personales):
			// va aparte, para anteponerla al primer lote de la sesión.
			ch.Header = append([]byte(nil), line...)
			continue
		case "ZONE_CHANGE":
			name, diff := zoneChange(line)
			inRaid = raidDifficulties[diff]
			zone = ""
			if inRaid {
				zone = name
				if !seen[name] {
					seen[name] = true
					ch.Zones = append(ch.Zones, name)
				}
			}
			keep = inRaid
		}
		if keep {
			out.Write(line)
			ch.Lines++
		} else {
			ch.Skipped++
		}
	}
	if inRaid && zone != "" && !seen[zone] {
		ch.Zones = append(ch.Zones, zone)
	}
	ch.Data = out.Bytes()
	ch.InRaid = inRaid
	ch.Zone = zone
	return ch
}

// "10/24/2026 21:03:11.123-3  ZONE_CHANGE,532,"Karazhan",3"
func event(line []byte) string {
	i := bytes.Index(line, []byte("  "))
	if i < 0 {
		return ""
	}
	rest := line[i+2:]
	j := bytes.IndexByte(rest, ',')
	if j < 0 {
		return strings.TrimSpace(string(rest))
	}
	return string(rest[:j])
}

func zoneChange(line []byte) (name string, difficulty int) {
	s := strings.TrimRight(string(line), "\r\n")
	i := strings.Index(s, "ZONE_CHANGE,")
	if i < 0 {
		return "", 0
	}
	s = s[i+len("ZONE_CHANGE,"):]
	// id,"Nombre, con comas",dificultad
	q1 := strings.IndexByte(s, '"')
	q2 := strings.LastIndexByte(s, '"')
	if q1 >= 0 && q2 > q1 {
		name = s[q1+1 : q2]
	}
	if k := strings.LastIndexByte(s, ','); k >= 0 {
		difficulty, _ = strconv.Atoi(strings.TrimSpace(s[k+1:]))
	}
	return name, difficulty
}

// FirstTimestamp lee la fecha de la primera línea ("M/D/YYYY HH:MM:SS.mmm"), si se puede.
func FirstTimestamp(path string) *time.Time {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, 64)
	n, _ := f.Read(b)
	s := string(b[:n])
	if i := strings.Index(s, "  "); i > 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, "-+"); i > 0 {
		s = s[:i]
	}
	for _, layout := range []string{"1/2/2006 15:04:05.000", "1/2/2006 15:04:05.0000"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t
		}
	}
	return nil
}
