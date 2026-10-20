// Package state guarda la configuración y el progreso de la app en la carpeta de configuración
// del usuario (%APPDATA%\RebirthTrack en Windows). Nada de esto sale de la PC salvo el token,
// que se usa como credencial contra Rebirth Track.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileProgress es hasta dónde se leyó cada archivo de log y en qué zona quedó.
type FileProgress struct {
	SessionID int    `json:"sessionId,omitempty"`
	Offset    int64  `json:"offset"`
	Seq       int    `json:"seq"`
	InRaid    bool   `json:"inRaid"`
	Zone      string `json:"zone,omitempty"`
	Header    string `json:"header,omitempty"`   // línea COMBAT_LOG_VERSION, va al principio de la sesión
	Skipped   bool   `json:"skipped,omitempty"`  // era anterior al consentimiento y no se subió (se puede importar)
	Imported  bool   `json:"imported,omitempty"` // el usuario pidió importarlo desde el principio
}

type State struct {
	ConsentAt  *time.Time               `json:"consentAt,omitempty"`
	Token      string                   `json:"-"`               // en memoria, en claro
	TokenEnc   string                   `json:"token,omitempty"` // en disco: cifrado (DPAPI en Windows)
	UserName   string                   `json:"userName,omitempty"`
	DeviceName string                   `json:"deviceName,omitempty"`
	Paused     bool                     `json:"paused"`
	WowRoot    string                   `json:"wowRoot,omitempty"` // elegido a mano; vacío = detectar
	Disabled   map[string]bool          `json:"disabledFlavors,omitempty"`
	AddonAuto  bool                     `json:"addonAutoUpdate"`
	AppAuto    bool                     `json:"appAutoUpdate"`
	Every      int                      `json:"uploadEverySeconds,omitempty"` // 0 = lo que diga el sitio (3 min)
	Files      map[string]*FileProgress `json:"files"`
}

type Store struct {
	mu   sync.Mutex
	path string
	s    State
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "RebirthTrack"), nil
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st := &Store{path: filepath.Join(dir, "state.json"), s: State{AddonAuto: true, AppAuto: true}}
	b, err := os.ReadFile(st.path)
	if err == nil {
		if err := json.Unmarshal(b, &st.s); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if st.s.TokenEnc != "" {
		tok, err := unprotect(st.s.TokenEnc)
		if err != nil {
			st.s.TokenEnc = "" // no se puede descifrar (otra PC/usuario): hay que volver a vincular
		}
		st.s.Token = tok
	}
	if st.s.Files == nil {
		st.s.Files = map[string]*FileProgress{}
	}
	if st.s.Disabled == nil {
		st.s.Disabled = map[string]bool{}
	}
	return st, nil
}

// Read devuelve una copia (los punteros de Files siguen compartidos: usar Update para tocarlos).
func (st *Store) Read() State {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// Update aplica un cambio y lo guarda en disco de forma atómica.
func (st *Store) Update(fn func(s *State)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	fn(&st.s)
	enc, err := protect(st.s.Token)
	if err != nil {
		return err
	}
	st.s.TokenEnc = enc
	b, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// File devuelve (creando si hace falta) el progreso de un archivo, bajo el lock.
func (st *Store) File(key string) FileProgress {
	st.mu.Lock()
	defer st.mu.Unlock()
	if p, ok := st.s.Files[key]; ok {
		return *p
	}
	return FileProgress{}
}
