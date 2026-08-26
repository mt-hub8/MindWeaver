package lcms2

// PostScript ColorRenderingDictionary (CRD) and ColorSpaceArray (CSA)
// generation. Port of src/cmsps2.c.
//
// These emit PostScript Level 2 colour resources from ICC profiles, mirroring
// cmsGetPostScriptCSA / cmsGetPostScriptCRD. Output is generated deterministically
// from the profile, except for the "% Created:" timestamp line in the CRD header
// (see emitHeader), so that generated text can be diffed byte-for-byte against
// the reference for a fixed profile.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// maxPSCols mirrors MAXPSCOLS: columns on hex tables.
const maxPSCols = 60

// PSResourceType selects the kind of PostScript colour resource, mirroring
// cmsPSResourceType.
type PSResourceType int

const (
	// PSResourceCSA is a PostScript ColorSpaceArray (cmsPS_RESOURCE_CSA).
	PSResourceCSA PSResourceType = 0
	// PSResourceCRD is a PostScript ColorRenderingDictionary (cmsPS_RESOURCE_CRD).
	PSResourceCRD PSResourceType = 1
)

// Local TYPE_* codes used only by this file.
var (
	psTypeGray8         = colorspaceSH(PTGray) | channelsSH(1) | bytesSH(1) // TYPE_GRAY_8
	psTypeLab16         = colorspaceSH(PTLab) | channelsSH(3) | bytesSH(2)  // TYPE_Lab_16
	psTypeNamedColorIdx = colorspaceSH(PTANY) | channelsSH(1) | bytesSH(2)  // TYPE_NAMED_COLOR_INDEX
)

// psWriter accumulates generated PostScript text. It mirrors the cmsIOHANDLER
// memory stream plus the _cmsPSActualColumn global used by the hex emitter.
type psWriter struct {
	buf bytes.Buffer
	col int // _cmsPSActualColumn
	ctx *Context
	err bool
}

// printf mirrors _cmsIOPrintf: format the arguments, then replace every ','
// with '.' (the reference does this to stay locale-independent — the PS
// generator never wants commas). A single call producing 2047 bytes or more is
// a fatal truncation error, exactly as the reference treats it. Format strings
// passed here must never use %g: C's %g emits 6 significant digits whereas Go's
// fmt %g emits the shortest round-trip form, so callers pre-format such values
// with g6 and pass them through %s.
func (w *psWriter) printf(format string, a ...any) {
	if w.err {
		return
	}
	s := fmt.Sprintf(format, a...)
	if len(s) >= 2047 {
		w.err = true
		return
	}
	if strings.IndexByte(s, ',') >= 0 {
		s = strings.ReplaceAll(s, ",", ".")
	}
	w.buf.WriteString(s)
}

// g6 formats v the way C's printf("%g") does with the default precision of 6
// significant digits: shortest form with trailing zeros stripped, switching to
// exponent form when the exponent is < -4 or >= 6.
func g6(v float64) string {
	return strconv.FormatFloat(v, 'g', 6, 64)
}

// word2Byte ports Word2Byte.
func word2Byte(w uint16) uint8 {
	return uint8(math.Floor(float64(w)/257.0 + 0.5))
}

// writeByte ports WriteByte.
func (w *psWriter) writeByte(b uint8) {
	w.printf("%02x", b)
	w.col += 2
	if w.col > maxPSCols {
		w.printf("\n")
		w.col = 0
	}
}

// removeCR ports RemoveCR: replace CR/LF with spaces (bounded to 2047 like the
// reference static buffer).
func removeCR(txt string) string {
	if len(txt) > 2047 {
		txt = txt[:2047]
	}
	b := []byte(txt)
	for i, c := range b {
		if c == '\n' || c == '\r' {
			b[i] = ' '
		}
	}
	return string(b)
}

// emitPSEscaped ports EmitPSEscaped: write the body of a PostScript string
// literal, escaping '\\', '(' and ')' and emitting non-printable / high-bit
// bytes as octal triples. The surrounding parentheses are the caller's job.
func (w *psWriter) emitPSEscaped(txt string) {
	for i := 0; i < len(txt); i++ {
		c := txt[i]
		switch {
		case c == '\\' || c == '(' || c == ')':
			w.printf("\\%c", c)
		case c < 0x20 || c >= 0x7F:
			w.printf("\\%03o", c)
		default:
			w.printf("%c", c)
		}
	}
}

// cmsSigProfileDescriptionTag / cmsSigCopyrightTag aliases matching the C names.
const (
	cmsSigProfileDescriptionTag = SigProfileDescriptionTag
	cmsSigCopyrightTag          = SigCopyrightTag
)

// emitHeader ports EmitHeader. The "% Created:" line carries the current time
// (ctime format) and is therefore the one non-deterministic line of output.
func (w *psWriter) emitHeader(title string, p *Profile) {
	var descASCII, copyrightASCII string
	if v, err := p.ReadTag(cmsSigProfileDescriptionTag); err == nil {
		if mlu, ok := v.(*MLU); ok && mlu != nil {
			descASCII = mluGetASCII255(mlu)
		}
	}
	if v, err := p.ReadTag(cmsSigCopyrightTag); err == nil {
		if mlu, ok := v.(*MLU); ok && mlu != nil {
			copyrightASCII = mluGetASCII255(mlu)
		}
	}

	w.printf("%%!PS-Adobe-3.0\n")
	w.printf("%%\n")
	w.printf("%% %s\n", title)
	w.printf("%% Source: %s\n", removeCR(descASCII))
	w.printf("%%         %s\n", removeCR(copyrightASCII))
	w.printf("%% Created: %s", ctimeNow()) // ctime appends a \n
	w.printf("%%\n")
	w.printf("%%%%BeginResource\n")
}

// mluGetASCII255 mirrors cmsMLUgetASCII(mlu, cmsNoLanguage, cmsNoCountry, buf, 255):
// the default translation, truncated into a 255-byte buffer. The reference then
// hands the result to printf("%s") / strncpy, which stop at the first NUL, so an
// embedded NUL (ICC 'desc' pads with one) truncates the string — mirror that
// C-string semantics here rather than carrying the NUL into the output.
func mluGetASCII255(mlu *MLU) string {
	buf := make([]byte, 255)
	n := mlu.mluGetASCII(0, 0, buf)
	if n == 0 {
		return ""
	}
	s := buf[:n-1]
	if idx := bytes.IndexByte(s, 0); idx >= 0 {
		s = s[:idx]
	}
	return string(s)
}

// ctimeNow renders the current local time the way C's ctime(3) does:
// "Www Mmm dd hh:mm:ss yyyy\n".
func ctimeNow() string {
	return time.Now().Format("Mon Jan _2 15:04:05 2006") + "\n"
}

// emitWhiteBlackD50 ports EmitWhiteBlackD50.
func (w *psWriter) emitWhiteBlackD50(black CIEXYZ) {
	w.printf("/BlackPoint [%f %f %f]\n", black.X, black.Y, black.Z)
	d50 := D50XYZ()
	w.printf("/WhitePoint [%f %f %f]\n", d50.X, d50.Y, d50.Z)
}

// emitRangeCheck ports EmitRangeCheck.
func (w *psWriter) emitRangeCheck() {
	w.printf("dup 0.0 lt { pop 0.0 } if " +
		"dup 1.0 gt { pop 1.0 } if ")
}

// emitIntent ports EmitIntent.
func (w *psWriter) emitIntent(intent uint32) {
	var s string
	switch intent {
	case IntentPerceptual:
		s = "Perceptual"
	case IntentRelativeColorimetric:
		s = "RelativeColorimetric"
	case IntentAbsoluteColorimetric:
		s = "AbsoluteColorimetric"
	case IntentSaturation:
		s = "Saturation"
	default:
		s = "Undefined"
	}
	w.printf("/RenderingIntent (%s)\n", s)
}

// emitLab2XYZ ports EmitLab2XYZ.
func (w *psWriter) emitLab2XYZ() {
	w.printf("/RangeABC [ 0 1 0 1 0 1]\n")
	w.printf("/DecodeABC [\n")
	w.printf("{100 mul  16 add 116 div } bind\n")
	w.printf("{255 mul 128 sub 500 div } bind\n")
	w.printf("{255 mul 128 sub 200 div } bind\n")
	w.printf("]\n")
	w.printf("/MatrixABC [ 1 1 1 1 0 0 0 0 -1]\n")
	w.printf("/RangeLMN [ -0.236 1.254 0 1 -0.635 1.640 ]\n")
	w.printf("/DecodeLMN [\n")
	w.printf("{dup 6 29 div ge {dup dup mul mul} {4 29 div sub 108 841 div mul} ifelse 0.964200 mul} bind\n")
	w.printf("{dup 6 29 div ge {dup dup mul mul} {4 29 div sub 108 841 div mul} ifelse } bind\n")
	w.printf("{dup 6 29 div ge {dup dup mul mul} {4 29 div sub 108 841 div mul} ifelse 0.824900 mul} bind\n")
	w.printf("]\n")
}

// emit1Gamma ports Emit1Gamma.
func (w *psWriter) emit1Gamma(table *ToneCurve) {
	// On error, empty tables or linear assume gamma 1.0
	if table == nil || table.EstimatedTableEntries() == 0 || table.IsLinear() {
		w.printf("{ 1 } bind ")
		return
	}

	// Check if is really an exponential. If so, emit "exp"
	gamma := table.EstimateGamma(0.001)
	if gamma > 0 {
		w.printf("{ %s exp } bind ", g6(gamma))
		return
	}

	w.printf("{ ")

	// Bounds check
	w.emitRangeCheck()

	// Emit interpolation code
	w.printf(" [")

	tab := table.EstimatedTable()
	for i := uint32(0); i < table.EstimatedTableEntries(); i++ {
		if i%10 == 0 {
			w.printf("\n  ")
		}
		w.printf("%d ", tab[i])
	}

	w.printf("] ")            // v tab
	w.printf("dup ")          // v tab tab
	w.printf("length 1 sub ") // v tab dom
	w.printf("3 -1 roll ")    // tab dom v
	w.printf("mul ")          // tab val2
	w.printf("dup ")          // tab val2 val2
	w.printf("dup ")          // tab val2 val2 val2
	w.printf("floor cvi ")    // tab val2 val2 cell0
	w.printf("exch ")         // tab val2 cell0 val2
	w.printf("ceiling cvi ")  // tab val2 cell0 cell1
	w.printf("3 index ")      // tab val2 cell0 cell1 tab
	w.printf("exch ")         // tab val2 cell0 tab cell1
	w.printf("get\n  ")       // tab val2 cell0 y1
	w.printf("4 -1 roll ")    // val2 cell0 y1 tab
	w.printf("3 -1 roll ")    // val2 y1 tab cell0
	w.printf("get ")          // val2 y1 y0
	w.printf("dup ")          // val2 y1 y0 y0
	w.printf("3 1 roll ")     // val2 y0 y1 y0
	w.printf("sub ")          // val2 y0 (y1-y0)
	w.printf("3 -1 roll ")    // y0 (y1-y0) val2
	w.printf("dup ")          // y0 (y1-y0) val2 val2
	w.printf("floor cvi ")    // y0 (y1-y0) val2 floor(val2)
	w.printf("sub ")          // y0 (y1-y0) rest
	w.printf("mul ")          // y0 t1
	w.printf("add ")          // y
	w.printf("65535 div\n")   // result

	w.printf(" } bind ")
}

// gammaTableEquals ports GammaTableEquals.
func gammaTableEquals(g1, g2 []uint16) bool {
	if len(g1) != len(g2) {
		return false
	}
	for i := range g1 {
		if g1[i] != g2[i] {
			return false
		}
	}
	return true
}

// emitNGamma ports EmitNGamma.
func (w *psWriter) emitNGamma(g []*ToneCurve) {
	for i := 0; i < len(g); i++ {
		if g[i] == nil {
			return // Error
		}
		if i > 0 && gammaTableEquals(g[i-1].EstimatedTable(), g[i].EstimatedTable()) {
			w.printf("dup ")
		} else {
			w.emit1Gamma(g[i])
		}
	}
}

// psSamplerCargo mirrors cmsPsSamplerCargo.
type psSamplerCargo struct {
	pipeline *stageCLutData
	w        *psWriter

	firstComponent  int
	secondComponent int

	preMaj  string
	postMaj string
	preMin  string
	postMin string

	fixWhite   bool
	colorSpace ColorSpaceSignature
}

// outputValueSampler ports OutputValueSampler. Returns true to continue the
// sweep (the reference returns 1).
func (sc *psSamplerCargo) outputValueSampler(in, out []uint16) bool {
	w := sc.w

	// The reference reads In[0..2] from a fixed MAX_INPUT_DIMENSIONS array; the
	// callers here only ever sample 3- or 4-input CLUTs, but read the first
	// three components defensively so a malformed pipeline can never panic.
	var in0, in1, in2 uint16
	if len(in) > 0 {
		in0 = in[0]
	}
	if len(in) > 1 {
		in1 = in[1]
	}
	if len(in) > 2 {
		in2 = in[2]
	}

	if sc.fixWhite {
		if in0 == 0xFFFF { // Only in L* = 100, ab = [-8..8]
			if (in1 >= 0x7800 && in1 <= 0x8800) &&
				(in2 >= 0x7800 && in2 <= 0x8800) {
				if white, _, nOutputs, ok := EndPointsBySpace(sc.colorSpace); ok {
					if nOutputs > uint32(len(out)) {
						nOutputs = uint32(len(out))
					}
					if nOutputs > uint32(len(white)) {
						nOutputs = uint32(len(white))
					}
					for i := uint32(0); i < nOutputs; i++ {
						out[i] = white[i]
					}
				}
			}
		}
	}

	// Handle the parenthesis on rows
	if int(in0) != sc.firstComponent {
		if sc.firstComponent != -1 {
			w.printf("%s", sc.postMin)
			sc.secondComponent = -1
			w.printf("%s", sc.postMaj)
		}
		// Begin block
		w.col = 0
		w.printf("%s", sc.preMaj)
		sc.firstComponent = int(in0)
	}

	if int(in1) != sc.secondComponent {
		if sc.secondComponent != -1 {
			w.printf("%s", sc.postMin)
		}
		w.printf("%s", sc.preMin)
		sc.secondComponent = int(in1)
	}

	// Dump table. We always deal with Lab4.
	n := sc.pipeline.params.NumOutputs
	if n > uint32(len(out)) {
		n = uint32(len(out))
	}
	for i := uint32(0); i < n; i++ {
		w.writeByte(word2Byte(out[i]))
	}

	return true
}

// writeCLUT ports WriteCLUT.
func (w *psWriter) writeCLUT(mpe *Stage, preMaj, postMaj, preMin, postMin string, fixWhite bool, colorSpace ColorSpaceSignature) {
	sc := psSamplerCargo{
		firstComponent:  -1,
		secondComponent: -1,
		pipeline:        mpe.CLUTData(),
		w:               w,
		preMaj:          preMaj,
		postMaj:         postMaj,
		preMin:          preMin,
		postMin:         postMin,
		fixWhite:        fixWhite,
		colorSpace:      colorSpace,
	}

	if sc.pipeline != nil && sc.pipeline.params != nil {
		w.printf("[")
		for i := uint32(0); i < sc.pipeline.params.NumInputs; i++ {
			if i < maxInputDimensions {
				w.printf(" %d ", sc.pipeline.params.NumSamples[i])
			}
		}
		w.printf(" [\n")

		mpe.SampleCLut16bit(func(in, out []uint16, _ any) bool {
			return sc.outputValueSampler(in, out)
		}, nil, samplerInspect)

		w.printf("%s", postMin)
		w.printf("%s", postMaj)
		w.printf("] ")
	}
}

// emitCIEBasedA ports EmitCIEBasedA.
func (w *psWriter) emitCIEBasedA(curve *ToneCurve, black CIEXYZ) {
	w.printf("[ /CIEBasedA\n")
	w.printf("  <<\n")
	w.printf("/DecodeA ")
	w.emit1Gamma(curve)
	w.printf(" \n")
	w.printf("/MatrixA [ 0.9642 1.0000 0.8249 ]\n")
	w.printf("/RangeLMN [ 0.0 0.9642 0.0 1.0000 0.0 0.8249 ]\n")
	w.emitWhiteBlackD50(black)
	w.emitIntent(IntentPerceptual)
	w.printf(">>\n")
	w.printf("]\n")
}

// emitCIEBasedABC ports EmitCIEBasedABC. matrix holds 9 row-major coefficients.
func (w *psWriter) emitCIEBasedABC(matrix []float64, curveSet []*ToneCurve, black CIEXYZ) {
	w.printf("[ /CIEBasedABC\n")
	w.printf("<<\n")
	w.printf("/DecodeABC [ ")
	w.emitNGamma(curveSet)
	w.printf("]\n")
	w.printf("/MatrixABC [ ")
	for i := 0; i < 3; i++ {
		w.printf("%.6f %.6f %.6f ", matrix[i+3*0], matrix[i+3*1], matrix[i+3*2])
	}
	w.printf("]\n")
	w.printf("/RangeLMN [ 0.0 0.9642 0.0 1.0000 0.0 0.8249 ]\n")
	w.emitWhiteBlackD50(black)
	w.emitIntent(IntentPerceptual)
	w.printf(">>\n")
	w.printf("]\n")
}

// emitCIEBasedDEF ports EmitCIEBasedDEF.
func (w *psWriter) emitCIEBasedDEF(pipeline *Pipeline, intent uint32, black CIEXYZ) error {
	var preMaj, postMaj, preMin, postMin string

	mpe := pipeline.GetPtrToFirstStage()
	if mpe == nil {
		return w.ctx.signalError(ErrColorspaceCheck, "Invalid pipeline for CSA")
	}

	switch mpe.InputChannelsCount() {
	case 3:
		w.printf("[ /CIEBasedDEF\n")
		preMaj = "<"
		postMaj = ">\n"
		preMin, postMin = "", ""
	case 4:
		w.printf("[ /CIEBasedDEFG\n")
		preMaj = "["
		postMaj = "]\n"
		preMin = "<"
		postMin = ">\n"
	default:
		return w.ctx.signalError(ErrColorspaceCheck, "Invalid input channels for CSA")
	}

	w.printf("<<\n")

	if mpe.StageType() == SigCurveSetElemType {
		w.printf("/DecodeDEF [ ")
		w.emitNGamma(mpe.GetToneCurves())
		w.printf("]\n")
		mpe = mpe.Next()
	}

	if mpe != nil && mpe.StageType() == SigCLutElemType {
		w.printf("/Table ")
		w.writeCLUT(mpe, preMaj, postMaj, preMin, postMin, false, ColorSpaceSignature(0))
		w.printf("]\n")
	}

	w.emitLab2XYZ()
	w.emitWhiteBlackD50(black)
	w.emitIntent(intent)

	w.printf("   >>\n")
	w.printf("]\n")
	return nil
}

// extractGray2Y ports ExtractGray2Y.
func extractGray2Y(ctx *Context, p *Profile, intent uint32) *ToneCurve {
	hXYZ, err := ctx.CreateXYZProfile()
	if err != nil || hXYZ == nil {
		return nil
	}
	xform, err := ctx.CreateTransform(p, psTypeGray8, hXYZ, typeXYZDBL, intent, FlagsNoOptimize)
	if err != nil || xform == nil {
		return nil
	}

	vals := make([]uint16, 256)
	in := make([]byte, 1)
	out := make([]byte, 24)
	for i := 0; i < 256; i++ {
		in[0] = byte(i)
		xform.DoTransform(in, out, 1)
		y := math.Float64frombits(binary.LittleEndian.Uint64(out[8:16]))
		vals[i] = quickSaturateWord(y * 65535.0)
	}

	tc, err := BuildTabulatedToneCurve16(vals)
	if err != nil {
		return nil
	}
	return tc
}

// writeInputLUT ports WriteInputLUT.
func (w *psWriter) writeInputLUT(p *Profile, intent, dwFlags uint32) error {
	ctx := w.ctx

	inputFormat := FormatterForColorspaceOfProfile(p, 2, false)
	nChannels := tChannels(inputFormat)

	blackPoint, _ := p.DetectBlackPoint(intent, 0)

	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create Lab profile")
	}

	xform, err := ctx.CreateMultiprofileTransform([]*Profile{p, hLab}, 2, inputFormat, typeLabDBL, intent, 0)
	if err != nil || xform == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create transform Profile -> Lab")
	}

	switch nChannels {
	case 1:
		gray2Y := extractGray2Y(ctx, p, intent)
		w.emitCIEBasedA(gray2Y, blackPoint)

	case 3, 4:
		outFrm := psTypeLab16
		deviceLink, derr := xform.Lut.Dup()
		if derr != nil || deviceLink == nil {
			return ctx.signalError(ErrColorspaceCheck, "Cannot duplicate device link")
		}
		inFmt := inputFormat
		fl := dwFlags | FlagsForceCLUT
		ctx.optimizePipeline(&deviceLink, intent, &inFmt, &outFrm, &fl)
		if err := w.emitCIEBasedDEF(deviceLink, intent, blackPoint); err != nil {
			return err
		}

	default:
		return ctx.signalError(ErrColorspaceCheck,
			"Only 3, 4 channels are supported for CSA. This profile has %d channels.", nChannels)
	}

	return nil
}

// writeInputMatrixShaper ports WriteInputMatrixShaper.
func (w *psWriter) writeInputMatrixShaper(p *Profile, matrix, shaper *Stage) error {
	ctx := w.ctx
	colorSpace := p.GetColorSpace()
	blackPoint, _ := p.DetectBlackPoint(IntentRelativeColorimetric, 0)

	switch colorSpace {
	case SigGrayData:
		shaperCurve := shaper.GetToneCurves()
		if len(shaperCurve) == 0 {
			return ctx.signalError(ErrColorspaceCheck, "Missing shaper curve")
		}
		w.emitCIEBasedA(shaperCurve[0], blackPoint)

	case SigRgbData:
		double, _ := matrix.MatrixData()
		if len(double) < 9 {
			return ctx.signalError(ErrColorspaceCheck, "Malformed matrix stage")
		}
		mat := make([]float64, 9)
		for i := 0; i < 9; i++ {
			mat[i] = double[i] * maxEncodeableXYZ
		}
		w.emitCIEBasedABC(mat, shaper.GetToneCurves(), blackPoint)

	default:
		return ctx.signalError(ErrColorspaceCheck, "Profile is not suitable for CSA. Unsupported colorspace.")
	}

	return nil
}

// writeNamedColorCSA ports WriteNamedColorCSA.
func (w *psWriter) writeNamedColorCSA(hNamedColor *Profile, intent uint32) error {
	ctx := w.ctx
	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create Lab profile")
	}
	xform, err := ctx.CreateTransform(hNamedColor, psTypeNamedColorIdx, hLab, typeLabDBL, intent, 0)
	if err != nil || xform == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create named color transform")
	}

	ncl := xform.GetNamedColorList()
	if ncl == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot access named color list")
	}

	w.printf("<<\n")
	w.printf("(colorlistcomment) (%s)\n", "Named color CSA")
	w.printf("(Prefix) [ (Pantone ) (PANTONE ) ]\n")
	w.printf("(Suffix) [ ( CV) ( CVC) ( C) ]\n")

	nColors := ncl.Count()
	in := make([]byte, 2)
	out := make([]byte, 24)
	for i := uint32(0); i < nColors; i++ {
		name, _, _, _, _, ok := ncl.Info(i)
		if !ok {
			continue
		}
		binary.LittleEndian.PutUint16(in, uint16(i))
		xform.DoTransform(in, out, 1)
		l := math.Float64frombits(binary.LittleEndian.Uint64(out[0:8]))
		a := math.Float64frombits(binary.LittleEndian.Uint64(out[8:16]))
		b := math.Float64frombits(binary.LittleEndian.Uint64(out[16:24]))

		w.printf("  (")
		w.emitPSEscaped(name)
		w.printf(") [ %.3f %.3f %.3f ]\n", l, a, b)
	}

	w.printf(">>\n")
	return nil
}

// generateCSA ports GenerateCSA.
func (w *psWriter) generateCSA(p *Profile, intent, dwFlags uint32) error {
	ctx := w.ctx

	if p.GetDeviceClass() == SigNamedColorClass {
		return w.writeNamedColorCSA(p, intent)
	}

	// Output (PCS) colorspace must be XYZ or Lab
	pcs := p.GetPCS()
	if pcs != SigXYZData && pcs != SigLabData {
		return ctx.signalError(ErrColorspaceCheck, "Invalid output color space")
	}

	lut, err := p.ReadInputLUT(intent)
	if err != nil || lut == nil {
		if err == nil {
			err = ctx.signalError(ErrColorspaceCheck, "Cannot read input LUT")
		}
		return err
	}

	// Tone curves + matrix can be implemented without any LUT
	if stages, ok := lut.CheckAndRetrieveStages(SigCurveSetElemType, SigMatrixElemType); ok {
		shaper, matrix := stages[0], stages[1]
		return w.writeInputMatrixShaper(p, matrix, shaper)
	}

	return w.writeInputLUT(p, intent, dwFlags)
}

// ---------------------------- CRD ------------------------------------------

// emitPQRStage ports EmitPQRStage.
func (w *psWriter) emitPQRStage(p *Profile, doBPC, isAbsolute bool) {
	if isAbsolute {
		// For absolute colorimetric intent, encode back to relative
		white := p.readMediaWhitePoint()

		w.printf("/MatrixPQR [1 0 0 0 1 0 0 0 1 ]\n")
		w.printf("/RangePQR [ -0.5 2 -0.5 2 -0.5 2 ]\n")

		w.printf("%% Absolute colorimetric -- encode to relative to maximize LUT usage\n"+
			"/TransformPQR [\n"+
			"{0.9642 mul %s div exch pop exch pop exch pop exch pop} bind\n"+
			"{1.0000 mul %s div exch pop exch pop exch pop exch pop} bind\n"+
			"{0.8249 mul %s div exch pop exch pop exch pop exch pop} bind\n]\n",
			g6(white.X), g6(white.Y), g6(white.Z))
		return
	}

	w.printf("%% Bradford Cone Space\n" +
		"/MatrixPQR [0.8951 -0.7502 0.0389 0.2664 1.7135 -0.0685 -0.1614 0.0367 1.0296 ] \n")

	w.printf("/RangePQR [ -0.5 2 -0.5 2 -0.5 2 ]\n")

	if !doBPC {
		w.printf("%% VonKries-like transform in Bradford Cone Space\n" +
			"/TransformPQR [\n" +
			"{exch pop exch 3 get mul exch pop exch 3 get div} bind\n" +
			"{exch pop exch 4 get mul exch pop exch 4 get div} bind\n" +
			"{exch pop exch 5 get mul exch pop exch 5 get div} bind\n]\n")
	} else {
		w.printf("%% VonKries-like transform in Bradford Cone Space plus BPC\n" +
			"/TransformPQR [\n")

		w.printf("{4 index 3 get div 2 index 3 get mul " +
			"2 index 3 get 2 index 3 get sub mul " +
			"2 index 3 get 4 index 3 get 3 index 3 get sub mul sub " +
			"3 index 3 get 3 index 3 get exch sub div " +
			"exch pop exch pop exch pop exch pop } bind\n")

		w.printf("{4 index 4 get div 2 index 4 get mul " +
			"2 index 4 get 2 index 4 get sub mul " +
			"2 index 4 get 4 index 4 get 3 index 4 get sub mul sub " +
			"3 index 4 get 3 index 4 get exch sub div " +
			"exch pop exch pop exch pop exch pop } bind\n")

		w.printf("{4 index 5 get div 2 index 5 get mul " +
			"2 index 5 get 2 index 5 get sub mul " +
			"2 index 5 get 4 index 5 get 3 index 5 get sub mul sub " +
			"3 index 5 get 3 index 5 get exch sub div " +
			"exch pop exch pop exch pop exch pop } bind\n]\n")
	}
}

// emitXYZ2Lab ports EmitXYZ2Lab.
func (w *psWriter) emitXYZ2Lab() {
	w.printf("/RangeLMN [ -0.635 2.0 0 2 -0.635 2.0 ]\n")
	w.printf("/EncodeLMN [\n")
	w.printf("{ 0.964200  div dup 0.008856 le {7.787 mul 16 116 div add}{1 3 div exp} ifelse } bind\n")
	w.printf("{ 1.000000  div dup 0.008856 le {7.787 mul 16 116 div add}{1 3 div exp} ifelse } bind\n")
	w.printf("{ 0.824900  div dup 0.008856 le {7.787 mul 16 116 div add}{1 3 div exp} ifelse } bind\n")
	w.printf("]\n")
	w.printf("/MatrixABC [ 0 1 0 1 -1 1 0 0 -1 ]\n")
	w.printf("/EncodeABC [\n")
	w.printf("{ 116 mul  16 sub 100 div  } bind\n")
	w.printf("{ 500 mul 128 add 256 div  } bind\n")
	w.printf("{ 200 mul 128 add 256 div  } bind\n")
	w.printf("]\n")
}

// writeOutputLUT ports WriteOutputLUT.
func (w *psWriter) writeOutputLUT(p *Profile, intent, dwFlags uint32) error {
	ctx := w.ctx

	doBPC := dwFlags&FlagsBlackPointCompensation != 0
	fixWhite := dwFlags&FlagsNoWhiteOnWhiteFixup == 0

	hLab, err := ctx.CreateLab4Profile(nil)
	if err != nil || hLab == nil {
		return falseErr(ctx)
	}

	outputFormat := FormatterForColorspaceOfProfile(p, 2, false)
	nChannels := tChannels(outputFormat)
	colorSpace := p.GetColorSpace()

	relativeEncodingIntent := intent
	if relativeEncodingIntent == IntentAbsoluteColorimetric {
		relativeEncodingIntent = IntentRelativeColorimetric
	}

	xform, err := ctx.CreateMultiprofileTransform([]*Profile{hLab, p}, 2, typeLabDBL, outputFormat, relativeEncodingIntent, 0)
	if err != nil || xform == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create transform Lab -> Profile in CRD creation")
	}

	deviceLink, derr := xform.Lut.Dup()
	if derr != nil || deviceLink == nil {
		return ctx.signalError(ErrCorruptionDetected, "Cannot access link for CRD")
	}

	inFrm := psTypeLab16
	fl := dwFlags | FlagsForceCLUT
	if !ctx.optimizePipeline(&deviceLink, relativeEncodingIntent, &inFrm, &outputFormat, &fl) {
		return ctx.signalError(ErrCorruptionDetected, "Cannot create CLUT table for CRD")
	}

	w.printf("<<\n")
	w.printf("/ColorRenderingType 1\n")

	blackPoint, _ := p.DetectBlackPoint(intent, 0)

	w.emitWhiteBlackD50(blackPoint)
	w.emitPQRStage(p, doBPC, intent == IntentAbsoluteColorimetric)
	w.emitXYZ2Lab()

	if intent == IntentAbsoluteColorimetric {
		fixWhite = false
	}

	w.printf("/RenderTable ")

	first := deviceLink.GetPtrToFirstStage()
	if first != nil {
		if first.StageType() != SigCLutElemType {
			return ctx.signalError(ErrCorruptionDetected, "Cannot create CLUT, revise your flags!")
		}
		w.writeCLUT(first, "<", ">\n", "", "", fixWhite, colorSpace)
	}

	w.printf(" %d {} bind ", nChannels)

	for i := uint32(1); i < nChannels; i++ {
		w.printf("dup ")
	}

	w.printf("]\n")

	w.emitIntent(intent)

	w.printf(">>\n")

	if dwFlags&FlagsNoDefaultResourceDef == 0 {
		w.printf("/Current exch /ColorRendering defineresource pop\n")
	}

	return nil
}

// falseErr mirrors the reference returning FALSE with no signalled message
// (the hLab == NULL path of WriteOutputLUT).
func falseErr(ctx *Context) error {
	return ctx.signalError(ErrColorspaceCheck, "Cannot create Lab profile")
}

// buildColorantList ports BuildColorantList.
func buildColorantList(nColorant uint32, out []uint16) string {
	if nColorant > maxChannels {
		nColorant = maxChannels
	}
	var b strings.Builder
	for j := uint32(0); j < nColorant; j++ {
		b.WriteString(fmt.Sprintf("%.3f", float64(out[j])/65535.0))
		if j < nColorant-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

// writeNamedColorCRD ports WriteNamedColorCRD.
func (w *psWriter) writeNamedColorCRD(hNamedColor *Profile, intent, dwFlags uint32) error {
	ctx := w.ctx

	outputFormat := FormatterForColorspaceOfProfile(hNamedColor, 2, false)
	nColorant := tChannels(outputFormat)

	xform, err := ctx.CreateTransform(hNamedColor, psTypeNamedColorIdx, nil, outputFormat, intent, dwFlags)
	if err != nil || xform == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot create named color transform")
	}

	ncl := xform.GetNamedColorList()
	if ncl == nil {
		return ctx.signalError(ErrColorspaceCheck, "Cannot access named color list")
	}

	w.printf("<<\n")
	w.printf("(colorlistcomment) (%s) \n", "Named profile")
	w.printf("(Prefix) [ (Pantone ) (PANTONE ) ]\n")
	w.printf("(Suffix) [ ( CV) ( CVC) ( C) ]\n")

	nColors := ncl.Count()
	in := make([]byte, 2)
	out := make([]byte, int(nColorant)*2)
	for i := uint32(0); i < nColors; i++ {
		name, _, _, _, _, ok := ncl.Info(i)
		if !ok {
			continue
		}
		binary.LittleEndian.PutUint16(in, uint16(i))
		xform.DoTransform(in, out, 1)
		colorant := make([]uint16, nColorant)
		for c := uint32(0); c < nColorant; c++ {
			colorant[c] = binary.LittleEndian.Uint16(out[c*2:])
		}
		list := buildColorantList(nColorant, colorant)

		w.printf("  (")
		w.emitPSEscaped(name)
		w.printf(") [ %s ]\n", list)
	}

	w.printf("   >>")

	if dwFlags&FlagsNoDefaultResourceDef == 0 {
		w.printf(" /Current exch /HPSpotTable defineresource pop\n")
	}

	return nil
}

// generateCRD ports GenerateCRD.
func (w *psWriter) generateCRD(p *Profile, intent, dwFlags uint32) error {
	if dwFlags&FlagsNoDefaultResourceDef == 0 {
		w.emitHeader("Color Rendering Dictionary (CRD)", p)
	}

	if p.GetDeviceClass() == SigNamedColorClass {
		if err := w.writeNamedColorCRD(p, intent, dwFlags); err != nil {
			return err
		}
	} else {
		if err := w.writeOutputLUT(p, intent, dwFlags); err != nil {
			return err
		}
	}

	if dwFlags&FlagsNoDefaultResourceDef == 0 {
		w.printf("%%%%EndResource\n")
		w.printf("\n%% CRD End\n")
	}

	return nil
}

// ---------------------------- public API -----------------------------------

// GetPostScriptColorResource ports cmsGetPostScriptColorResource: it generates
// the requested PostScript colour resource (CSA or CRD) for the profile and
// returns the generated bytes.
func (ctx *Context) GetPostScriptColorResource(typ PSResourceType, p *Profile, intent, dwFlags uint32) ([]byte, error) {
	if p == nil {
		return nil, ctx.signalError(ErrColorspaceCheck, "nil profile")
	}
	w := &psWriter{ctx: ctx}

	var err error
	switch typ {
	case PSResourceCSA:
		err = w.generateCSA(p, intent, dwFlags)
	default:
		err = w.generateCRD(p, intent, dwFlags)
	}
	if err != nil {
		return nil, err
	}
	if w.err {
		return nil, ctx.signalError(ErrColorspaceCheck, "PostScript output truncated")
	}
	return w.buf.Bytes(), nil
}

// GetPostScriptCSA ports cmsGetPostScriptCSA: it returns the ColorSpaceArray as
// bytes.
func (ctx *Context) GetPostScriptCSA(p *Profile, intent, dwFlags uint32) ([]byte, error) {
	return ctx.GetPostScriptColorResource(PSResourceCSA, p, intent, dwFlags)
}

// GetPostScriptCRD ports cmsGetPostScriptCRD: it returns the
// ColorRenderingDictionary as bytes.
func (ctx *Context) GetPostScriptCRD(p *Profile, intent, dwFlags uint32) ([]byte, error) {
	return ctx.GetPostScriptColorResource(PSResourceCRD, p, intent, dwFlags)
}

// GetPostScriptColorResource is the default-context form.
func GetPostScriptColorResource(typ PSResourceType, p *Profile, intent, dwFlags uint32) ([]byte, error) {
	return profileContextID(p).GetPostScriptColorResource(typ, p, intent, dwFlags)
}

// GetPostScriptCSA is the default-context form of (*Context).GetPostScriptCSA.
func GetPostScriptCSA(p *Profile, intent, dwFlags uint32) ([]byte, error) {
	return profileContextID(p).GetPostScriptCSA(p, intent, dwFlags)
}

// GetPostScriptCRD is the default-context form of (*Context).GetPostScriptCRD.
func GetPostScriptCRD(p *Profile, intent, dwFlags uint32) ([]byte, error) {
	return profileContextID(p).GetPostScriptCRD(p, intent, dwFlags)
}
