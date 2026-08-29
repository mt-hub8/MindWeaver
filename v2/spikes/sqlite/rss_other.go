//go:build !windows

package sqliteprobe

type rssSample struct {
	supported   bool
	measurement string
	current     uint64
	peak        uint64
	err         string
}

func readRSS() rssSample {
	return rssSample{
		measurement: "unsupported on this platform",
		err:         "this spike records process RSS only on Windows",
	}
}
