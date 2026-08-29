//go:build !windows

package backup

import "os"

func secureVerifyScratchDirectory(*os.File) error { return ErrUnsupportedPlatform }
func secureVerifyScratchFile(*os.File) error      { return ErrUnsupportedPlatform }
func applyVerifyScratchPathSecurity(string) error { return ErrUnsupportedPlatform }
