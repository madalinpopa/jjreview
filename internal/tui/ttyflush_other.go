//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package tui

func discardInput(uintptr) error {
	return nil
}
