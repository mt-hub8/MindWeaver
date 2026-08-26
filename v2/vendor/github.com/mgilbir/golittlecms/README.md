# golittlecms

A pure-Go port of [Little-CMS](https://github.com/mm2/Little-CMS) (lcms2), the
ICC-based colour-management engine. It reads and writes ICC profiles, builds
colour transforms between them, and applies those transforms to pixel buffers —
matching the C library's results bit-for-bit on the integer paths.

```go
import "github.com/mgilbir/golittlecms"
```

## Why

- **Pure Go, standard library only.** No cgo, no third-party modules, no
  assembly. It builds and cross-compiles like any Go package.
- **Never panics.** Every failure — including malformed, untrusted profiles — is
  returned as an `error`, never a panic. The profile, tag, and IT8 parsers are
  fuzz-tested for this.
- **Faithful to lcms2 2.19.** Algorithms, tables, and rounding are ported from
  the reference (pinned at commit `e2c0840`), which is also used as a
  differential-testing oracle. Integer transform and profile-I/O paths are
  bit-exact; float paths match within the reference testbed's tolerances.

## Status

Feature-complete against the lcms2 **core** public API: profile I/O for every
ICC tag type (with byte-identical serialization and MD5 profile IDs), tone
curves, interpolation, pipelines, the transform engine (all intents, optimized
fast paths, soft-proofing, gamut check, alpha), virtual profiles, CIECAM02,
IT8.7/CGATS, PostScript CSA/CRD generation, and the `.cube` device-link reader.

The optional lcms2 **plugins** (`fast_float`, `threaded`, GPU) are **GPL-3.0**,
a different licence from the MIT core, and are deliberately **not** ported so
this library can stay MIT. The built-in optimizer already provides the
matrix-shaper and CLUT fast paths, and transforms are safe to drive from
multiple goroutines if you need parallelism.

## Usage

### Transform pixels between two profiles

```go
package main

import (
	"fmt"

	lcms2 "github.com/mgilbir/golittlecms"
)

func main() {
	in, err := lcms2.Create_sRGBProfile()
	if err != nil {
		panic(err)
	}
	out, err := lcms2.OpenProfileFromFile("USWebCoatedSWOP.icc", "r")
	if err != nil {
		panic(err)
	}

	xform, err := lcms2.CreateTransform(
		in, lcms2.TypeRGB8,
		out, lcms2.TypeCMYK8,
		lcms2.IntentPerceptual, 0,
	)
	if err != nil {
		panic(err)
	}

	rgb := []byte{255, 128, 0} // one pixel
	cmyk := make([]byte, 4)
	xform.DoTransform(rgb, cmyk, 1)
	fmt.Println(cmyk)
}
```

A `*Transform` is safe for concurrent `DoTransform` calls, so you can split a
large image across goroutines. Profiles are opened from a file
(`OpenProfileFromFile`), from memory (`OpenProfileFromMem`), or created as
virtual profiles (`Create_sRGBProfile`, `CreateLab4Profile`, `CreateXYZProfile`,
`CreateGrayProfile`, …).

### Pixel formats

Refer to a format by name, or compose one with the `*SH` builders (same encoding
as the C `TYPE_*` macros and `*_SH` bitfields):

```go
lcms2.TypeRGB8    // 8-bit RGB
lcms2.TypeRGBA8   // 8-bit RGB + alpha
lcms2.TypeCMYK16  // 16-bit CMYK
lcms2.TypeLab16   // 16-bit CIELab
lcms2.TypeRGBFlt  // 32-bit float RGB

// Custom: 16-bit, 3-channel RGB with one extra (alpha) channel
custom := lcms2.ColorspaceSH(lcms2.PTRGB) | lcms2.ExtraSH(1) |
	lcms2.ChannelsSH(3) | lcms2.BytesSH(2)
```

Use `lcms2.FlagsCopyAlpha` to carry extra channels through, and
`lcms2.FlagsBlackPointCompensation`, `lcms2.FlagsSoftProofing`,
`lcms2.FlagsGamutCheck`, etc. as the transform flags argument.

### Errors

All fallible calls return a plain `error`. Recover the library's error class
with `errors.As`:

```go
_, err := lcms2.OpenProfileFromMem(data)
var e *lcms2.Error
if errors.As(err, &e) && e.Code == lcms2.ErrBadSignature {
	// not an ICC profile
}
```

More runnable examples live alongside the code as Go `Example` functions (see
`go doc`).

## Testing

`go test ./...` runs the unit tests and replays committed golden vectors, so it
passes without any C toolchain. For differential testing against the reference,
build the oracle harness once:

```sh
scripts/build-oracle.sh   # fetches the pinned lcms2 and builds bin/lcms2_oracle
go test ./...             # differential tests now run live; they skip if absent
```

### Fuzzing

The fuzz targets cover the parsers of untrusted input and the public entry
points that take adversarial scalar arguments:

- **Profiles** — `FuzzOpenProfileFromMem` (header, tag table, raw tag reads).
- **Tag-type decoders** — `FuzzAllTagTypeDecoders` drives all 36 registered
  built-in decoders generically, and eleven of them also have dedicated targets
  with type-specific seeds: `FuzzTypeCurveRead`, `FuzzTypeMLURead`,
  `FuzzTypeTextDescriptionRead`, `FuzzTypeNamedColor2Read`, `FuzzDictRead`,
  `FuzzTypeLUT8Read`, `FuzzTypeLUT16Read`, `FuzzTypeLUTA2BRead`,
  `FuzzTypeLUTB2ARead`, `FuzzTypeMPERead`, `FuzzPipelineCLut`.
- **Other file formats** — `FuzzIT8LoadFromMem` (IT8/CGATS), `FuzzParseCube`
  (`.cube` LUTs), `FuzzGDBSectors` (gamut-boundary descriptors).
- **Transforms and pixel formats** — `FuzzTransformFromProfiles` (builds a
  transform from two fuzzed profiles), `FuzzDoTransform`,
  `FuzzOptimizedDoTransform`, `FuzzFormatters`, `FuzzGetPostScriptCSA`.
- **Constructors** — `FuzzToneCurveConstructors`, `FuzzStageConstructors`
  (adversarial entry counts, segment parameters, channel counts, grid sizes).

The invariant every target checks is the same: no panic and no unbounded
allocation, however malformed the input — failures must come back as an
`*lcms2.Error`.

`go test -fuzz` only accepts a pattern that matches exactly one target, so name
one at a time:

```sh
go test -run '^$' -fuzz '^FuzzOpenProfileFromMem$' -fuzztime 60s
```

To sweep them all, loop over the list:

```sh
for t in $(go test -list '^Fuzz' ./... | grep '^Fuzz'); do
  go test -run '^$' -fuzz "^$t\$" -fuzztime 60s || break
done
```

Inputs that fail are written to `testdata/fuzz/<Target>/` and are committed as
regression cases; plain `go test ./...` replays them as ordinary seeds.

## Licence

MIT. This is a derivative work of Little-CMS and retains its copyright notice;
see [LICENSE](LICENSE). The GPL-3.0 lcms2 plugins are not included.
