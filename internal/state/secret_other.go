//go:build !windows

package state

// Fuera de Windows (solo pruebas) el token queda en el archivo, que es 0600.
func protect(plain string) (string, error)    { return plain, nil }
func unprotect(stored string) (string, error) { return stored, nil }
