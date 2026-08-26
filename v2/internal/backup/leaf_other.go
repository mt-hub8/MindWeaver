//go:build !windows

package backup

import "strings"

func hasPlatformReservedPrefix(name, prefix string) bool { return strings.HasPrefix(name, prefix) }

func validPlatformLeaf(string) bool { return true }

func validPlatformDestinationInput(string) bool { return true }
