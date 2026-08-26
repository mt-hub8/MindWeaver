// Package lcms2 is a pure-Go port of Little-CMS 2
// (https://github.com/mm2/Little-CMS), the ICC-based colour-management engine.
// It reads and writes ICC profiles, builds colour transforms between them, and
// applies those transforms to pixel buffers.
//
// The port targets the lcms2 2.19 core public API and uses the reference C
// implementation as a differential-testing oracle. It depends only on the Go
// standard library — no cgo, no third-party modules — and never panics: every
// failure, including malformed or untrusted input, is returned as an error
// (see [Error]). Integer transform and profile-I/O paths are bit-exact with the
// C library; floating-point paths match within the reference testbed tolerances.
//
// # Transforms
//
// Open or create two profiles, build a [Transform] between them for a given
// pixel format and rendering intent, then apply it:
//
//	in, _ := lcms2.Create_sRGBProfile()
//	out, _ := lcms2.OpenProfileFromFile("USWebCoatedSWOP.icc", "r")
//	xform, err := lcms2.CreateTransform(
//		in, lcms2.TypeRGB8,
//		out, lcms2.TypeCMYK8,
//		lcms2.IntentPerceptual, 0)
//	if err != nil {
//		// handle error
//	}
//	xform.DoTransform(rgb, cmyk, nPixels)
//
// A [Transform] is safe for concurrent DoTransform calls, so a large image can
// be split across goroutines.
//
// # Pixel formats
//
// Pixel formats are uint32 values encoded exactly as the C TYPE_* macros. Use a
// named constant such as [TypeRGB8], [TypeCMYK16], or [TypeRGBFlt], or compose
// one with the bitfield builders, for example
// ColorspaceSH([PTRGB]) | ChannelsSH(3) | BytesSH(2).
//
// # Scope
//
// The core library is covered: profile I/O for every ICC tag type, tone curves,
// interpolation, pipelines, the transform engine (all intents, optimization,
// soft-proofing, gamut check, alpha), virtual profiles, CIECAM02, IT8.7/CGATS,
// PostScript CSA/CRD generation, and the .cube device-link reader. The GPL-3.0
// lcms2 plugins (fast_float, threaded, GPU) are not included, keeping this
// library MIT-licensed.
package lcms2
