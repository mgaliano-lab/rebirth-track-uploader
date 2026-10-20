package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenRoundTripNotInClearOnWindows(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *State) { s.Token = "secreto-de-prueba" }); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Read().Token != "secreto-de-prueba" {
		t.Fatalf("token: %q", again.Read().Token)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if isWindows && strings.Contains(string(raw), "secreto-de-prueba") {
		t.Fatal("en Windows el token no puede quedar en claro en el archivo")
	}
}
