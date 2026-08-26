package lcms2

// This file ports the tone-curve engine from src/cmsgamma.c (lcms2 2.19):
// tabulated 16-bit curves, floating-point sampled curves, segmented curves,
// the built-in parametric function family (types +/-1..8, 108, 109) and the
// parametric-curve plug-in mechanism, plus curve inversion, joining, smoothing,
// gamma estimation and the linearity/monotonicity predicates.
//
// Interpolation seam (interp.go, W5)
// ----------------------------------
// Every tone curve owns a cmsInterpParams computed by _cmsComputeInterpParams,
// exactly as the reference's AllocateToneCurveStruct does. cmsEvalToneCurve16
// dispatches through InterpParams.Interpolation.Lerp16 (the 1->1 fixed-point
// interpolator LinLerp1D in cmsintrp.c) and each sampled segment (Type==0) owns
// a second cmsInterpParams whose LerpFloat is LinLerp1Dfloat, driven by
// EvalSegmentedFn:
//   - ToneCurve.interp        <- computeInterpParams(nEntries,1,1,Table16,16BITS)
//   - ToneCurve.segInterp[i]  <- computeInterpParams(nGridPoints,1,1,pts,FLOAT)
// The interpolation math lives entirely in interp.go (W5); this file only wires
// the params and dispatches through the selected InterpFunction.
//
// Intentional deviations from the C reference (Go runtime model, not behaviour):
//   - Memory management (cmsFreeToneCurve, cmsFreeToneCurveTriple, the goto
//     Error cleanup in AllocateToneCurveStruct) is owned by the Go garbage
//     collector; Free is a documented no-op kept only for API symmetry.
//   - computeInterpParams never fails for a 1->1 curve, but its error is still
//     propagated for completeness.
//   - cmsBuildTabulatedToneCurveFloat returns a *Error where C silently returns
//     NULL for nEntries==0/NULL values; the primary error channel replaces the
//     NULL sentinel per the project's error convention.

import "math"

// maxNodesInCurve mirrors MAX_NODES_IN_CURVE.
const maxNodesInCurve = 4097

// maxTypesInLCMSPlugin mirrors MAX_TYPES_IN_LCMS_PLUGIN (lcms2_plugin.h).
const maxTypesInLCMSPlugin = 20

// curveMinusInf / curvePlusInf mirror MINUS_INF / PLUS_INF, which the reference
// defines as the float32 literals -1E22F / +1E22F. Widened to float64 they keep
// the exact float32-rounded magnitude the C code observes.
const (
	curveMinusInf float32 = -1e22
	curvePlusInf  float32 = 1e22
)

var (
	minusInf = float64(curveMinusInf)
	plusInf  = float64(curvePlusInf)
)

// CurveSegment mirrors cmsCurveSegment: one segment of a segmented tone curve.
// A segment applies for x0 < x <= x1. Type==0 marks a sampled segment
// (SampledPoints/NGridPoints); any other Type selects a parametric function
// evaluated with Params.
type CurveSegment struct {
	X0, X1        float32     // Domain; for x0 < x <= x1
	Type          int32       // Parametric type; 0 means sampled, negatives reserved
	Params        [10]float64 // Parameters if Type != 0
	NGridPoints   uint32      // Number of grid points if Type == 0
	SampledPoints []float32   // Sample array if Type == 0
}

// ToneCurve mirrors cmsToneCurve (struct _cms_curve_struct). It keeps a
// limited-precision 16-bit table (Table16) for fast 8/16-bit transforms and,
// for segmented/parametric curves, the segment description used by the
// higher-precision floating-point evaluator.
type ToneCurve struct {
	ctx *Context

	nSegments uint32
	segments  []CurveSegment
	segInterp []*InterpParams       // per sampled segment (Type==0): _cmsComputeInterpParams(nGridPoints,1,1,pts,FLOAT)
	evals     []parametricEvaluator // one evaluator per segment (nil for sampled)

	nEntries uint32
	table16  []uint16

	interp *InterpParams // the whole-curve lookup: _cmsComputeInterpParams(nEntries,1,1,Table16,16BITS)
}

// parametricEvaluator mirrors cmsParametricCurveEvaluator: evaluate parametric
// function Type at R with the given parameters. A negative Type asks for the
// analytic inverse.
type parametricEvaluator func(typ int32, params []float64, r float64) float64

// PluginParametricCurves mirrors cmsPluginParametricCurves: a plug-in that adds
// one or more parametric function types, each with a fixed parameter count, all
// served by a single Evaluator. Register it through (*Context).RegisterPlugins;
// the plug-in header's Type must be the parametric-curve signature.
type PluginParametricCurves struct {
	PluginBase
	NFunctions     uint32
	FunctionTypes  []int32
	ParameterCount []uint32
	Evaluator      parametricEvaluator
}

// defaultCurveTypes / defaultCurveParamCounts mirror the built-in DefaultCurves
// collection: 10 function types with their parameter counts.
var (
	defaultCurveTypes       = [...]int32{1, 2, 3, 4, 5, 6, 7, 8, 108, 109}
	defaultCurveParamCounts = [...]uint32{1, 3, 4, 5, 7, 4, 5, 5, 1, 1}
)

// getParametricCurveByType mirrors GetParametricCurveByType: search the
// registered parametric-curve plug-ins (newest first), then the built-in
// defaults, for a collection that supports abs(typ). It returns that
// collection's evaluator and the matched type's parameter count.
func (ctx *Context) getParametricCurveByType(typ int32) (parametricEvaluator, uint32, bool) {
	ctx.mu.Lock()
	entries := ctx.parametricCurves.entries
	ctx.mu.Unlock()

	want := abs32(typ)
	for _, pl := range entries {
		pc, ok := pl.(*PluginParametricCurves)
		if !ok {
			continue
		}
		n := pc.NFunctions
		if n > maxTypesInLCMSPlugin {
			n = maxTypesInLCMSPlugin
		}
		if int(n) > len(pc.FunctionTypes) {
			n = uint32(len(pc.FunctionTypes))
		}
		for i := uint32(0); i < n; i++ {
			if want == pc.FunctionTypes[i] {
				var pcount uint32
				if int(i) < len(pc.ParameterCount) {
					pcount = pc.ParameterCount[i]
				}
				return pc.Evaluator, pcount, true
			}
		}
	}
	for i, ft := range defaultCurveTypes {
		if want == ft {
			return defaultEvalParametricFn, defaultCurveParamCounts[i], true
		}
	}
	return nil, 0, false
}

// -------------------------------------------------- parametric evaluators

func sigmoidBase(k, t float64) float64 {
	return 1.0/(1.0+math.Exp(-k*t)) - 0.5
}

func invertedSigmoidBase(k, t float64) float64 {
	return -math.Log((1.0/(t+0.5))-1.0) / k
}

func sigmoidFactory(k, t float64) float64 {
	correction := 0.5 / sigmoidBase(k, 1)
	return fmul64(correction, sigmoidBase(k, fmul64(2.0, t)-1.0)) + 0.5
}

func inverseSigmoidFactory(k, t float64) float64 {
	correction := 0.5 / sigmoidBase(k, 1)
	return (invertedSigmoidBase(k, (t-0.5)/correction) + 1.0) / 2.0
}

// defaultEvalParametricFn ports DefaultEvalParametricFn: the built-in evaluator
// for parametric types +/-1..8, 108, 109. Params follows the ICC convention
// (Curve gamma, a, b, c, d, e, f). A negative Type selects the analytic inverse.
func defaultEvalParametricFn(typ int32, params []float64, r float64) float64 {
	var e, val, disc float64

	switch typ {

	// X = Y ^ Gamma
	case 1:
		if r < 0 {
			if math.Abs(params[0]-1.0) < matrixDetTolerance {
				val = r
			} else {
				val = 0
			}
		} else {
			val = math.Pow(r, params[0])
		}

	// Type 1 reversed: X = Y ^ 1/gamma
	case -1:
		if r < 0 {
			if math.Abs(params[0]-1.0) < matrixDetTolerance {
				val = r
			} else {
				val = 0
			}
		} else {
			if math.Abs(params[0]) < matrixDetTolerance {
				val = plusInf
			} else {
				val = math.Pow(r, 1/params[0])
			}
		}

	// CIE 122-1966: Y = (aX + b)^Gamma | X >= -b/a ; Y = 0 else
	case 2:
		if math.Abs(params[1]) < matrixDetTolerance {
			val = 0
		} else {
			disc = -params[2] / params[1]
			if r >= disc {
				e = fmul64(params[1], r) + params[2]
				if e > 0 {
					val = math.Pow(e, params[0])
				} else {
					val = 0
				}
			} else {
				val = 0
			}
		}

	// Type 2 reversed: X = (Y^1/g - b) / a
	case -2:
		if math.Abs(params[0]) < matrixDetTolerance ||
			math.Abs(params[1]) < matrixDetTolerance {
			val = 0
		} else {
			if r < 0 {
				val = 0
			} else {
				val = (math.Pow(r, 1.0/params[0]) - params[2]) / params[1]
			}
			if val < 0 {
				val = 0
			}
		}

	// IEC 61966-3: Y = (aX + b)^Gamma + c | X <= -b/a ; Y = c else
	case 3:
		if math.Abs(params[1]) < matrixDetTolerance {
			val = 0
		} else {
			disc = -params[2] / params[1]
			if disc < 0 {
				disc = 0
			}
			if r >= disc {
				e = fmul64(params[1], r) + params[2]
				if e > 0 {
					val = math.Pow(e, params[0]) + params[3]
				} else {
					val = 0
				}
			} else {
				val = params[3]
			}
		}

	// Type 3 reversed
	case -3:
		if math.Abs(params[0]) < matrixDetTolerance ||
			math.Abs(params[1]) < matrixDetTolerance {
			val = 0
		} else {
			if r >= params[3] {
				e = r - params[3]
				if e > 0 {
					val = (math.Pow(e, 1/params[0]) - params[2]) / params[1]
				} else {
					val = 0
				}
			} else {
				val = -params[2] / params[1]
			}
		}

	// IEC 61966-2.1 (sRGB): Y = (aX + b)^Gamma | X >= d ; Y = cX else
	case 4:
		if r >= params[4] {
			e = fmul64(params[1], r) + params[2]
			if e > 0 {
				val = math.Pow(e, params[0])
			} else {
				val = 0
			}
		} else {
			val = r * params[3]
		}

	// Type 4 reversed
	case -4:
		e = fmul64(params[1], params[4]) + params[2]
		if e < 0 {
			disc = 0
		} else {
			disc = math.Pow(e, params[0])
		}
		if r >= disc {
			if math.Abs(params[0]) < matrixDetTolerance ||
				math.Abs(params[1]) < matrixDetTolerance {
				val = 0
			} else {
				val = (math.Pow(r, 1.0/params[0]) - params[2]) / params[1]
			}
		} else {
			if math.Abs(params[3]) < matrixDetTolerance {
				val = 0
			} else {
				val = r / params[3]
			}
		}

	// Y = (aX + b)^Gamma + e | X >= d ; Y = cX + f else
	case 5:
		if r >= params[4] {
			e = fmul64(params[1], r) + params[2]
			if e > 0 {
				val = math.Pow(e, params[0]) + params[5]
			} else {
				val = params[5]
			}
		} else {
			val = fmul64(r, params[3]) + params[6]
		}

	// Type 5 reversed
	case -5:
		disc = fmul64(params[3], params[4]) + params[6]
		if r >= disc {
			e = r - params[5]
			if e < 0 {
				val = 0
			} else {
				if math.Abs(params[0]) < matrixDetTolerance ||
					math.Abs(params[1]) < matrixDetTolerance {
					val = 0
				} else {
					val = (math.Pow(e, 1.0/params[0]) - params[2]) / params[1]
				}
			}
		} else {
			if math.Abs(params[3]) < matrixDetTolerance {
				val = 0
			} else {
				val = (r - params[6]) / params[3]
			}
		}

	// Y = (a*X + b)^Gamma + c
	case 6:
		e = fmul64(params[1], r) + params[2]
		if params[0] == 1.0 {
			val = e + params[3]
		} else {
			if e < 0 {
				val = params[3]
			} else {
				val = math.Pow(e, params[0]) + params[3]
			}
		}

	// ((Y - c)^1/Gamma - b) / a
	case -6:
		if math.Abs(params[0]) < matrixDetTolerance ||
			math.Abs(params[1]) < matrixDetTolerance {
			val = 0
		} else {
			e = r - params[3]
			if e < 0 {
				val = 0
			} else {
				val = (math.Pow(e, 1.0/params[0]) - params[2]) / params[1]
			}
		}

	// Y = a*log(b*X^Gamma + c) + d
	case 7:
		e = fmul64(params[2], math.Pow(r, params[0])) + params[3]
		if e <= 0 {
			val = params[4]
		} else {
			val = fmul64(params[1], math.Log10(e)) + params[4]
		}

	// Type 7 reversed
	case -7:
		if math.Abs(params[0]) < matrixDetTolerance ||
			math.Abs(params[1]) < matrixDetTolerance ||
			math.Abs(params[2]) < matrixDetTolerance {
			val = 0
		} else {
			val = math.Pow((math.Pow(10.0, (r-params[4])/params[1])-params[3])/params[2], 1.0/params[0])
		}

	// Y = a*b^(c*X+d) + e
	case 8:
		val = fmul64(params[0], math.Pow(params[1], fmul64(params[2], r)+params[3])) + params[4]

	// Type 8 reversed: Y = (log((y-e)/a)/log(b) - d) / c
	case -8:
		disc = r - params[4]
		if disc < 0 {
			val = 0
		} else {
			if math.Abs(params[0]) < matrixDetTolerance ||
				math.Abs(params[2]) < matrixDetTolerance {
				val = 0
			} else {
				val = (math.Log(disc/params[0])/math.Log(params[1]) - params[3]) / params[2]
			}
		}

	// S-Shaped: (1 - (1-x)^1/g)^1/g
	case 108:
		if math.Abs(params[0]) < matrixDetTolerance {
			val = 0
		} else {
			val = math.Pow(1.0-math.Pow(1-r, 1/params[0]), 1/params[0])
		}

	// Type 108 reversed: 1 - (1 - y^g)^g
	case -108:
		val = 1 - math.Pow(1-math.Pow(r, params[0]), params[0])

	// Sigmoidals
	case 109:
		val = sigmoidFactory(params[0], r)

	case -109:
		val = inverseSigmoidFactory(params[0], r)

	default:
		// Unsupported parametric curve. Should never reach here.
		return 0
	}

	return val
}

// evalSegmentedFn ports EvalSegmentedFn: evaluate the segmented (floating-point)
// description at R. Segments are scanned high to low; the first whose domain
// contains R wins. Type==0 segments interpolate their sample table, others call
// the located parametric evaluator. Any infinity collapses to PLUS_INF (the
// reference's isinf test matches both signs, so MINUS_INF is unreachable there).
// If no segment matches, MINUS_INF is returned.
func (g *ToneCurve) evalSegmentedFn(r float64) float64 {
	for i := int(g.nSegments) - 1; i >= 0; i-- {
		seg := &g.segments[i]

		if r > float64(seg.X0) && r <= float64(seg.X1) {
			var out float64

			if seg.Type == 0 {
				num := float32(r - float64(seg.X0))
				r1 := num / (seg.X1 - seg.X0)
				// EvalSegmentedFn: dispatch through the segment's cmsInterpParams
				// (LinLerp1Dfloat). The reference re-points ->Table at each eval;
				// here the params already reference the segment's SampledPoints.
				p := g.segInterp[i]
				if p == nil {
					// allocateToneCurveStruct rejects sampled segments it cannot
					// build an interp seam for, so this is unreachable; the
					// reference would deref NULL here. Fall back to the no-match
					// sentinel rather than panic.
					return minusInf
				}
				in := [1]float32{r1}
				var out32 [1]float32
				p.Interpolation.LerpFloat(in[:], out32[:], p)
				out = float64(out32[0])
			} else {
				eval := g.evals[i]
				if eval == nil {
					// Unreachable for the same reason (unregistered types fail
					// construction); the reference calls a NULL function pointer.
					return minusInf
				}
				out = eval(seg.Type, seg.Params[:], r)
			}

			if math.IsInf(out, 0) {
				return plusInf
			}
			return out
		}
	}
	return minusInf
}

// ---------------------------------------------------------- construction

// maxSampledSegmentPoints bounds the grid-point count of a Type-0 (sampled)
// tone-curve segment when a curve is built through the public API. The tag
// reader already caps the count against the profile size (a sampled point is
// four bytes on disk), so no real profile approaches this ceiling; it exists
// solely so a caller passing a 32-bit NGridPoints cannot request a multi-
// gigabyte allocation. 16M points is a 64 MB float table — far beyond any
// legitimate curve.
const maxSampledSegmentPoints = 1 << 24

// allocateToneCurveStruct ports AllocateToneCurveStruct: build the ToneCurve
// storage, wiring per-segment evaluators and interpolation seams. nEntries may
// be zero (segment-only inverse curves) or nSegments zero (pure tables); both
// zero is an error, as is nEntries beyond the 65530 smoothing limit.
func (ctx *Context) allocateToneCurveStruct(nEntries, nSegments uint32, segments []CurveSegment, values []uint16) (*ToneCurve, error) {
	if nEntries > 65530 {
		return nil, ctx.signalError(ErrRange, "Couldn't create tone curve of more than 65530 entries")
	}
	if nEntries == 0 && nSegments == 0 {
		return nil, ctx.signalError(ErrRange, "Couldn't create tone curve with zero segments and no table")
	}

	p := &ToneCurve{ctx: ctx, nSegments: nSegments, nEntries: nEntries}

	if nSegments > 0 {
		p.segments = make([]CurveSegment, nSegments)
		p.evals = make([]parametricEvaluator, nSegments)
		p.segInterp = make([]*InterpParams, nSegments)
	}

	if nEntries > 0 {
		p.table16 = make([]uint16, nEntries)
	}
	if values != nil && nEntries > 0 {
		// copy already bounds to min(len(dst), len(values)); reslicing values to
		// nEntries would instead panic if a caller ever passed a short table.
		copy(p.table16, values)
	}

	if segments != nil && nSegments > 0 {
		for i := uint32(0); i < nSegments; i++ {
			p.segments[i] = segments[i]

			if segments[i].Type == 0 {
				// Sampled segment: dup the points and build the float
				// interp seam over them (Table set here rather than lazily).
				//
				// A zero-length sampled segment is rejected here rather than
				// deferred: computeInterpParams would set Domain[0] = 0-1, which
				// wraps to 0xFFFFFFFF and drives the float interpolator to index
				// far past the (empty) table. The reference dereferences a NULL
				// SampledPoints in the same situation. Cap the point count too so
				// an API caller cannot request an unbounded allocation.
				if segments[i].NGridPoints == 0 {
					return nil, ctx.signalError(ErrRange,
						"tone curve: sampled segment %d has zero grid points", i)
				}
				if segments[i].NGridPoints > maxSampledSegmentPoints {
					return nil, ctx.signalError(ErrRange,
						"tone curve: sampled segment %d has too many grid points (%d, max %d)",
						i, segments[i].NGridPoints, maxSampledSegmentPoints)
				}
				pts := make([]float32, segments[i].NGridPoints)
				if segments[i].SampledPoints != nil {
					n := segments[i].NGridPoints
					if int(n) > len(segments[i].SampledPoints) {
						n = uint32(len(segments[i].SampledPoints))
					}
					copy(pts, segments[i].SampledPoints[:n])
				}
				p.segments[i].SampledPoints = pts

				// _cmsComputeInterpParams(nGridPoints,1,1,pts,CMS_LERP_FLAGS_FLOAT).
				// The reference passes a NULL table and re-points ->Table at eval
				// time to SampledPoints; pts is that same array, so binding it here
				// is equivalent.
				sp, err := computeInterpParams(ctx, segments[i].NGridPoints, 1, 1, pts, cmsLerpFlagsFloat)
				if err != nil {
					return nil, err
				}
				p.segInterp[i] = sp
			} else {
				p.segments[i].SampledPoints = nil
			}

			if segments[i].Type != 0 {
				// A non-sampled segment needs a registered parametric evaluator;
				// without one, evalSegmentedFn would call a nil function pointer.
				// Reject the curve rather than panic on the unknown type.
				eval, _, ok := ctx.getParametricCurveByType(segments[i].Type)
				if !ok {
					return nil, ctx.signalError(ErrUnknownExtension,
						"tone curve: unsupported parametric segment type %d", segments[i].Type)
				}
				p.evals[i] = eval
			}
		}
	}

	// Whole-curve 16-bit interpolation params:
	// _cmsComputeInterpParams(nEntries,1,1,Table16,CMS_LERP_FLAGS_16BITS). When
	// nEntries==0 (segment-only inverse curves) the reference passes a NULL
	// table; the params are then never evaluated (EvalToneCurveFloat routes
	// such curves through EvalSegmentedFn), matching C's Domain[0] underflow.
	var table16 any
	if nEntries > 0 {
		table16 = p.table16
	}
	ip, err := computeInterpParams(ctx, nEntries, 1, 1, table16, cmsLerpFlags16Bits)
	if err != nil {
		return nil, err
	}
	p.interp = ip

	return p, nil
}

// entriesByGamma ports EntriesByGamma: a near-identity gamma needs only two
// table nodes, everything else uses 4096.
func entriesByGamma(gamma float64) uint32 {
	if math.Abs(gamma-1.0) < 0.001 {
		return 2
	}
	return 4096
}

// BuildSegmentedToneCurve ports cmsBuildSegmentedToneCurve on a context. It
// builds the floating-point segment description, then samples it into the
// 16-bit optimization table.
func (ctx *Context) BuildSegmentedToneCurve(segments []CurveSegment) (*ToneCurve, error) {
	if len(segments) == 0 {
		return nil, ctx.signalError(ErrNull, "cmsBuildSegmentedToneCurve: no segments")
	}

	nSegments := uint32(len(segments))
	nGridPoints := uint32(4096)

	// Optimization for identity curves.
	if nSegments == 1 && segments[0].Type == 1 {
		nGridPoints = entriesByGamma(segments[0].Params[0])
	}

	g, err := ctx.allocateToneCurveStruct(nGridPoints, nSegments, segments, nil)
	if err != nil {
		return nil, err
	}

	for i := uint32(0); i < nGridPoints; i++ {
		r := float64(i) / float64(nGridPoints-1)
		val := g.evalSegmentedFn(r)
		g.table16[i] = quickSaturateWord(val * 65535.0)
	}

	return g, nil
}

// BuildSegmentedToneCurve builds a segmented tone curve on the default context.
func BuildSegmentedToneCurve(segments []CurveSegment) (*ToneCurve, error) {
	return defaultContext.BuildSegmentedToneCurve(segments)
}

// BuildTabulatedToneCurve16 ports cmsBuildTabulatedToneCurve16 on a context: a
// limited-precision curve defined purely by a 16-bit table (no floating-point
// description).
func (ctx *Context) BuildTabulatedToneCurve16(values []uint16) (*ToneCurve, error) {
	return ctx.allocateToneCurveStruct(uint32(len(values)), 0, nil, values)
}

// BuildTabulatedToneCurve16 builds a 16-bit tabulated curve on the default context.
func BuildTabulatedToneCurve16(values []uint16) (*ToneCurve, error) {
	return defaultContext.BuildTabulatedToneCurve16(values)
}

// BuildTabulatedToneCurveFloat ports cmsBuildTabulatedToneCurveFloat on a
// context: a floating-point sample table is wrapped in a 3-segment curve whose
// middle segment is sampled and whose flanks hold the first/last sample.
func (ctx *Context) BuildTabulatedToneCurveFloat(values []float32) (*ToneCurve, error) {
	nEntries := uint32(len(values))
	if nEntries == 0 || values == nil {
		return nil, ctx.signalError(ErrRange, "cmsBuildTabulatedToneCurveFloat: empty values")
	}

	seg := make([]CurveSegment, 3)

	// Constant = values[0] up to 0.
	seg[0].X0 = curveMinusInf
	seg[0].X1 = 0
	seg[0].Type = 6
	seg[0].Params[0] = 1
	seg[0].Params[3] = float64(values[0])

	// Sampled from 0 to 1.
	seg[1].X0 = 0
	seg[1].X1 = 1.0
	seg[1].Type = 0
	seg[1].NGridPoints = nEntries
	seg[1].SampledPoints = values

	// Constant = last sample beyond 1.
	seg[2].X0 = 1.0
	seg[2].X1 = curvePlusInf
	seg[2].Type = 6
	seg[2].Params[0] = 1
	seg[2].Params[3] = float64(values[nEntries-1])

	return ctx.BuildSegmentedToneCurve(seg)
}

// BuildTabulatedToneCurveFloat builds a float tabulated curve on the default context.
func BuildTabulatedToneCurveFloat(values []float32) (*ToneCurve, error) {
	return defaultContext.BuildTabulatedToneCurveFloat(values)
}

// BuildParametricToneCurve ports cmsBuildParametricToneCurve on a context.
// Type selects a parametric function (built-in or plug-in); a negative Type
// requests the analytic inverse. Returns an error for an unknown type.
func (ctx *Context) BuildParametricToneCurve(typ int32, params []float64) (*ToneCurve, error) {
	_, paramCount, ok := ctx.getParametricCurveByType(typ)
	if !ok {
		return nil, ctx.signalError(ErrUnknownExtension, "Invalid parametric curve type %d", typ)
	}
	if params == nil {
		return nil, ctx.signalError(ErrNull, "cmsBuildParametricToneCurve: nil params")
	}

	var seg CurveSegment
	seg.X0 = curveMinusInf
	seg.X1 = curvePlusInf
	seg.Type = typ

	n := paramCount
	if n > 10 {
		n = 10
	}
	for i := uint32(0); i < n && int(i) < len(params); i++ {
		seg.Params[i] = params[i]
	}

	return ctx.BuildSegmentedToneCurve([]CurveSegment{seg})
}

// BuildParametricToneCurve builds a parametric curve on the default context.
func BuildParametricToneCurve(typ int32, params []float64) (*ToneCurve, error) {
	return defaultContext.BuildParametricToneCurve(typ, params)
}

// BuildGamma ports cmsBuildGamma on a context: a pure power curve X = Y^gamma.
func (ctx *Context) BuildGamma(gamma float64) (*ToneCurve, error) {
	return ctx.BuildParametricToneCurve(1, []float64{gamma})
}

// BuildGamma builds a gamma curve on the default context.
func BuildGamma(gamma float64) (*ToneCurve, error) {
	return defaultContext.BuildGamma(gamma)
}

// Dup ports cmsDupToneCurve: an independent deep copy of the curve.
func (c *ToneCurve) Dup() (*ToneCurve, error) {
	if c == nil {
		return nil, nil
	}
	return c.ctx.allocateToneCurveStruct(c.nEntries, c.nSegments, c.segments, c.table16)
}

// Free ports cmsFreeToneCurve as a documented no-op: the Go garbage collector
// reclaims the curve. Kept only for API symmetry with the reference.
func (c *ToneCurve) Free() {}

// JoinToneCurve ports cmsJoinToneCurve on a context: build y = Y^-1(X(t)) by
// sampling X forward and Y reversed over nResultingPoints. X and Y should be
// monotonic.
func (ctx *Context) JoinToneCurve(x, y *ToneCurve, nResultingPoints uint32) (*ToneCurve, error) {
	if x == nil || y == nil {
		return nil, ctx.signalError(ErrNull, "cmsJoinToneCurve: nil curve")
	}

	yReversed, err := y.ReverseEx(nResultingPoints)
	if err != nil {
		return nil, err
	}

	// _cmsCalloc(nResultingPoints, sizeof(cmsFloat32Number)) in cmsJoinToneCurve,
	// placed after the reverse exactly as the reference orders it. The bound is
	// not redundant with ReverseEx's 65530-entry check: ReverseEx returns early
	// through the analytic-inverse path (see below) for any single-segment
	// parametric curve, so nResultingPoints can arrive here unexamined.
	if !allocSizeOK(nResultingPoints, 4) {
		return nil, ctx.signalError(ErrRange, "cmsJoinToneCurve: %d points exceeds the %d byte allocation limit", nResultingPoints, maxMemoryForAlloc)
	}

	res := make([]float32, nResultingPoints)
	for i := uint32(0); i < nResultingPoints; i++ {
		t := float32(i) / float32(nResultingPoints-1)
		xv := x.EvalFloat(t)
		res[i] = yReversed.EvalFloat(xv)
	}

	return ctx.BuildTabulatedToneCurveFloat(res)
}

// JoinToneCurve joins two curves on the default context.
func JoinToneCurve(x, y *ToneCurve, nResultingPoints uint32) (*ToneCurve, error) {
	return defaultContext.JoinToneCurve(x, y, nResultingPoints)
}

// ------------------------------------------------------------- inversion

// getInterval ports GetInterval: locate the table cell whose [y0,y1] range
// brackets In, handling both ascending and descending tables and local
// direction changes (non-monotonic tables). domain is nEntries-1. Returns the
// cell's lower index or -1.
func getInterval(in float64, lut []uint16, domain int) int {
	if domain < 1 {
		return -1
	}

	if lut[0] < lut[domain] {
		// Table is overall ascending.
		for i := domain - 1; i >= 0; i-- {
			y0 := int(lut[i])
			y1 := int(lut[i+1])
			if y0 <= y1 {
				if in >= float64(y0) && in <= float64(y1) {
					return i
				}
			} else if y1 < y0 {
				if in >= float64(y1) && in <= float64(y0) {
					return i
				}
			}
		}
	} else {
		// Table is overall descending.
		for i := 0; i < domain; i++ {
			y0 := int(lut[i])
			y1 := int(lut[i+1])
			if y0 <= y1 {
				if in >= float64(y0) && in <= float64(y1) {
					return i
				}
			} else if y1 < y0 {
				if in >= float64(y1) && in <= float64(y0) {
					return i
				}
			}
		}
	}

	return -1
}

// ReverseEx ports cmsReverseToneCurveEx: invert the curve into a table of
// nResultSamples entries. Single-segment curves of a known parametric type are
// inverted analytically; otherwise the 16-bit table is inverted numerically.
func (c *ToneCurve) ReverseEx(nResultSamples uint32) (*ToneCurve, error) {
	ctx := c.ctx

	// Try to reverse analytically where possible.
	if c.nSegments == 1 && c.segments[0].Type > 0 {
		if _, _, ok := ctx.getParametricCurveByType(c.segments[0].Type); ok {
			return ctx.BuildParametricToneCurve(-c.segments[0].Type, c.segments[0].Params[:])
		}
	}

	out, err := ctx.allocateToneCurveStruct(nResultSamples, 0, nil, nil)
	if err != nil {
		return nil, err
	}

	ascending := !c.IsDescending()

	var a, b float64
	for i := 0; i < int(nResultSamples); i++ {
		y := float64(i) * 65535.0 / float64(nResultSamples-1)

		j := getInterval(y, c.table16, int(c.interp.Domain[0]))
		if j >= 0 {
			x1 := float64(c.table16[j])
			x2 := float64(c.table16[j+1])

			y1 := float64(j) * 65535.0 / float64(c.nEntries-1)
			y2 := float64(j+1) * 65535.0 / float64(c.nEntries-1)

			if x1 == x2 {
				if ascending {
					out.table16[i] = quickSaturateWord(y2)
				} else {
					out.table16[i] = quickSaturateWord(y1)
				}
				continue
			}
			a = (y2 - y1) / (x2 - x1)
			b = y2 - fmul64(a, x2)
		}

		out.table16[i] = quickSaturateWord(fmul64(a, y) + b)
	}

	return out, nil
}

// Reverse ports cmsReverseToneCurve: invert into a 4096-entry table.
func (c *ToneCurve) Reverse() (*ToneCurve, error) {
	return c.ReverseEx(4096)
}

// ------------------------------------------------------------- smoothing

// smooth2 ports smooth2: solve the tridiagonal system of Eilers' second-
// difference smoothing. Vectors are 1-indexed (index 0 unused) to mirror the
// reference exactly. Returns false when the problem is ill-posed.
func smooth2(w, y, z []float32, lambda float32, m int) bool {
	if m < 4 || float64(lambda) < matrixDetTolerance {
		return false
	}

	c := make([]float32, maxNodesInCurve)
	d := make([]float32, maxNodesInCurve)
	e := make([]float32, maxNodesInCurve)

	d[1] = w[1] + lambda
	c[1] = -2 * lambda / d[1]
	e[1] = lambda / d[1]
	z[1] = w[1] * y[1]
	d[2] = w[2] + float32(5*lambda) - float32(d[1]*c[1]*c[1])
	c[2] = (float32(-4*lambda) - float32(d[1]*c[1]*e[1])) / d[2]
	e[2] = lambda / d[2]
	z[2] = float32(w[2]*y[2]) - float32(c[1]*z[1])

	for i := 3; i < m-1; i++ {
		i1 := i - 1
		i2 := i - 2
		d[i] = w[i] + float32(6*lambda) - float32(c[i1]*c[i1]*d[i1]) - float32(e[i2]*e[i2]*d[i2])
		c[i] = (float32(-4*lambda) - float32(d[i1]*c[i1]*e[i1])) / d[i]
		e[i] = lambda / d[i]
		z[i] = float32(w[i]*y[i]) - float32(c[i1]*z[i1]) - float32(e[i2]*z[i2])
	}

	i1 := m - 2
	i2 := m - 3
	d[m-1] = w[m-1] + float32(5*lambda) - float32(c[i1]*c[i1]*d[i1]) - float32(e[i2]*e[i2]*d[i2])
	c[m-1] = (float32(-2*lambda) - float32(d[i1]*c[i1]*e[i1])) / d[m-1]
	z[m-1] = float32(w[m-1]*y[m-1]) - float32(c[i1]*z[i1]) - float32(e[i2]*z[i2])

	i1 = m - 1
	i2 = m - 2
	d[m] = w[m] + lambda - float32(c[i1]*c[i1]*d[i1]) - float32(e[i2]*e[i2]*d[i2])
	z[m] = (float32(w[m]*y[m]) - float32(c[i1]*z[i1]) - float32(e[i2]*z[i2])) / d[m]
	z[m-1] = z[m-1]/d[m-1] - float32(c[m-1]*z[m])

	for i := m - 2; 1 <= i; i-- {
		z[i] = z[i]/d[i] - float32(c[i]*z[i+1]) - float32(e[i]*z[i+2])
	}

	return true
}

// Smooth ports cmsSmoothToneCurve: smooth a regularly-sampled curve in place.
// A negative lambda disables the monotonicity/degeneracy sanity checks (its
// magnitude is used). Returns nil on success; on a genuine failure it returns
// the error the reference would have signalled. Linear curves are left
// untouched (success). Mirrors the reference's SuccessStatus/notCheck logic.
func (t *ToneCurve) Smooth(lambda float64) error {
	if t == nil || t.interp == nil {
		return errorf(ErrInternal, "cmsSmoothToneCurve: nil curve")
	}

	ctx := t.ctx

	if t.IsLinear() {
		// Only non-linear curves need smoothing.
		return nil
	}

	nItems := t.nEntries
	if nItems >= maxNodesInCurve {
		return ctx.signalError(ErrRange, "cmsSmoothToneCurve: Too many points.")
	}

	w := make([]float32, nItems+1)
	y := make([]float32, nItems+1)
	z := make([]float32, nItems+1)

	for i := uint32(0); i < nItems; i++ {
		y[i+1] = float32(t.table16[i])
		w[i+1] = 1.0
	}

	notCheck := false
	if lambda < 0 {
		notCheck = true
		lambda = -lambda
	}

	if !smooth2(w, y, z, float32(lambda), int(nItems)) {
		return ctx.signalError(ErrRange, "cmsSmoothToneCurve: Function smooth2 failed.")
	}

	// Reality checks.
	successStatus := true
	var lastErr error
	var zeros, poles uint32

	for i := nItems; i > 1; i-- {
		if z[i] == 0 {
			zeros++
		}
		if z[i] >= 65535 {
			poles++
		}
		if z[i] < z[i-1] {
			lastErr = ctx.signalError(ErrRange, "cmsSmoothToneCurve: Non-Monotonic.")
			successStatus = notCheck
			break
		}
	}

	if successStatus && zeros > nItems/3 {
		lastErr = ctx.signalError(ErrRange, "cmsSmoothToneCurve: Degenerated, mostly zeros.")
		successStatus = notCheck
	}

	if successStatus && poles > nItems/3 {
		lastErr = ctx.signalError(ErrRange, "cmsSmoothToneCurve: Degenerated, mostly poles.")
		successStatus = notCheck
	}

	if successStatus {
		for i := uint32(0); i < nItems; i++ {
			t.table16[i] = quickSaturateWord(float64(z[i+1]))
		}
		return nil
	}

	return lastErr
}

// ------------------------------------------------------------- predicates

// IsLinear ports cmsIsToneCurveLinear: true when the 16-bit table matches a
// linear ramp to within 12 bits (0x0f counts) at every node.
func (c *ToneCurve) IsLinear() bool {
	for i := uint32(0); i < c.nEntries; i++ {
		q := quickSaturateWord(float64(i) * 65535.0 / float64(c.nEntries-1))
		if absInt(int(c.table16[i])-int(q)) > 0x0f {
			return false
		}
	}
	return true
}

// IsMonotonic ports cmsIsToneCurveMonotonic (allowing a 2-count ripple).
func (t *ToneCurve) IsMonotonic() bool {
	n := t.nEntries
	if n < 2 {
		return true
	}

	if t.IsDescending() {
		last := int(t.table16[0])
		for i := 1; i < int(n); i++ {
			if int(t.table16[i])-last > 2 {
				return false
			}
			last = int(t.table16[i])
		}
	} else {
		last := int(t.table16[n-1])
		for i := int(n) - 2; i >= 0; i-- {
			if int(t.table16[i])-last > 2 {
				return false
			}
			last = int(t.table16[i])
		}
	}

	return true
}

// IsDescending ports cmsIsToneCurveDescending.
func (t *ToneCurve) IsDescending() bool {
	return t.table16[0] > t.table16[t.nEntries-1]
}

// IsMultisegment ports cmsIsToneCurveMultisegment.
func (t *ToneCurve) IsMultisegment() bool {
	return t.nSegments > 1
}

// ParametricType ports cmsGetToneCurveParametricType: the parametric type of a
// single-segment curve, or 0 for multi-segment/tabulated curves.
func (t *ToneCurve) ParametricType() int32 {
	if t.nSegments != 1 {
		return 0
	}
	return t.segments[0].Type
}

// -------------------------------------------------------------- evaluation

// EvalFloat ports cmsEvalToneCurveFloat: evaluate the curve at v with high
// precision. Tabulated (segment-less) curves fall back to the 16-bit table.
func (c *ToneCurve) EvalFloat(v float32) float32 {
	if c.nSegments == 0 {
		in := quickSaturateWord(float64(v) * 65535.0)
		out := c.Eval16(in)
		return float32(float64(out) / 65535.0)
	}
	return float32(c.evalSegmentedFn(float64(v)))
}

// Eval16 ports cmsEvalToneCurve16: evaluate the curve through its 16-bit
// interpolation params (the throughput path used by 8/16-bit transforms),
// dispatching through InterpParams.Interpolation.Lerp16 (LinLerp1D).
func (c *ToneCurve) Eval16(v uint16) uint16 {
	in := [1]uint16{v}
	var out [1]uint16
	c.interp.Interpolation.Lerp16(in[:], out[:], c.interp)
	return out[0]
}

// EstimateGamma ports cmsEstimateGamma: least-squares estimate of the curve's
// effective gamma, or -1 if the fit's standard deviation exceeds precision
// (i.e. the curve is not well described by a single exponent).
func (t *ToneCurve) EstimateGamma(precision float64) float64 {
	var sum, sum2, n float64

	for i := 1; i < maxNodesInCurve-1; i++ {
		x := float64(i) / float64(maxNodesInCurve-1)
		y := float64(t.EvalFloat(float32(x)))

		// Avoid the lower 7% to skip linear-ramp artifacts.
		if y > 0.0 && y < 1.0 && x > 0.07 {
			gamma := math.Log(y) / math.Log(x)
			sum += gamma
			sum2 += fmul64(gamma, gamma)
			n++
		}
	}

	if n <= 1 {
		return -1.0
	}

	std := math.Sqrt((fmul64(n, sum2) - fmul64(sum, sum)) / (n * (n - 1)))
	if std > precision {
		return -1.0
	}

	return sum / n
}

// -------------------------------------------------------------- accessors

// EstimatedTableEntries ports cmsGetToneCurveEstimatedTableEntries.
func (t *ToneCurve) EstimatedTableEntries() uint32 { return t.nEntries }

// EstimatedTable ports cmsGetToneCurveEstimatedTable: the low-resolution 16-bit
// table backing the curve. The slice aliases the curve's storage.
func (t *ToneCurve) EstimatedTable() []uint16 { return t.table16 }

// GetSegment ports cmsGetToneCurveSegment: the n-th segment, or nil if out of
// range. The pointer aliases the curve's storage.
func (t *ToneCurve) GetSegment(n int32) *CurveSegment {
	if n < 0 || n >= int32(t.nSegments) {
		return nil
	}
	return &t.segments[n]
}

// -------------------------------------------------------------- small helpers

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
