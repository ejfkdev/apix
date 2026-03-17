//go:build !windows

package main

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func stdinPrefetch() ([]byte, bool, error) {
	fd := int(os.Stdin.Fd())
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, false, err
	}
	defer func() {
		_ = unix.SetNonblock(fd, false)
	}()

	buf := make([]byte, 1)
	n, err := os.Stdin.Read(buf)
	if n > 0 {
		return buf[:n], true, nil
	}
	if err == nil {
		return nil, false, nil
	}
	if errors.Is(err, io.EOF) {
		return nil, false, nil
	}
	if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return nil, true, nil
	}
	return nil, false, err
}
