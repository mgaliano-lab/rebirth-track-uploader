//go:build !windows

package app

import "errors"

func applyUpdate(mode, path string) error {
	return errors.New("la actualización automática es solo para Windows")
}
