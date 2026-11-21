//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package tui

import (
	"errors"

	"golang.org/x/sys/unix"
)

const fread = 0x1

func discardInput(fd uintptr) error {
	err := unix.IoctlSetPointerInt(int(fd), unix.TIOCFLUSH, fread)
	if errors.Is(err, unix.ENOTTY) {
		return nil
	}
	return err
}
