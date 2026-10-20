// Package addon instala o actualiza el addon Rebirth Track en las carpetas AddOns de cada
// versión de WoW, desde la última versión publicada en el CDN (con verificación SHA-256).
package addon

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Folder = "RebirthTrack"

type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

var client = &http.Client{Timeout: 60 * time.Second}

func Latest(url string) (Release, error) {
	var r Release
	res, err := client.Get(url)
	if err != nil {
		return r, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return r, fmt.Errorf("latest.json: %s", res.Status)
	}
	return r, json.NewDecoder(res.Body).Decode(&r)
}

// Download baja el zip y verifica su SHA-256 contra latest.json.
func Download(r Release) ([]byte, error) {
	res, err := client.Get(r.URL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("descarga del addon: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), r.SHA256) {
		return nil, fmt.Errorf("el addon descargado no coincide con la firma publicada (SHA-256)")
	}
	return b, nil
}

// Install reemplaza AddOns/RebirthTrack por el contenido del zip (solo esa carpeta).
func Install(zipData []byte, addonsDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(addonsDir, ".rebirthtrack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		// Solo archivos dentro de RebirthTrack/, sin rutas que se escapen.
		if !strings.HasPrefix(f.Name, Folder+"/") || strings.Contains(f.Name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(tmp, name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 10<<20))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, Folder, Folder+".toc")); err != nil {
		return fmt.Errorf("el zip no trae %s/%s.toc", Folder, Folder)
	}
	final := filepath.Join(addonsDir, Folder)
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(filepath.Join(tmp, Folder), final)
}

// InstalledVersion lee "## Version:" del .toc instalado ("" si no está).
func InstalledVersion(addonsDir string) string {
	f, err := os.Open(filepath.Join(addonsDir, Folder, Folder+".toc"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "## Version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return "?"
}
