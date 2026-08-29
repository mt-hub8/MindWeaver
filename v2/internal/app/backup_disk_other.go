//go:build !windows

package app

import (
	"errors"
	"syscall"
)

func isDiskFull(err error) bool { return errors.Is(err, syscall.ENOSPC) }
