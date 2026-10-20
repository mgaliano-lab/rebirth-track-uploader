//go:build windows

package state

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

// El token del dispositivo se guarda cifrado con DPAPI: solo este usuario de Windows, en esta PC,
// puede descifrarlo (aunque alguien copie state.json a otra máquina, no le sirve).
var entropy = []byte("RebirthTrackUploader/token/v1")

func protect(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	in := blob([]byte(plain))
	ent := blob(entropy)
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return "dpapi:" + base64.StdEncoding.EncodeToString(unsafe.Slice(out.Data, out.Size)), nil
}

func unprotect(stored string) (string, error) {
	if len(stored) < 6 || stored[:6] != "dpapi:" {
		return stored, nil // formato viejo en claro: se vuelve a guardar cifrado
	}
	raw, err := base64.StdEncoding.DecodeString(stored[6:])
	if err != nil {
		return "", err
	}
	in := blob(raw)
	ent := blob(entropy)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return string(unsafe.Slice(out.Data, out.Size)), nil
}

func blob(b []byte) windows.DataBlob {
	if len(b) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}
