//go:build !windows

package browserqualification

import "os"

func singleLink(*os.File) bool { return false }
