//go:build windows

package backup

import "strings"

func hasPlatformReservedPrefix(name, prefix string) bool {
	return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

func validPlatformDestinationInput(raw string) bool {
	separator := strings.LastIndexAny(raw, `/\\`)
	leaf := raw[separator+1:]
	return validDestinationLeaf(leaf)
}

func validPlatformLeaf(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `<>:"|?*`) {
		return false
	}
	for _, character := range name {
		if character <= 0x1f {
			return false
		}
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	}
	if len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' &&
		(stem[:3] == "COM" || stem[:3] == "LPT") {
		return false
	}
	return true
}
