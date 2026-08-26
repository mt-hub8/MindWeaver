package lcms2

// Version is the level of ICC format supported, encoded as in the C
// implementation's LCMS_VERSION (2.19 -> 2190).
const Version = 2190

// GetEncodedCMMVersion mirrors cmsGetEncodedCMMversion.
func GetEncodedCMMVersion() int { return Version }
