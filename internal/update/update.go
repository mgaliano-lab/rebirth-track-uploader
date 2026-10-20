// Package update busca versiones nuevas de la app en los releases públicos de GitHub y las baja
// verificando el SHA-256 publicado en el mismo release (SHA256SUMS.txt). La versión de la
// Microsoft Store no usa esto: la actualiza la Store.
package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Repo = "mgaliano-lab/rebirth-track-uploader"

var latestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	Version  string `json:"version"`
	PageURL  string `json:"pageUrl"`
	Setup    *Asset `json:"-"` // instalador (RebirthTrackUploader-Setup-X.exe)
	Portable *Asset `json:"-"` // ejecutable suelto (RebirthTrackUploader-X.exe)
	sums     *Asset
}

var client = &http.Client{Timeout: 5 * time.Minute}

func Latest() (Release, error) {
	req, _ := http.NewRequest("GET", latestURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "RebirthTrackUploader")
	res, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Release{}, fmt.Errorf("GitHub: %s", res.Status)
	}
	var gh struct {
		Tag    string  `json:"tag_name"`
		URL    string  `json:"html_url"`
		Assets []Asset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&gh); err != nil {
		return Release{}, err
	}
	r := Release{Version: strings.TrimPrefix(gh.Tag, "v"), PageURL: gh.URL}
	for i := range gh.Assets {
		a := &gh.Assets[i]
		switch a.Name {
		case "RebirthTrackUploader-Setup-" + r.Version + ".exe":
			r.Setup = a
		case "RebirthTrackUploader-" + r.Version + ".exe":
			r.Portable = a
		case "SHA256SUMS.txt":
			r.sums = a
		}
	}
	return r, nil
}

// Newer dice si latest es una versión posterior a current (x.y.z). "dev" nunca se actualiza.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return out, false // versiones de prueba (0.1.2-dev.abc): no se comparan
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download baja un archivo del release a dir y verifica su SHA-256 contra SHA256SUMS.txt.
func Download(r Release, a *Asset, dir string) (string, error) {
	if a == nil || r.sums == nil {
		return "", fmt.Errorf("el release %s no tiene el archivo para esta instalación", r.Version)
	}
	want, err := sumFor(r.sums.URL, a.Name)
	if err != nil {
		return "", err
	}
	res, err := client.Get(a.URL)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("descarga de %s: %s", a.Name, res.Status)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, a.Name)
	f, err := os.Create(dst + ".part")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, 100<<20))
	f.Close()
	if err != nil {
		os.Remove(dst + ".part")
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		os.Remove(dst + ".part")
		return "", fmt.Errorf("%s no coincide con su SHA-256 publicado; no se instala", a.Name)
	}
	return dst, os.Rename(dst+".part", dst)
}

func sumFor(url, name string) (string, error) {
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("SHA256SUMS.txt: %s", res.Status)
	}
	sc := bufio.NewScanner(io.LimitReader(res.Body, 1<<16))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS.txt no tiene %s", name)
}
