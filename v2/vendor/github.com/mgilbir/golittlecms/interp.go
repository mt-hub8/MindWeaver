package lcms2

// Port of the interpolation engine from src/cmsintrp.c (lcms2 2.19).
//
// This module implements forward interpolation for 1..15 input channels and up
// to MAX_STAGE_CHANNELS output channels, in both 16-bit fixed-point and float32
// flavours, plus the plug-in machinery (cmsPluginInterpolation) that lets an
// external factory override the built-in interpolator selection.
//
// Parity rules honoured here:
//
//   - The 16-bit interpolators use integer and 15.16 fixed-point arithmetic
//     exclusively and are BIT-EXACT with the reference C build. Go's signed
//     integer overflow is defined as two's-complement wraparound, matching the
//     behaviour the C code relies on (the tetrahedral rounding trick multiplies
//     values that overflow int32 and depends on the wraparound), so the int32
//     arithmetic reproduces the C result exactly.
//
//   - The float interpolators use float32 arithmetic in the exact operation
//     order of the C source. Products that feed an add are forced through a
//     float32 temporary (or an explicit float32 conversion) so the Go compiler
//     cannot fuse them into an FMA; the oracle is built for a baseline x86-64
//     target without FMA, so each multiply and add rounds separately. Targeting
//     bit-exact parity.
//
// _cmsFreeInterpParams is intentionally not ported: it only calls _cmsFree on
// the params block, which the Go garbage collector reclaims. Nothing else in
// the C routine has an effect under GC.

import "math"

// Interpolation selection flags, mirroring the CMS_LERP_FLAGS_* macros in
// include/lcms2_plugin.h.
const (
	cmsLerpFlags16Bits    = 0x0000 // the default: 16-bit fixed-point path
	cmsLerpFlagsFloat     = 0x0001 // requires the float32 implementation
	cmsLerpFlagsTrilinear = 0x0100 // hint: use trilinear instead of tetrahedral
)

// maxInputDimensions mirrors MAX_INPUT_DIMENSIONS (include/lcms2_plugin.h): the
// interpolation engine handles at most 15 input channels.
const maxInputDimensions = 15

// maxStageChannels mirrors MAX_STAGE_CHANNELS (src/lcms2_internal.h): the width
// of the scratch buffers used while recursing through N-input evaluators.
const maxStageChannels = 128

// InterpFn16 is the 16-bit forward interpolation callback, mirroring
// _cmsInterpFn16. It reads len==nInputs samples from input and writes
// len==nOutputs samples to output.
type InterpFn16 func(input, output []uint16, p *InterpParams)

// InterpFnFloat is the float32 forward interpolation callback, mirroring
// _cmsInterpFnFloat.
type InterpFnFloat func(input, output []float32, p *InterpParams)

// InterpFunction mirrors the cmsInterpFunction union: a holder for either a
// 16-bit or a float32 interpolator. Unlike the C union the two callbacks live
// in separate fields; the "which member is set" test in
// _cmsSetInterpolationRoutine is reproduced by checking both fields (see
// setInterpolationRoutine).
type InterpFunction struct {
	Lerp16    InterpFn16    // forward interpolation in 16 bits
	LerpFloat InterpFnFloat // forward interpolation in floating point
}

// InterpFnFactory mirrors cmsInterpFnFactory: given the channel counts and the
// selection flags it returns the interpolator to use, or a zero InterpFunction
// if the combination is unsupported.
type InterpFnFactory func(nInputChannels, nOutputChannels, dwFlags uint32) InterpFunction

// InterpParams mirrors cmsInterpParams (the public interpolation descriptor in
// include/lcms2_plugin.h). It precomputes everything the interpolators need:
// the per-dimension domain (nodes minus one) and the opta strides that index
// the flattened CLUT. The table is held as one of two typed views; exactly one
// of table16/tableFloat is set, matching the const void* Table union in C.
type InterpParams struct {
	ContextID *Context // the owning context

	Flags      uint32 // original flags (CMS_LERP_FLAGS_*)
	NumInputs  uint32 // number of input channels
	NumOutputs uint32 // number of output channels

	NumSamples [maxInputDimensions]uint32 // nodes per input direction
	Domain     [maxInputDimensions]uint32 // Domain = nSamples - 1
	Opta       [maxInputDimensions]uint32 // grid strides, premultiplied per dimension

	table16    []uint16  // 16-bit CLUT view (nil for float tables)
	tableFloat []float32 // float32 CLUT view (nil for 16-bit tables)

	Interpolation InterpFunction // the selected interpolator
}

// Table16 returns the 16-bit CLUT view, or nil for a float table.
func (p *InterpParams) Table16() []uint16 { return p.table16 }

// TableFloat returns the float32 CLUT view, or nil for a 16-bit table.
func (p *InterpParams) TableFloat() []float32 { return p.tableFloat }

// Eval16 runs the selected 16-bit interpolator; input has NumInputs samples and
// output receives NumOutputs samples.
func (p *InterpParams) Eval16(input, output []uint16) {
	p.Interpolation.Lerp16(input, output, p)
}

// EvalFloat runs the selected float32 interpolator.
func (p *InterpParams) EvalFloat(input, output []float32) {
	p.Interpolation.LerpFloat(input, output, p)
}

// PluginInterpolation mirrors cmsPluginInterpolation: a plug-in that replaces
// the built-in interpolator-selection factory. Register it through
// (*Context).RegisterPlugins; the newest registered factory wins and, when it
// returns an empty InterpFunction, the built-in factory is used as a fallback.
type PluginInterpolation struct {
	PluginBase
	Factory InterpFnFactory
}

// interpolatorsFactory returns the most recently registered interpolation
// factory, or nil when none is installed. It mirrors reading
// _cmsInterpPluginChunk.Interpolators: the C code keeps a single slot that the
// last registration overwrites, which the newest-first plug-in list reproduces
// by consulting entries[0].
func (ctx *Context) interpolatorsFactory() InterpFnFactory {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	for _, pl := range ctx.interp.entries {
		if ip, ok := pl.(*PluginInterpolation); ok {
			return ip.Factory
		}
	}
	return nil
}

// setInterpolationRoutine ports _cmsSetInterpolationRoutine. It first tries the
// plug-in factory (if any) and falls back to the built-in factory when the
// plug-in does not supply an interpolator. In C the "unset" test inspects the
// Lerp16 member of the union, which aliases LerpFloat, so a float interpolator
// also passes; here both fields are checked to reproduce that. Returns false
// when no interpolator matches the requested channel/flag combination.
func (ctx *Context) setInterpolationRoutine(p *InterpParams) bool {
	p.Interpolation = InterpFunction{}

	if f := ctx.interpolatorsFactory(); f != nil {
		p.Interpolation = f(p.NumInputs, p.NumOutputs, p.Flags)
	}

	if p.Interpolation.Lerp16 == nil && p.Interpolation.LerpFloat == nil {
		p.Interpolation = defaultInterpolatorsFactory(p.NumInputs, p.NumOutputs, p.Flags)
	}

	if p.Interpolation.Lerp16 == nil && p.Interpolation.LerpFloat == nil {
		return false
	}
	return true
}

// computeInterpParamsEx ports _cmsComputeInterpParamsEx. table must be a
// []uint16 or []float32 (matching the flags); nSamples must have at least
// inputChan entries. It precomputes the domain and opta strides and selects the
// interpolator. On any failure it signals the appropriate error and returns it.
func computeInterpParamsEx(ctx *Context, nSamples []uint32, inputChan, outputChan uint32, table any, dwFlags uint32) (*InterpParams, error) {
	if inputChan > maxInputDimensions {
		return nil, ctx.signalError(ErrRange,
			"Too many input channels (%d channels, max=%d)", inputChan, maxInputDimensions)
	}

	p := &InterpParams{
		ContextID:  ctx,
		Flags:      dwFlags,
		NumInputs:  inputChan,
		NumOutputs: outputChan,
	}

	switch t := table.(type) {
	case []uint16:
		p.table16 = t
	case []float32:
		p.tableFloat = t
	case nil:
		// A nil table is accepted (the reference stores a raw pointer); the
		// interpolators are simply not runnable until one is assigned.
	default:
		return nil, ctx.signalError(ErrInternal, "unsupported interpolation table type %T", table)
	}

	// Fill samples per input direction and the domain (nodes minus one).
	for i := uint32(0); i < inputChan; i++ {
		p.NumSamples[i] = nSamples[i]
		p.Domain[i] = nSamples[i] - 1
	}

	// Compute the strides used to index the flattened grid.
	p.Opta[0] = p.NumOutputs
	for i := uint32(1); i < inputChan; i++ {
		p.Opta[i] = p.Opta[i-1] * nSamples[inputChan-i]
	}

	if !ctx.setInterpolationRoutine(p) {
		return nil, ctx.signalError(ErrUnknownExtension,
			"Unsupported interpolation (%d->%d channels)", inputChan, outputChan)
	}

	return p, nil
}

// computeInterpParams ports _cmsComputeInterpParams: the uniform-grid wrapper
// that assumes every input direction has the same number of nodes.
func computeInterpParams(ctx *Context, nSamples, inputChan, outputChan uint32, table any, dwFlags uint32) (*InterpParams, error) {
	var samples [maxInputDimensions]uint32
	for i := range samples {
		samples[i] = nSamples
	}
	return computeInterpParamsEx(ctx, samples[:], inputChan, outputChan, table, dwFlags)
}

// linearInterp ports the inline fixed-point LinearInterp. a is the fractional
// weight in 0..0xFFFF, l and h are the two endpoints. The arithmetic is carried
// out in uint32 exactly as the C code does, including the wraparound for a
// negative (h-l).
func linearInterp(a, l, h int32) uint16 {
	dif := uint32(h-l)*uint32(a) + 0x8000
	dif = (dif >> 16) + uint32(l)
	return uint16(dif)
}

// lerp16 ports the LERP macro shared by BilinearInterp16 and TrilinearInterp16:
//
//	(cmsUInt16Number)(l + ROUND_FIXED_TO_INT((h-l)*a))
//
// The (h-l)*a product is int32 (wrapping on overflow), rounded to an integer,
// added to l, then truncated to 16 bits. The truncated value is returned as an
// int32 so it can feed the next LERP as an endpoint.
func lerp16(a, l, h int32) int32 {
	return int32(uint16(l + roundFixedToInt((h-l)*a)))
}

// fclamp ports the inline fclamp: clamps to [0,1], mapping sub-1e-9 values and
// NaN to 0 and values above 1 to 1.
func fclamp(v float32) float32 {
	if v < 1.0e-9 || v != v {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

// lerpFloat ports the float LERP macro: l + (h-l)*a. The product is rounded
// through an explicit float32 conversion (fmul32) before the add, which the
// compiler will not fuse across; this keeps the result bit-identical to the
// non-FMA C reference on FMA-capable targets. A plain local temporary does NOT
// prevent the fusion, so the conversion is required.
func lerpFloat(a, l, h float32) float32 {
	return l + fmul32(h-l, a)
}

// clampCell bounds a computed lower-cell index into [0, domain-1] so that both
// lut[cell] and lut[cell+1] stay in range for a table of domain+1 entries. It is
// a no-op whenever the caller's fixed-point arithmetic did not overflow (the
// only case the reference relies on); see linLerp1D for why that covers every
// tone curve of up to 32768 entries.
func clampCell(cell int32, domain uint32) int32 {
	if cell < 0 {
		return 0
	}
	if domain > 0 && cell > int32(domain-1) {
		return int32(domain - 1)
	}
	return cell
}

// LinLerp1D ports LinLerp1D: linear interpolation, 1 input -> 1 output, 16-bit.
func linLerp1D(value, output []uint16, p *InterpParams) {
	lut := p.table16

	if value[0] == 0xffff || p.Domain[0] == 0 {
		output[0] = lut[p.Domain[0]]
		return
	}

	val3 := int(p.Domain[0]) * int(value[0])
	fx := toFixedDomain(val3)

	cell0 := fixedToInt(fx)
	rest := fixedRestToInt(fx)

	// For nEntries <= 32768 (Domain[0] <= 32767) the product above stays below
	// 2^31 and cell0 is always in [0, Domain[0]-1], so this clamp is a no-op and
	// the fast path stays bit-exact with the reference. Above that size the
	// 15.16 fixed-point form overflows int32 (as it does in C, which reads out
	// of bounds under CMS_NO_SANITIZE); clamping keeps us panic-free instead.
	cell0 = clampCell(cell0, p.Domain[0])

	y0 := lut[cell0]
	y1 := lut[cell0+1]

	output[0] = linearInterp(rest, int32(y0), int32(y1))
}

// linLerp1Dfloat ports LinLerp1Dfloat.
func linLerp1Dfloat(value, output []float32, p *InterpParams) {
	lut := p.tableFloat

	val2 := fclamp(value[0])

	if val2 == 1.0 || p.Domain[0] == 0 {
		output[0] = lut[p.Domain[0]]
		return
	}

	val2 *= float32(p.Domain[0])

	cell0 := int(math.Floor(float64(val2)))
	cell1 := int(math.Ceil(float64(val2)))

	rest := val2 - float32(cell0)

	y0 := lut[cell0]
	y1 := lut[cell1]

	output[0] = y0 + fmul32(y1-y0, rest)
}

// eval1Input ports Eval1Input: 1 input -> N outputs, 16-bit.
func eval1Input(input, output []uint16, p *InterpParams) {
	lut := p.table16

	if input[0] == 0xffff || p.Domain[0] == 0 {
		y0 := p.Domain[0] * p.Opta[0]
		for oc := uint32(0); oc < p.NumOutputs; oc++ {
			output[oc] = lut[y0+oc]
		}
		return
	}

	v := int(input[0]) * int(p.Domain[0])
	fk := toFixedDomain(v)

	k0 := fixedToInt(fk)
	rk := fixedRestToInt(fk)

	// See linLerp1D: a no-op for the well-defined (<=32768-entry) domain, a
	// panic guard for the int32-overflow region C leaves unchecked.
	k0 = clampCell(k0, p.Domain[0])

	var k1 int32 = k0
	if input[0] != 0xffff {
		k1 = k0 + 1
	}

	K0 := int32(p.Opta[0]) * k0
	K1 := int32(p.Opta[0]) * k1

	for oc := int32(0); oc < int32(p.NumOutputs); oc++ {
		output[oc] = linearInterp(rk, int32(lut[K0+oc]), int32(lut[K1+oc]))
	}
}

// eval1InputFloat ports Eval1InputFloat.
func eval1InputFloat(value, output []float32, p *InterpParams) {
	lut := p.tableFloat

	val2 := fclamp(value[0])

	if val2 == 1.0 || p.Domain[0] == 0 {
		start := p.Domain[0] * p.Opta[0]
		for oc := uint32(0); oc < p.NumOutputs; oc++ {
			output[oc] = lut[start+oc]
		}
		return
	}

	val2 *= float32(p.Domain[0])

	cell0 := int(math.Floor(float64(val2)))
	cell1 := int(math.Ceil(float64(val2)))

	rest := val2 - float32(cell0)

	cell0 *= int(p.Opta[0])
	cell1 *= int(p.Opta[0])

	for oc := 0; oc < int(p.NumOutputs); oc++ {
		y0 := lut[cell0+oc]
		y1 := lut[cell1+oc]
		output[oc] = y0 + fmul32(y1-y0, rest)
	}
}

// bilinearInterp16 ports BilinearInterp16: 2 inputs, 16-bit.
func bilinearInterp16(input, output []uint16, p *InterpParams) {
	lut := p.table16
	totalOut := int32(p.NumOutputs)

	fx := toFixedDomain(int(input[0]) * int(p.Domain[0]))
	x0 := fixedToInt(fx)
	rx := fixedRestToInt(fx)

	fy := toFixedDomain(int(input[1]) * int(p.Domain[1]))
	y0 := fixedToInt(fy)
	ry := fixedRestToInt(fy)

	X0 := int32(p.Opta[1]) * x0
	X1 := X0
	if input[0] != 0xffff {
		X1 += int32(p.Opta[1])
	}

	Y0 := int32(p.Opta[0]) * y0
	Y1 := Y0
	if input[1] != 0xffff {
		Y1 += int32(p.Opta[0])
	}

	for oc := int32(0); oc < totalOut; oc++ {
		d00 := int32(lut[X0+Y0+oc])
		d01 := int32(lut[X0+Y1+oc])
		d10 := int32(lut[X1+Y0+oc])
		d11 := int32(lut[X1+Y1+oc])

		dx0 := lerp16(rx, d00, d10)
		dx1 := lerp16(rx, d01, d11)

		dxy := lerp16(ry, dx0, dx1)

		output[oc] = uint16(dxy)
	}
}

// bilinearInterpFloat ports BilinearInterpFloat. It uses _cmsQuickFloor, like
// the C source (trilinear/tetrahedral use full floor instead).
func bilinearInterpFloat(input, output []float32, p *InterpParams) {
	lut := p.tableFloat
	totalOut := int(p.NumOutputs)

	px := fclamp(input[0]) * float32(p.Domain[0])
	py := fclamp(input[1]) * float32(p.Domain[1])

	x0 := quickFloor(float64(px))
	fx := px - float32(x0)
	y0 := quickFloor(float64(py))
	fy := py - float32(y0)

	X0 := int(p.Opta[1]) * x0
	X1 := X0
	if !(fclamp(input[0]) >= 1.0) {
		X1 += int(p.Opta[1])
	}

	Y0 := int(p.Opta[0]) * y0
	Y1 := Y0
	if !(fclamp(input[1]) >= 1.0) {
		Y1 += int(p.Opta[0])
	}

	for oc := 0; oc < totalOut; oc++ {
		d00 := lut[X0+Y0+oc]
		d01 := lut[X0+Y1+oc]
		d10 := lut[X1+Y0+oc]
		d11 := lut[X1+Y1+oc]

		dx0 := lerpFloat(fx, d00, d10)
		dx1 := lerpFloat(fx, d01, d11)

		dxy := lerpFloat(fy, dx0, dx1)

		output[oc] = dxy
	}
}

// trilinearInterp16 ports TrilinearInterp16: 3 inputs, 16-bit, trilinear.
func trilinearInterp16(input, output []uint16, p *InterpParams) {
	lut := p.table16
	totalOut := int32(p.NumOutputs)

	fx := toFixedDomain(int(input[0]) * int(p.Domain[0]))
	x0 := fixedToInt(fx)
	rx := fixedRestToInt(fx)

	fy := toFixedDomain(int(input[1]) * int(p.Domain[1]))
	y0 := fixedToInt(fy)
	ry := fixedRestToInt(fy)

	fz := toFixedDomain(int(input[2]) * int(p.Domain[2]))
	z0 := fixedToInt(fz)
	rz := fixedRestToInt(fz)

	X0 := int32(p.Opta[2]) * x0
	X1 := X0
	if input[0] != 0xffff {
		X1 += int32(p.Opta[2])
	}

	Y0 := int32(p.Opta[1]) * y0
	Y1 := Y0
	if input[1] != 0xffff {
		Y1 += int32(p.Opta[1])
	}

	Z0 := int32(p.Opta[0]) * z0
	Z1 := Z0
	if input[2] != 0xffff {
		Z1 += int32(p.Opta[0])
	}

	for oc := int32(0); oc < totalOut; oc++ {
		d000 := int32(lut[X0+Y0+Z0+oc])
		d001 := int32(lut[X0+Y0+Z1+oc])
		d010 := int32(lut[X0+Y1+Z0+oc])
		d011 := int32(lut[X0+Y1+Z1+oc])
		d100 := int32(lut[X1+Y0+Z0+oc])
		d101 := int32(lut[X1+Y0+Z1+oc])
		d110 := int32(lut[X1+Y1+Z0+oc])
		d111 := int32(lut[X1+Y1+Z1+oc])

		dx00 := lerp16(rx, d000, d100)
		dx01 := lerp16(rx, d001, d101)
		dx10 := lerp16(rx, d010, d110)
		dx11 := lerp16(rx, d011, d111)

		dxy0 := lerp16(ry, dx00, dx10)
		dxy1 := lerp16(ry, dx01, dx11)

		dxyz := lerp16(rz, dxy0, dxy1)

		output[oc] = uint16(dxyz)
	}
}

// trilinearInterpFloat ports TrilinearInterpFloat. Uses full floor().
func trilinearInterpFloat(input, output []float32, p *InterpParams) {
	lut := p.tableFloat
	totalOut := int(p.NumOutputs)

	px := fclamp(input[0]) * float32(p.Domain[0])
	py := fclamp(input[1]) * float32(p.Domain[1])
	pz := fclamp(input[2]) * float32(p.Domain[2])

	x0 := int(math.Floor(float64(px)))
	fx := px - float32(x0)
	y0 := int(math.Floor(float64(py)))
	fy := py - float32(y0)
	z0 := int(math.Floor(float64(pz)))
	fz := pz - float32(z0)

	X0 := int(p.Opta[2]) * x0
	X1 := X0
	if !(fclamp(input[0]) >= 1.0) {
		X1 += int(p.Opta[2])
	}

	Y0 := int(p.Opta[1]) * y0
	Y1 := Y0
	if !(fclamp(input[1]) >= 1.0) {
		Y1 += int(p.Opta[1])
	}

	Z0 := int(p.Opta[0]) * z0
	Z1 := Z0
	if !(fclamp(input[2]) >= 1.0) {
		Z1 += int(p.Opta[0])
	}

	for oc := 0; oc < totalOut; oc++ {
		d000 := lut[X0+Y0+Z0+oc]
		d001 := lut[X0+Y0+Z1+oc]
		d010 := lut[X0+Y1+Z0+oc]
		d011 := lut[X0+Y1+Z1+oc]
		d100 := lut[X1+Y0+Z0+oc]
		d101 := lut[X1+Y0+Z1+oc]
		d110 := lut[X1+Y1+Z0+oc]
		d111 := lut[X1+Y1+Z1+oc]

		dx00 := lerpFloat(fx, d000, d100)
		dx01 := lerpFloat(fx, d001, d101)
		dx10 := lerpFloat(fx, d010, d110)
		dx11 := lerpFloat(fx, d011, d111)

		dxy0 := lerpFloat(fy, dx00, dx10)
		dxy1 := lerpFloat(fy, dx01, dx11)

		dxyz := lerpFloat(fz, dxy0, dxy1)

		output[oc] = dxyz
	}
}

// tetra16At runs the Sakamoto tetrahedral interpolation over the three input
// dimensions j, j+1, j+2 of a 16-bit table, reading from base (a flat offset
// into the CLUT). It ports the optimised pointer-walk TetrahedralInterp16: the
// three sub-cube dimensions always use the lowest three strides opta[2],
// opta[1], opta[0], which is what both the stand-alone 3D case and every
// N-input recursion base require. The +0x8001 / (Rest+(Rest>>16))>>16 rounding
// matches the C source (off-by-one at 0x7fff and 0x17ffe by design).
func (p *InterpParams) tetra16At(base int32, j uint32, input, output []uint16) {
	lut := p.table16
	total := int(p.NumOutputs)

	fx := toFixedDomain(int(input[0]) * int(p.Domain[j]))
	fy := toFixedDomain(int(input[1]) * int(p.Domain[j+1]))
	fz := toFixedDomain(int(input[2]) * int(p.Domain[j+2]))

	x0 := fixedToInt(fx)
	y0 := fixedToInt(fy)
	z0 := fixedToInt(fz)

	rx := fixedRestToInt(fx)
	ry := fixedRestToInt(fy)
	rz := fixedRestToInt(fz)

	X0 := int32(p.Opta[2]) * x0
	Y0 := int32(p.Opta[1]) * y0
	Z0 := int32(p.Opta[0]) * z0

	var X1, Y1, Z1 int32
	if input[0] != 0xffff {
		X1 = int32(p.Opta[2])
	}
	if input[1] != 0xffff {
		Y1 = int32(p.Opta[1])
	}
	if input[2] != 0xffff {
		Z1 = int32(p.Opta[0])
	}

	off := int(base + X0 + Y0 + Z0)

	if rx >= ry {
		if ry >= rz {
			Y1 += X1
			Z1 += Y1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c3 := c - b
				c2 := b - a
				c1 := a - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		} else if rz >= rx {
			X1 += Z1
			Y1 += X1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c2 := b - a
				c1 := a - c
				c3 := c - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		} else {
			Z1 += X1
			Y1 += Z1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c2 := b - c
				c3 := c - a
				c1 := a - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		}
	} else {
		if rx >= rz {
			X1 += Y1
			Z1 += X1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c3 := c - a
				c1 := a - b
				c2 := b - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		} else if ry >= rz {
			Z1 += Y1
			X1 += Z1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c1 := a - c
				c3 := c - b
				c2 := b - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		} else {
			Y1 += Z1
			X1 += Y1
			for i := 0; i < total; i++ {
				a := int32(lut[off+int(X1)])
				b := int32(lut[off+int(Y1)])
				c := int32(lut[off+int(Z1)])
				c0 := int32(lut[off])
				off++
				c1 := a - b
				c2 := b - c
				c3 := c - c0
				rest := c1*rx + c2*ry + c3*rz + 0x8001
				output[i] = uint16(c0 + ((rest + (rest >> 16)) >> 16))
			}
		}
	}
}

// tetrahedralInterp16 ports the stand-alone 3-input TetrahedralInterp16.
func tetrahedralInterp16(input, output []uint16, p *InterpParams) {
	p.tetra16At(0, 0, input, output)
}

// tetraFloatAt runs the float32 tetrahedral interpolation over dimensions j,
// j+1, j+2 of a float table at flat offset base, porting TetrahedralInterpFloat
// (which uses full floor()). Like tetra16At, the sub-cube always uses the three
// lowest strides opta[2], opta[1], opta[0]. Products feeding the final sum go
// through float32 temporaries to prevent FMA contraction.
func (p *InterpParams) tetraFloatAt(base int, j uint32, input, output []float32) {
	lut := p.tableFloat
	total := int(p.NumOutputs)

	px := fclamp(input[0]) * float32(p.Domain[j])
	py := fclamp(input[1]) * float32(p.Domain[j+1])
	pz := fclamp(input[2]) * float32(p.Domain[j+2])

	x0 := int(math.Floor(float64(px)))
	rx := px - float32(x0)
	y0 := int(math.Floor(float64(py)))
	ry := py - float32(y0)
	z0 := int(math.Floor(float64(pz)))
	rz := pz - float32(z0)

	X0 := int(p.Opta[2]) * x0
	Y0 := int(p.Opta[1]) * y0
	Z0 := int(p.Opta[0]) * z0

	var X1, Y1, Z1 int
	if !(fclamp(input[0]) >= 1.0) {
		X1 = int(p.Opta[2])
	}
	if !(fclamp(input[1]) >= 1.0) {
		Y1 = int(p.Opta[1])
	}
	if !(fclamp(input[2]) >= 1.0) {
		Z1 = int(p.Opta[0])
	}

	b := base + X0 + Y0 + Z0

	for oc := 0; oc < total; oc++ {
		d000 := lut[b+0+0+0+oc]
		d001 := lut[b+0+0+Z1+oc]
		d010 := lut[b+0+Y1+0+oc]
		d011 := lut[b+0+Y1+Z1+oc]
		d100 := lut[b+X1+0+0+oc]
		d101 := lut[b+X1+0+Z1+oc]
		d110 := lut[b+X1+Y1+0+oc]
		d111 := lut[b+X1+Y1+Z1+oc]

		c0 := d000
		var c1, c2, c3 float32

		if rx >= ry && ry >= rz {
			c1 = d100 - c0
			c2 = d110 - d100
			c3 = d111 - d110
		} else if rx >= rz && rz >= ry {
			c1 = d100 - c0
			c2 = d111 - d101
			c3 = d101 - d100
		} else if rz >= rx && rx >= ry {
			c1 = d101 - d001
			c2 = d111 - d101
			c3 = d001 - c0
		} else if ry >= rx && rx >= rz {
			c1 = d110 - d010
			c2 = d010 - c0
			c3 = d111 - d110
		} else if ry >= rz && rz >= rx {
			c1 = d111 - d011
			c2 = d010 - c0
			c3 = d011 - d010
		} else if rz >= ry && ry >= rx {
			c1 = d111 - d011
			c2 = d011 - d001
			c3 = d001 - c0
		} else {
			c1, c2, c3 = 0, 0, 0
		}

		p0 := c1 * rx
		p1 := c2 * ry
		p2 := c3 * rz
		output[oc] = c0 + p0 + p1 + p2
	}
}

// tetrahedralInterpFloat ports the stand-alone 3-input TetrahedralInterpFloat.
func tetrahedralInterpFloat(input, output []float32, p *InterpParams) {
	p.tetraFloatAt(0, 0, input, output)
}

// evalSplit16 evaluates an n-input 16-bit interpolation by splitting dimension
// j into two sub-interpolations and linearly blending them, recursing until
// three dimensions remain (handled by tetra16At). It ports the Eval4Inputs /
// EVAL_FNS(N,NM) family without copying the params struct: dimension j uses
// Domain[j] and stride opta[n-1-j], and the recursion carries the accumulated
// table offset in base. This is the same decomposition the C macros perform
// (split the outermost channel, tetrahedral on the innermost three), and it is
// bit-exact with them.
func evalSplit16(p *InterpParams, n, j uint32, base int32, input, output []uint16) {
	if n-j == 3 {
		p.tetra16At(base, j, input, output)
		return
	}

	opt := int32(p.Opta[n-1-j])

	fk := toFixedDomain(int(input[0]) * int(p.Domain[j]))
	k0 := fixedToInt(fk)
	rk := fixedRestToInt(fk)

	var k1 int32 = k0
	if input[0] != 0xffff {
		k1 = k0 + 1
	}

	K0 := base + opt*k0
	K1 := base + opt*k1

	var tmp1, tmp2 [maxStageChannels]uint16
	evalSplit16(p, n, j+1, K0, input[1:], tmp1[:])
	evalSplit16(p, n, j+1, K1, input[1:], tmp2[:])

	for i := uint32(0); i < p.NumOutputs; i++ {
		output[i] = linearInterp(rk, int32(tmp1[i]), int32(tmp2[i]))
	}
}

// evalSplitFloat is the float32 counterpart of evalSplit16, porting
// Eval4InputsFloat / the float half of EVAL_FNS. The split levels use
// _cmsQuickFloor (matching the C source), while the tetrahedral base uses full
// floor via tetraFloatAt.
func evalSplitFloat(p *InterpParams, n, j uint32, base int, input, output []float32) {
	if n-j == 3 {
		p.tetraFloatAt(base, j, input, output)
		return
	}

	opt := int(p.Opta[n-1-j])

	pk := fclamp(input[0]) * float32(p.Domain[j])
	k0 := quickFloor(float64(pk))
	rest := pk - float32(k0)

	K0 := base + opt*k0
	K1 := K0
	if !(fclamp(input[0]) >= 1.0) {
		K1 += opt
	}

	var tmp1, tmp2 [maxStageChannels]float32
	evalSplitFloat(p, n, j+1, K0, input[1:], tmp1[:])
	evalSplitFloat(p, n, j+1, K1, input[1:], tmp2[:])

	for i := uint32(0); i < p.NumOutputs; i++ {
		d := (tmp2[i] - tmp1[i]) * rest
		output[i] = tmp1[i] + d
	}
}

// evalNInputs16 dispatches the 16-bit N-input (N>=4) recursion. It stands in
// for Eval4Inputs and Eval5Inputs..Eval15Inputs, which share one decomposition.
func evalNInputs16(input, output []uint16, p *InterpParams) {
	evalSplit16(p, p.NumInputs, 0, 0, input, output)
}

// evalNInputsFloat dispatches the float32 N-input (N>=4) recursion.
func evalNInputsFloat(input, output []float32, p *InterpParams) {
	evalSplitFloat(p, p.NumInputs, 0, 0, input, output)
}

// defaultInterpolatorsFactory ports DefaultInterpolatorsFactory: the built-in
// selection matrix over channel counts and flags. It returns a zero
// InterpFunction for unsupported combinations (matching the C memset/NULL
// defaults), including the nInputs>=4 && nOutputs>=MAX_STAGE_CHANNELS guard.
func defaultInterpolatorsFactory(nInputChannels, nOutputChannels, dwFlags uint32) InterpFunction {
	var it InterpFunction

	isFloat := dwFlags&cmsLerpFlagsFloat != 0
	isTrilinear := dwFlags&cmsLerpFlagsTrilinear != 0

	// Safety check.
	if nInputChannels >= 4 && nOutputChannels >= maxStageChannels {
		return it
	}

	switch nInputChannels {

	case 1: // Gray LUT / linear
		if nOutputChannels == 1 {
			if isFloat {
				it.LerpFloat = linLerp1Dfloat
			} else {
				it.Lerp16 = linLerp1D
			}
		} else {
			if isFloat {
				it.LerpFloat = eval1InputFloat
			} else {
				it.Lerp16 = eval1Input
			}
		}

	case 2: // Duotone
		if isFloat {
			it.LerpFloat = bilinearInterpFloat
		} else {
			it.Lerp16 = bilinearInterp16
		}

	case 3: // RGB et al
		if isTrilinear {
			if isFloat {
				it.LerpFloat = trilinearInterpFloat
			} else {
				it.Lerp16 = trilinearInterp16
			}
		} else {
			if isFloat {
				it.LerpFloat = tetrahedralInterpFloat
			} else {
				it.Lerp16 = tetrahedralInterp16
			}
		}

	case 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15:
		// CMYK and 5..15 inks: one recursive decomposition serves them all.
		if isFloat {
			it.LerpFloat = evalNInputsFloat
		} else {
			it.Lerp16 = evalNInputs16
		}

	default:
		// Leaves it zero (Lerp16 == nil), i.e. unsupported.
	}

	return it
}
