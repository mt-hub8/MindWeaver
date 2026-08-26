// Port of src/cmsio1.c: building input/output/devicelink pipelines from a
// profile, plus the capability queries (cmsIsMatrixShaper / cmsIsCLUT /
// cmsIsIntentSupported), profile-info accessors, and the profile-sequence
// read/write/compile helpers.
//
// The functions here read cooked tags (via ReadTag) and assemble cmsPipelines
// with the version/encoding fix-ups the reference performs: Lab V2<->V4 matrix
// wrapping for mft2 LUTs, the InpAdj/OutpAdj 1.15-fixed-point scaling of the
// matrix-shaper matrices, the float DToB/BToD normalization stages, and the
// perceptual fallback when an intent's tag is absent.
//
// Naming: the C names map to Go as
//
//	_cmsReadInputLUT      -> (*Profile).ReadInputLUT
//	_cmsReadOutputLUT     -> (*Profile).ReadOutputLUT
//	_cmsReadDevicelinkLUT -> (*Profile).ReadDevicelinkLUT
//	cmsIsMatrixShaper     -> (*Profile).IsMatrixShaper
//	cmsIsCLUT             -> (*Profile).IsCLUT
//	cmsIsIntentSupported  -> (*Profile).IsIntentSupported
//	_cmsReadMediaWhitePoint -> (*Profile).readMediaWhitePoint
//	_cmsReadCHAD            -> (*Profile).readCHAD
//	_cmsReadProfileSequence -> (*Profile).ReadProfileSequence
//	_cmsWriteProfileSequence-> (*Profile).WriteProfileSequence
//	_cmsCompileProfileSequence -> (*Context).CompileProfileSequence
//	cmsGetProfileInfo/ASCII/UTF8 -> (*Profile).GetProfileInfo{,ASCII,UTF8}
//
// PORTNOTES:
//   - The pipeline builders return (*Pipeline, error). A nil pipeline with a
//     non-nil error mirrors the reference's uniform "return NULL": callers treat
//     any nil result as failure. The error carries a diagnostic reason.
//   - Everything in cmsio1.c is self-contained relative to this port's existing
//     modules (io0/lut/gamma/named/mtrx/pcs/wtpnt). No function here depends on
//     cnvrt/xform, so the whole file is ported.

package lcms2

// Rendering intents, mirroring the INTENT_* macros in include/lcms2.h.
const (
	IntentPerceptual           uint32 = 0
	IntentRelativeColorimetric uint32 = 1
	IntentSaturation           uint32 = 2
	IntentAbsoluteColorimetric uint32 = 3
)

// LUT usage directions, mirroring LCMS_USED_AS_INPUT/OUTPUT/PROOF.
const (
	UsedAsInput  uint32 = 0
	UsedAsOutput uint32 = 1
	UsedAsProof  uint32 = 2
)

// InfoType selects which descriptive MLU tag a profile-info accessor reads,
// mirroring cmsInfoType.
type InfoType int

const (
	InfoDescription InfoType = iota
	InfoManufacturer
	InfoModel
	InfoCopyright
)

// Per-intent LUT tag tables, mirroring the cmsio1.c statics. Absolute
// colorimetric reuses the relative-colorimetric tag, exactly as the reference.
var (
	device2PCS16 = [4]TagSignature{
		SigAToB0Tag, // Perceptual
		SigAToB1Tag, // Relative colorimetric
		SigAToB2Tag, // Saturation
		SigAToB1Tag, // Absolute colorimetric
	}
	device2PCSFloat = [4]TagSignature{
		SigDToB0Tag, SigDToB1Tag, SigDToB2Tag, SigDToB3Tag,
	}
	pcs2Device16 = [4]TagSignature{
		SigBToA0Tag, SigBToA1Tag, SigBToA2Tag, SigBToA1Tag,
	}
	pcs2DeviceFloat = [4]TagSignature{
		SigBToD0Tag, SigBToD1Tag, SigBToD2Tag, SigBToD3Tag,
	}
)

// Factors to convert from 1.15 fixed point to 0..1.0 range and vice-versa.
const (
	inpAdj = 1.0 / maxEncodeableXYZ // (65536.0/(65535.0*2.0))
	outAdj = maxEncodeableXYZ       // ((2.0*65535.0)/65536.0)
)

// Gray-conversion resources, mirroring the cmsio1.c statics.
var (
	grayInputMatrix       = []float64{inpAdj * D50X, inpAdj * D50Y, inpAdj * D50Z}
	oneToThreeInputMatrix = []float64{1, 1, 1}
	pickYMatrix           = []float64{0, outAdj * D50Y, 0}
	pickLstarMatrix       = []float64{1, 0, 0}
)

// readTagPtr returns the cooked value of a tag, or nil when the tag is absent or
// cannot be read — mirroring cmsReadTag's NULL return.
func (p *Profile) readTagPtr(sig TagSignature) any {
	v, err := p.ReadTag(sig)
	if err != nil || v == nil {
		return nil
	}
	return v
}

// ---------------------------------------------------------------------------
// White point / CHAD
// ---------------------------------------------------------------------------

// readMediaWhitePoint ports _cmsReadMediaWhitePoint: the media white point with
// the old-profile fix-ups (missing tag or a V2 display profile => D50).
func (p *Profile) readMediaWhitePoint() CIEXYZ {
	tag, _ := p.readTagPtr(SigMediaWhitePointTag).(*CIEXYZ)
	if tag == nil {
		return D50XYZ()
	}
	if p.GetEncodedICCversion() < 0x4000000 && p.GetDeviceClass() == SigDisplayClass {
		return D50XYZ()
	}
	return *tag
}

// readCHAD ports _cmsReadCHAD: the chromatic adaptation matrix, defaulting to
// identity (with the V2-display adaptation fix-up). The chromaticAdaptation tag
// is stored as a 9-element s15Fixed16 array which the reference reinterprets as
// a cmsMAT3 (row-major).
func (p *Profile) readCHAD() (MAT3, bool) {
	if arr, ok := p.readTagPtr(SigChromaticAdaptationTag).([]float64); ok && len(arr) >= 9 {
		return MAT3{
			{arr[0], arr[1], arr[2]},
			{arr[3], arr[4], arr[5]},
			{arr[6], arr[7], arr[8]},
		}, true
	}

	dest := MAT3Identity()

	if p.GetEncodedICCversion() < 0x4000000 && p.GetDeviceClass() == SigDisplayClass {
		white, _ := p.readTagPtr(SigMediaWhitePointTag).(*CIEXYZ)
		if white == nil {
			return MAT3Identity(), true
		}
		return adaptationMatrix(nil, *white, D50XYZ())
	}
	return dest, true
}

// readICCMatrixRGB2XYZ ports ReadICCMatrixRGB2XYZ: read the RGB colorant tags
// into a MAT3 whose columns are the red/green/blue XYZ tristimulus values.
func (p *Profile) readICCMatrixRGB2XYZ() (MAT3, bool) {
	red, _ := p.readTagPtr(SigRedColorantTag).(*CIEXYZ)
	green, _ := p.readTagPtr(SigGreenColorantTag).(*CIEXYZ)
	blue, _ := p.readTagPtr(SigBlueColorantTag).(*CIEXYZ)
	if red == nil || green == nil || blue == nil {
		return MAT3{}, false
	}
	return MAT3{
		{red.X, green.X, blue.X},
		{red.Y, green.Y, blue.Y},
		{red.Z, green.Z, blue.Z},
	}, true
}

// matFlat flattens a MAT3 to the row-major []float64 that StageAllocMatrix wants.
func matFlat(m MAT3) []float64 {
	return []float64{
		m[0][0], m[0][1], m[0][2],
		m[1][0], m[1][1], m[1][2],
		m[2][0], m[2][1], m[2][2],
	}
}

// ---------------------------------------------------------------------------
// Input pipelines
// ---------------------------------------------------------------------------

// buildGrayInputMatrixPipeline ports BuildGrayInputMatrixPipeline.
func (p *Profile) buildGrayInputMatrixPipeline() (*Pipeline, error) {
	ctx := p.ContextID
	grayTRC, _ := p.readTagPtr(SigGrayTRCTag).(*ToneCurve)
	if grayTRC == nil {
		return nil, errorf(ErrCorruptionDetected, "gray input: no grayTRC")
	}

	lut, err := ctx.PipelineAlloc(1, 3)
	if err != nil {
		return nil, err
	}

	if p.GetPCS() == SigLabData {
		// Identity matrix plus 3 tone curves.
		emptyTab, err := ctx.BuildTabulatedToneCurve16([]uint16{0x8080, 0x8080})
		if err != nil {
			lut.Free()
			return nil, err
		}
		labCurves := []*ToneCurve{grayTRC, emptyTab, emptyTab}
		m, err := ctx.StageAllocMatrix(3, 1, oneToThreeInputMatrix, nil)
		if err != nil {
			lut.Free()
			return nil, err
		}
		tc, err := ctx.StageAllocToneCurves(3, labCurves)
		if err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, m); err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, tc); err != nil {
			lut.Free()
			return nil, err
		}
	} else {
		tc, err := ctx.StageAllocToneCurves(1, []*ToneCurve{grayTRC})
		if err != nil {
			lut.Free()
			return nil, err
		}
		m, err := ctx.StageAllocMatrix(3, 1, grayInputMatrix, nil)
		if err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, tc); err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, m); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// buildRGBInputMatrixShaper ports BuildRGBInputMatrixShaper.
func (p *Profile) buildRGBInputMatrixShaper() (*Pipeline, error) {
	ctx := p.ContextID
	mat, ok := p.readICCMatrixRGB2XYZ()
	if !ok {
		return nil, errorf(ErrCorruptionDetected, "RGB input: missing colorant tags")
	}

	// Scale the matrix into 1.15 fixed-point PCS encoding.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			mat[i][j] *= inpAdj
		}
	}

	shapes := [3]*ToneCurve{}
	shapes[0], _ = p.readTagPtr(SigRedTRCTag).(*ToneCurve)
	shapes[1], _ = p.readTagPtr(SigGreenTRCTag).(*ToneCurve)
	shapes[2], _ = p.readTagPtr(SigBlueTRCTag).(*ToneCurve)
	if shapes[0] == nil || shapes[1] == nil || shapes[2] == nil {
		return nil, errorf(ErrCorruptionDetected, "RGB input: missing TRC tags")
	}

	lut, err := ctx.PipelineAlloc(3, 3)
	if err != nil {
		return nil, err
	}

	tc, err := ctx.StageAllocToneCurves(3, shapes[:])
	if err != nil {
		lut.Free()
		return nil, err
	}
	m, err := ctx.StageAllocMatrix(3, 3, matFlat(mat), nil)
	if err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, tc); err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, m); err != nil {
		lut.Free()
		return nil, err
	}

	// Tolerant handling of a matrix-shaper feeding a Lab PCS (out of spec).
	if p.GetPCS() == SigLabData {
		if err := lut.InsertStage(AtEnd, ctx.stageAllocXYZ2Lab()); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// readFloatInputTag ports _cmsReadFloatInputTag: dup a DToB tag and wrap it with
// the Lab/XYZ normalization stages for both the device and PCS sides.
func (p *Profile) readFloatInputTag(tagFloat TagSignature) (*Pipeline, error) {
	ctx := p.ContextID
	src, _ := p.readTagPtr(tagFloat).(*Pipeline)
	if src == nil {
		return nil, errorf(ErrCorruptionDetected, "float input: tag not a pipeline")
	}
	lut, err := src.Dup()
	if err != nil {
		return nil, err
	}

	spc := p.GetColorSpace()
	pcs := p.GetPCS()

	if spc == SigLabData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if spc == SigXYZData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}

	if pcs == SigLabData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if pcs == SigXYZData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// insertNormStage is a small helper to allocate a normalization stage and insert
// it, freeing nothing on error (the caller frees the pipeline).
func insertNormStage(lut *Pipeline, loc StageLoc, alloc func() (*Stage, error)) error {
	st, err := alloc()
	if err != nil {
		return err
	}
	return lut.InsertStage(loc, st)
}

// ReadInputLUT ports _cmsReadInputLUT: build the device->PCS pipeline for an
// intent, handling named-color profiles, float tags, Lab V2/V4 fix-ups, and the
// matrix-shaper fallbacks. Intent > INTENT_ABSOLUTE_COLORIMETRIC forces the
// matrix-shaper path (used with 0xffffffff).
func (p *Profile) ReadInputLUT(intent uint32) (*Pipeline, error) {
	ctx := p.ContextID

	// Named color takes the appropriate tag.
	if p.GetDeviceClass() == SigNamedColorClass {
		nc, _ := p.readTagPtr(SigNamedColor2Tag).(*NamedColorList)
		if nc == nil {
			return nil, errorf(ErrCorruptionDetected, "input LUT: no named color list")
		}
		lut, err := ctx.PipelineAlloc(0, 0)
		if err != nil {
			return nil, err
		}
		if err := lut.InsertStage(AtBegin, stageAllocNamedColor(nc, true)); err != nil {
			lut.Free()
			return nil, err
		}
		v2v4, err := ctx.stageAllocLabV2ToV4()
		if err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, v2v4); err != nil {
			lut.Free()
			return nil, err
		}
		return lut, nil
	}

	if intent <= IntentAbsoluteColorimetric {
		tag16 := device2PCS16[intent]
		tagFloat := device2PCSFloat[intent]

		if p.IsTag(tagFloat) { // Float tag takes precedence
			return p.readFloatInputTag(tagFloat)
		}

		// Revert to perceptual if no tag is found.
		if !p.IsTag(tag16) {
			tag16 = device2PCS16[0]
		}

		if p.IsTag(tag16) {
			src, _ := p.readTagPtr(tag16).(*Pipeline)
			if src == nil {
				return nil, errorf(ErrCorruptionDetected, "input LUT: tag not a pipeline")
			}
			originalType := p.GetTagTrueType(tag16)

			lut, err := src.Dup()
			if err != nil {
				return nil, err
			}

			// Adjust only for Lab16 on output.
			if originalType != SigLut16Type || p.GetPCS() != SigLabData {
				return lut, nil
			}

			// If input is Lab, add a V4->V2 conversion at the begin.
			if p.GetColorSpace() == SigLabData {
				v4v2, err := ctx.stageAllocLabV4ToV2()
				if err != nil {
					lut.Free()
					return nil, err
				}
				if err := lut.InsertStage(AtBegin, v4v2); err != nil {
					lut.Free()
					return nil, err
				}
			}
			// Add the V2->V4 Lab PCS matrix at the end.
			v2v4, err := ctx.stageAllocLabV2ToV4()
			if err != nil {
				lut.Free()
				return nil, err
			}
			if err := lut.InsertStage(AtEnd, v2v4); err != nil {
				lut.Free()
				return nil, err
			}
			return lut, nil
		}
	}

	// LUT not found: build a matrix-shaper.
	if p.GetColorSpace() == SigGrayData {
		return p.buildGrayInputMatrixPipeline()
	}
	return p.buildRGBInputMatrixShaper()
}

// ---------------------------------------------------------------------------
// Output pipelines
// ---------------------------------------------------------------------------

// changeInterpolationToTrilinear ports ChangeInterpolationToTrilinear: force the
// CLUT stages of a pipeline to trilinear interpolation. It updates the interp
// flags, re-selects the generic interpolator, and re-classifies the concrete
// interpolator kind so the fast path also uses trilinear.
func changeInterpolationToTrilinear(lut *Pipeline) {
	ctx := lut.ContextID()
	for stage := lut.GetPtrToFirstStage(); stage != nil; stage = stage.Next() {
		if stage.StageType() != SigCLutElemType {
			continue
		}
		clut := stage.CLUTData()
		if clut == nil || clut.params == nil {
			continue
		}
		clut.params.Flags |= cmsLerpFlagsTrilinear
		ctx.setInterpolationRoutine(clut.params)
		clut.kind = classifyInterpKind(ctx, clut.params.NumInputs, clut.params.NumOutputs, clut.params.Flags)
		stage.concrete = clut.kind != genericKind
	}
}

// buildGrayOutputPipeline ports BuildGrayOutputPipeline.
func (p *Profile) buildGrayOutputPipeline() (*Pipeline, error) {
	ctx := p.ContextID
	grayTRC, _ := p.readTagPtr(SigGrayTRCTag).(*ToneCurve)
	if grayTRC == nil {
		return nil, errorf(ErrCorruptionDetected, "gray output: no grayTRC")
	}
	revGrayTRC, err := grayTRC.Reverse()
	if err != nil || revGrayTRC == nil {
		return nil, errorf(ErrCorruptionDetected, "gray output: cannot reverse grayTRC")
	}

	lut, err := ctx.PipelineAlloc(3, 1)
	if err != nil {
		return nil, err
	}

	var mtx []float64
	if p.GetPCS() == SigLabData {
		mtx = pickLstarMatrix
	} else {
		mtx = pickYMatrix
	}
	m, err := ctx.StageAllocMatrix(1, 3, mtx, nil)
	if err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, m); err != nil {
		lut.Free()
		return nil, err
	}
	tc, err := ctx.StageAllocToneCurves(1, []*ToneCurve{revGrayTRC})
	if err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, tc); err != nil {
		lut.Free()
		return nil, err
	}
	return lut, nil
}

// buildRGBOutputMatrixShaper ports BuildRGBOutputMatrixShaper.
func (p *Profile) buildRGBOutputMatrixShaper() (*Pipeline, error) {
	ctx := p.ContextID
	mat, ok := p.readICCMatrixRGB2XYZ()
	if !ok {
		return nil, errorf(ErrCorruptionDetected, "RGB output: missing colorant tags")
	}
	inv, ok := MAT3Inverse(mat)
	if !ok {
		return nil, errorf(ErrCorruptionDetected, "RGB output: singular colorant matrix")
	}

	// Scale the inverse matrix out of 1.15 fixed-point PCS encoding.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			inv[i][j] *= outAdj
		}
	}

	shapes := [3]*ToneCurve{}
	shapes[0], _ = p.readTagPtr(SigRedTRCTag).(*ToneCurve)
	shapes[1], _ = p.readTagPtr(SigGreenTRCTag).(*ToneCurve)
	shapes[2], _ = p.readTagPtr(SigBlueTRCTag).(*ToneCurve)
	if shapes[0] == nil || shapes[1] == nil || shapes[2] == nil {
		return nil, errorf(ErrCorruptionDetected, "RGB output: missing TRC tags")
	}

	invShapes := [3]*ToneCurve{}
	for i := 0; i < 3; i++ {
		rev, err := shapes[i].Reverse()
		if err != nil || rev == nil {
			return nil, errorf(ErrCorruptionDetected, "RGB output: cannot reverse TRC")
		}
		invShapes[i] = rev
	}

	lut, err := ctx.PipelineAlloc(3, 3)
	if err != nil {
		return nil, err
	}

	// Tolerant handling of a Lab PCS (out of spec).
	if p.GetPCS() == SigLabData {
		if err := lut.InsertStage(AtEnd, ctx.stageAllocLab2XYZ()); err != nil {
			lut.Free()
			return nil, err
		}
	}
	m, err := ctx.StageAllocMatrix(3, 3, matFlat(inv), nil)
	if err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, m); err != nil {
		lut.Free()
		return nil, err
	}
	tc, err := ctx.StageAllocToneCurves(3, invShapes[:])
	if err != nil {
		lut.Free()
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, tc); err != nil {
		lut.Free()
		return nil, err
	}
	return lut, nil
}

// readFloatOutputTag ports _cmsReadFloatOutputTag.
func (p *Profile) readFloatOutputTag(tagFloat TagSignature) (*Pipeline, error) {
	ctx := p.ContextID
	src, _ := p.readTagPtr(tagFloat).(*Pipeline)
	if src == nil {
		return nil, errorf(ErrCorruptionDetected, "float output: tag not a pipeline")
	}
	lut, err := src.Dup()
	if err != nil {
		return nil, err
	}

	pcs := p.GetPCS()
	dataSpace := p.GetColorSpace()

	if pcs == SigLabData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if pcs == SigXYZData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}

	if dataSpace == SigLabData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if dataSpace == SigXYZData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// ReadOutputLUT ports _cmsReadOutputLUT: build the PCS->device pipeline.
func (p *Profile) ReadOutputLUT(intent uint32) (*Pipeline, error) {
	ctx := p.ContextID

	if intent <= IntentAbsoluteColorimetric {
		tag16 := pcs2Device16[intent]
		tagFloat := pcs2DeviceFloat[intent]

		if p.IsTag(tagFloat) { // Float tag takes precedence
			return p.readFloatOutputTag(tagFloat)
		}

		if !p.IsTag(tag16) {
			tag16 = pcs2Device16[0]
		}

		if p.IsTag(tag16) {
			src, _ := p.readTagPtr(tag16).(*Pipeline)
			if src == nil {
				return nil, errorf(ErrCorruptionDetected, "output LUT: tag not a pipeline")
			}
			originalType := p.GetTagTrueType(tag16)

			lut, err := src.Dup()
			if err != nil {
				return nil, err
			}

			// Lab as indexer space => trilinear interpolation.
			if p.GetPCS() == SigLabData {
				changeInterpolationToTrilinear(lut)
			}

			// Adjust only for Lab and Lut16 type.
			if originalType != SigLut16Type || p.GetPCS() != SigLabData {
				return lut, nil
			}

			// Add a V4->V2 Lab PCS matrix at the begin.
			v4v2, err := ctx.stageAllocLabV4ToV2()
			if err != nil {
				lut.Free()
				return nil, err
			}
			if err := lut.InsertStage(AtBegin, v4v2); err != nil {
				lut.Free()
				return nil, err
			}
			// If output is Lab, add a V2->V4 conversion at the end.
			if p.GetColorSpace() == SigLabData {
				v2v4, err := ctx.stageAllocLabV2ToV4()
				if err != nil {
					lut.Free()
					return nil, err
				}
				if err := lut.InsertStage(AtEnd, v2v4); err != nil {
					lut.Free()
					return nil, err
				}
			}
			return lut, nil
		}
	}

	// LUT not found: build a matrix-shaper.
	if p.GetColorSpace() == SigGrayData {
		return p.buildGrayOutputPipeline()
	}
	return p.buildRGBOutputMatrixShaper()
}

// ---------------------------------------------------------------------------
// Devicelink pipelines
// ---------------------------------------------------------------------------

// readFloatDevicelinkTag ports _cmsReadFloatDevicelinkTag.
func (p *Profile) readFloatDevicelinkTag(tagFloat TagSignature) (*Pipeline, error) {
	ctx := p.ContextID
	src, _ := p.readTagPtr(tagFloat).(*Pipeline)
	if src == nil {
		return nil, errorf(ErrCorruptionDetected, "float devicelink: tag not a pipeline")
	}
	lut, err := src.Dup()
	if err != nil {
		return nil, err
	}

	pcs := p.GetPCS()
	spc := p.GetColorSpace()

	if spc == SigLabData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if spc == SigXYZData {
		if err := insertNormStage(lut, AtBegin, ctx.stageNormalizeToXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}

	if pcs == SigLabData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromLabFloat); err != nil {
			lut.Free()
			return nil, err
		}
	} else if pcs == SigXYZData {
		if err := insertNormStage(lut, AtEnd, ctx.stageNormalizeFromXyzFloat); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// ReadDevicelinkLUT ports _cmsReadDevicelinkLUT: build the devicelink pipeline
// (also handles abstract profiles). No matrix-shaper fallback exists here.
func (p *Profile) ReadDevicelinkLUT(intent uint32) (*Pipeline, error) {
	ctx := p.ContextID

	if intent > IntentAbsoluteColorimetric {
		return nil, errorf(ErrRange, "devicelink LUT: intent out of range")
	}

	tag16 := device2PCS16[intent]
	tagFloat := device2PCSFloat[intent]

	// Named color takes the appropriate tag.
	if p.GetDeviceClass() == SigNamedColorClass {
		nc, _ := p.readTagPtr(SigNamedColor2Tag).(*NamedColorList)
		if nc == nil {
			return nil, errorf(ErrCorruptionDetected, "devicelink LUT: no named color list")
		}
		lut, err := ctx.PipelineAlloc(0, 0)
		if err != nil {
			return nil, err
		}
		if err := lut.InsertStage(AtBegin, stageAllocNamedColor(nc, false)); err != nil {
			lut.Free()
			return nil, err
		}
		if p.GetColorSpace() == SigLabData {
			v2v4, err := ctx.stageAllocLabV2ToV4()
			if err != nil {
				lut.Free()
				return nil, err
			}
			if err := lut.InsertStage(AtEnd, v2v4); err != nil {
				lut.Free()
				return nil, err
			}
		}
		return lut, nil
	}

	if p.IsTag(tagFloat) { // Float tag takes precedence
		return p.readFloatDevicelinkTag(tagFloat)
	}

	tagFloat = device2PCSFloat[0]
	if p.IsTag(tagFloat) {
		src, _ := p.readTagPtr(tagFloat).(*Pipeline)
		if src == nil {
			return nil, errorf(ErrCorruptionDetected, "devicelink LUT: tag not a pipeline")
		}
		return src.Dup()
	}

	if !p.IsTag(tag16) {
		tag16 = device2PCS16[0]
		if !p.IsTag(tag16) {
			return nil, errorf(ErrCorruptionDetected, "devicelink LUT: no LUT tag")
		}
	}

	src, _ := p.readTagPtr(tag16).(*Pipeline)
	if src == nil {
		return nil, errorf(ErrCorruptionDetected, "devicelink LUT: tag not a pipeline")
	}
	lut, err := src.Dup()
	if err != nil {
		return nil, err
	}

	// Lab as indexer space => trilinear interpolation.
	if p.GetPCS() == SigLabData {
		changeInterpolationToTrilinear(lut)
	}

	originalType := p.GetTagTrueType(tag16)

	// Adjust data for Lab16 on output.
	if originalType != SigLut16Type {
		return lut, nil
	}

	// Lab can be on both sides here.
	if p.GetColorSpace() == SigLabData {
		v4v2, err := ctx.stageAllocLabV4ToV2()
		if err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtBegin, v4v2); err != nil {
			lut.Free()
			return nil, err
		}
	}
	if p.GetPCS() == SigLabData {
		v2v4, err := ctx.stageAllocLabV2ToV4()
		if err != nil {
			lut.Free()
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, v2v4); err != nil {
			lut.Free()
			return nil, err
		}
	}
	return lut, nil
}

// ---------------------------------------------------------------------------
// Capability queries
// ---------------------------------------------------------------------------

// IsMatrixShaper ports cmsIsMatrixShaper.
func (p *Profile) IsMatrixShaper() bool {
	switch p.GetColorSpace() {
	case SigGrayData:
		return p.IsTag(SigGrayTRCTag)
	case SigRgbData:
		return p.IsTag(SigRedColorantTag) &&
			p.IsTag(SigGreenColorantTag) &&
			p.IsTag(SigBlueColorantTag) &&
			p.IsTag(SigRedTRCTag) &&
			p.IsTag(SigGreenTRCTag) &&
			p.IsTag(SigBlueTRCTag)
	default:
		return false
	}
}

// IsCLUT ports cmsIsCLUT.
func (p *Profile) IsCLUT(intent, usedDirection uint32) bool {
	// For devicelinks, the supported intent is the one in the header.
	if p.GetDeviceClass() == SigLinkClass {
		return p.GetHeaderRenderingIntent() == intent
	}

	var tagTable [4]TagSignature
	switch usedDirection {
	case UsedAsInput:
		tagTable = device2PCS16
	case UsedAsOutput:
		tagTable = pcs2Device16
	case UsedAsProof:
		// Proofing needs rel. colorimetric in output; recurse.
		return p.IsIntentSupported(intent, UsedAsInput) &&
			p.IsIntentSupported(IntentRelativeColorimetric, UsedAsOutput)
	default:
		p.ContextID.signalError(ErrRange, "Unexpected direction (%d)", usedDirection)
		return false
	}

	// Extended intents are not strictly CLUT-based.
	if intent > IntentAbsoluteColorimetric {
		return false
	}
	return p.IsTag(tagTable[intent])
}

// IsIntentSupported ports cmsIsIntentSupported.
func (p *Profile) IsIntentSupported(intent, usedDirection uint32) bool {
	if p.IsCLUT(intent, usedDirection) {
		return true
	}
	// Any matrix shaper also counts as supporting the intent (see the reference's
	// note on V2 rel.col. accuracy).
	return p.IsMatrixShaper()
}

// ---------------------------------------------------------------------------
// Profile sequence
// ---------------------------------------------------------------------------

// ReadProfileSequence ports _cmsReadProfileSequence: combine the profile
// sequence description and profile sequence id tags into one structure.
func (p *Profile) ReadProfileSequence() *ProfileSequence {
	profileSeq, _ := p.readTagPtr(SigProfileSequenceDescTag).(*ProfileSequence)
	profileID, _ := p.readTagPtr(SigProfileSequenceIdTag).(*ProfileSequence)

	if profileSeq == nil && profileID == nil {
		return nil
	}
	if profileSeq == nil {
		return profileID.Dup()
	}
	if profileID == nil {
		return profileSeq.Dup()
	}

	// Mix both; they must agree on count.
	if profileSeq.n != profileID.n {
		return profileSeq.Dup()
	}

	newSeq := profileSeq.Dup()
	if newSeq != nil {
		for i := uint32(0); i < profileSeq.n; i++ {
			newSeq.seq[i].ProfileID = profileID.seq[i].ProfileID
			newSeq.seq[i].Description = profileID.seq[i].Description.Dup()
		}
	}
	return newSeq
}

// WriteProfileSequence ports _cmsWriteProfileSequence: write the sequence to the
// desc tag, and (for v4) also to the id tag.
func (p *Profile) WriteProfileSequence(seq *ProfileSequence) error {
	if err := p.WriteTag(SigProfileSequenceDescTag, seq); err != nil {
		return err
	}
	if p.GetEncodedICCversion() >= 0x4000000 {
		if err := p.WriteTag(SigProfileSequenceIdTag, seq); err != nil {
			return err
		}
	}
	return nil
}

// getMLUFromProfile ports GetMLUFromProfile: read and duplicate an MLU tag.
func (p *Profile) getMLUFromProfile(sig TagSignature) *MLU {
	mlu, _ := p.readTagPtr(sig).(*MLU)
	if mlu == nil {
		return nil
	}
	return mlu.Dup()
}

// CompileProfileSequence ports _cmsCompileProfileSequence: build a sequence
// descriptor from an array of profiles.
func (ctx *Context) CompileProfileSequence(profiles []*Profile) *ProfileSequence {
	nProfiles := uint32(len(profiles))
	seq := ctx.AllocProfileSequenceDescription(nProfiles)
	if seq == nil {
		return nil
	}
	for i := uint32(0); i < nProfiles; i++ {
		ps := &seq.seq[i]
		h := profiles[i]

		// A nil entry would panic on the accessors below; leave its slot
		// zero-valued instead (the reference dereferences NULL here).
		if h == nil {
			continue
		}

		ps.Attributes = h.GetHeaderAttributes()
		ps.ProfileID = h.GetProfileID()
		ps.DeviceMfg = h.GetHeaderManufacturer()
		ps.DeviceModel = h.GetHeaderModel()

		if tech, ok := h.readTagPtr(SigTechnologyTag).(Signature); ok {
			ps.Technology = uint32(tech)
		} else {
			ps.Technology = 0
		}

		ps.Manufacturer = h.getMLUFromProfile(SigDeviceMfgDescTag)
		ps.Model = h.getMLUFromProfile(SigDeviceModelDescTag)
		ps.Description = h.getMLUFromProfile(SigProfileDescriptionTag)
	}
	return seq
}

// ---------------------------------------------------------------------------
// Profile info accessors
// ---------------------------------------------------------------------------

// getInfoMLU ports GetInfo: select the descriptive MLU tag for an InfoType.
func (p *Profile) getInfoMLU(info InfoType) *MLU {
	var sig TagSignature
	switch info {
	case InfoDescription:
		// MacOS proprietary description tag takes precedence when present.
		if p.IsTag(SigProfileDescriptionMLTag) {
			sig = SigProfileDescriptionMLTag
		} else {
			sig = SigProfileDescriptionTag
		}
	case InfoManufacturer:
		sig = SigDeviceMfgDescTag
	case InfoModel:
		sig = SigDeviceModelDescTag
	case InfoCopyright:
		sig = SigCopyrightTag
	default:
		return nil
	}
	mlu, _ := p.readTagPtr(sig).(*MLU)
	return mlu
}

// GetProfileInfo ports cmsGetProfileInfo: copy the localized wide string into
// buffer (capacity in runes). Returns the number of wchar bytes required or
// written; see cmsMLUgetWide for the exact contract.
func (p *Profile) GetProfileInfo(info InfoType, languageCode, countryCode string, buffer []rune) uint32 {
	mlu := p.getInfoMLU(info)
	if mlu == nil {
		return 0
	}
	return mlu.mluGetWideBuf(strTo16(languageCode), strTo16(countryCode), buffer)
}

// GetProfileInfoASCII ports cmsGetProfileInfoASCII.
func (p *Profile) GetProfileInfoASCII(info InfoType, languageCode, countryCode string, buffer []byte) uint32 {
	mlu := p.getInfoMLU(info)
	if mlu == nil {
		return 0
	}
	return mlu.mluGetASCII(strTo16(languageCode), strTo16(countryCode), buffer)
}

// GetProfileInfoUTF8 ports cmsGetProfileInfoUTF8.
func (p *Profile) GetProfileInfoUTF8(info InfoType, languageCode, countryCode string, buffer []byte) uint32 {
	mlu := p.getInfoMLU(info)
	if mlu == nil {
		return 0
	}
	return mlu.mluGetUTF8(strTo16(languageCode), strTo16(countryCode), buffer)
}
