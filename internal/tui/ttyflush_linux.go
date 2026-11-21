package tui

import (
	"errors"

	"golang.org/x/sys/unix"
)

func discardInput(fd uintptr) error {
	err := unix.IoctlSetInt(int(fd), unix.TCFLSH, unix.TCIFLUSH)
	if errors.Is(err, unix.ENOTTY) {
		return nil
	}
	return err
}
