package lcms2

// virt.go ports src/cmsvirt.c: the built-in ("virtual") profile constructors —
// RGB matrix-shaper, gray, linearization / ink-limiting device links, the sRGB
// space, Lab v2/v4, XYZ, NULL, the BCHSW abstract profile, the experimental
// OkLab space, and cmsTransform2DeviceLink (the transform -> device-link builder
// used by linkicc).
//
// The tag-write order below follows the reference digit-for-digit so that the
// on-disk tag directory (and thus cmsSaveProfileToMem bytes) matches the C
// reference for the deterministic builtins. Only the header creation date/time
// differs (it is a live timestamp).
//
// Naming follows PLAN.md: the C cmsCreateXxxTHR(ContextID, ...) becomes a
// (*Context) method, and the non-THR cmsCreateXxx(...) becomes a package-level
// wrapper on the default context. All failures return an error; nothing panics.

import "encoding/binary"

// ---------------------------------------------------------------------------
// Shared text/sequence helpers (static SetTextTags / SetSeqDescTag in C).
// ---------------------------------------------------------------------------

// setTextTags ports SetTextTags: write the 'desc' and 'cprt' tags as an MLU with
// a single "en"/"US" translation. On V4+ profiles these serialize as mluc; on
// V2 profiles 'desc' becomes textDescription and 'cprt' plain text (handled by
// the tag descriptors' DecideType).
func (p *Profile) setTextTags(description string) error {
	ctx := p.ContextID

	descriptionMLU := ctx.NewMLU(1)
	copyrightMLU := ctx.NewMLU(1)
	if descriptionMLU == nil || copyrightMLU == nil {
		return ctx.signalError(ErrNull, "SetTextTags: cannot allocate MLU")
	}

	if !descriptionMLU.SetWideString("en", "US", description) {
		return ctx.signalError(ErrInternal, "SetTextTags: cannot set description")
	}
	if !copyrightMLU.SetWideString("en", "US", "No copyright, use freely") {
		return ctx.signalError(ErrInternal, "SetTextTags: cannot set copyright")
	}

	if err := p.WriteTag(SigProfileDescriptionTag, descriptionMLU); err != nil {
		return err
	}
	if err := p.WriteTag(SigCopyrightTag, copyrightMLU); err != nil {
		return err
	}
	return nil
}

// setSeqDescTag ports SetSeqDescTag: write a one-entry profile-sequence
// description tagging the profile as produced by "Little CMS".
func (p *Profile) setSeqDescTag(model string) error {
	ctx := p.ContextID
	seq := ctx.AllocProfileSequenceDescription(1)
	if seq == nil {
		return ctx.signalError(ErrNull, "SetSeqDescTag: cannot allocate sequence")
	}

	s := seq.Seq()
	s[0].DeviceMfg = 0
	s[0].DeviceModel = 0
	s[0].Attributes = 0
	s[0].Technology = 0

	// cmsAllocProfileSequenceDescription leaves Manufacturer/Model/Description
	// NULL (cmsnamed.c), so the reference's cmsMLUsetASCII calls in SetSeqDescTag
	// are silent no-ops and the serialized pseq tag carries empty descriptors.
	// Allocating and populating them here diverged from the reference byte-for-
	// byte (and changed the MD5 profile ID) for the linearization and
	// ink-limiting device links; leave them nil to match. model is intentionally
	// unused, mirroring the dropped write.
	_ = model
	s[0].Manufacturer = nil
	s[0].Model = nil
	s[0].Description = nil

	if err := p.WriteProfileSequence(seq); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// RGB matrix-shaper profile (cmsCreateRGBProfileTHR).
// ---------------------------------------------------------------------------

// CreateRGBProfile ports cmsCreateRGBProfileTHR: build a display RGB profile from
// a white point, primaries and per-channel transfer functions. Any of the three
// may be nil (mirroring the C NULL arguments), producing a partial profile. When
// transferFunction is non-nil it must hold exactly three curves.
func (ctx *Context) CreateRGBProfile(whitePoint *CIExyY, primaries *CIExyYTRIPLE,
	transferFunction []*ToneCurve) (*Profile, error) {

	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateRGBProfile: cannot allocate profile")
	}

	hICC.SetProfileVersion(4.4)
	hICC.SetDeviceClass(SigDisplayClass)
	hICC.SetColorSpace(SigRgbData)
	hICC.SetPCS(SigXYZData)
	hICC.SetHeaderRenderingIntent(IntentPerceptual)

	if err := hICC.setTextTags("RGB built-in"); err != nil {
		return nil, err
	}

	if whitePoint != nil {
		// The media white point is always D50 for a V4 display profile; the
		// actual white is carried by the chromatic-adaptation tag.
		d50 := D50XYZ()
		if err := hICC.WriteTag(SigMediaWhitePointTag, &d50); err != nil {
			return nil, err
		}

		whitePointXYZ := XyY2XYZ(*whitePoint)
		chad, ok := adaptationMatrix(nil, whitePointXYZ, D50XYZ())
		if !ok {
			return nil, ctx.signalError(ErrColorspaceCheck, "CreateRGBProfile: cannot build adaptation matrix")
		}
		if err := hICC.WriteTag(SigChromaticAdaptationTag, matFlat(chad)); err != nil {
			return nil, err
		}
	}

	if whitePoint != nil && primaries != nil {
		maxWhite := CIExyY{X: whitePoint.X, Y: whitePoint.Y, YY: 1.0}

		colorants, ok := buildRGB2XYZTransferMatrix(maxWhite, *primaries)
		if !ok {
			return nil, ctx.signalError(ErrColorspaceCheck, "CreateRGBProfile: cannot build primaries matrix")
		}

		// Columns of the matrix are the red/green/blue XYZ tristimulus values.
		red := CIEXYZ{X: colorants[0][0], Y: colorants[1][0], Z: colorants[2][0]}
		green := CIEXYZ{X: colorants[0][1], Y: colorants[1][1], Z: colorants[2][1]}
		blue := CIEXYZ{X: colorants[0][2], Y: colorants[1][2], Z: colorants[2][2]}

		// The reference writes red, blue, green in this order.
		if err := hICC.WriteTag(SigRedColorantTag, &red); err != nil {
			return nil, err
		}
		if err := hICC.WriteTag(SigBlueColorantTag, &blue); err != nil {
			return nil, err
		}
		if err := hICC.WriteTag(SigGreenColorantTag, &green); err != nil {
			return nil, err
		}
	}

	if transferFunction != nil {
		if len(transferFunction) < 3 {
			return nil, ctx.signalError(ErrRange, "CreateRGBProfile: transferFunction needs 3 curves")
		}

		// Tries to minimize space by linking identical TRC tags.
		if err := hICC.WriteTag(SigRedTRCTag, transferFunction[0]); err != nil {
			return nil, err
		}

		if transferFunction[1] == transferFunction[0] {
			if err := hICC.LinkTag(SigGreenTRCTag, SigRedTRCTag); err != nil {
				return nil, err
			}
		} else {
			if err := hICC.WriteTag(SigGreenTRCTag, transferFunction[1]); err != nil {
				return nil, err
			}
		}

		if transferFunction[2] == transferFunction[0] {
			if err := hICC.LinkTag(SigBlueTRCTag, SigRedTRCTag); err != nil {
				return nil, err
			}
		} else {
			if err := hICC.WriteTag(SigBlueTRCTag, transferFunction[2]); err != nil {
				return nil, err
			}
		}
	}

	if primaries != nil {
		if err := hICC.WriteTag(SigChromaticityTag, primaries); err != nil {
			return nil, err
		}
	}

	return hICC, nil
}

// CreateRGBProfile builds an RGB matrix-shaper profile on the default context.
func CreateRGBProfile(whitePoint *CIExyY, primaries *CIExyYTRIPLE,
	transferFunction []*ToneCurve) (*Profile, error) {
	return defaultContext.CreateRGBProfile(whitePoint, primaries, transferFunction)
}

// ---------------------------------------------------------------------------
// Gray profile (cmsCreateGrayProfileTHR).
// ---------------------------------------------------------------------------

// CreateGrayProfile ports cmsCreateGrayProfileTHR: a gray display profile with a
// white point and a single gray transfer function.
func (ctx *Context) CreateGrayProfile(whitePoint *CIExyY, transferFunction *ToneCurve) (*Profile, error) {
	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateGrayProfile: cannot allocate profile")
	}

	hICC.SetProfileVersion(4.4)
	hICC.SetDeviceClass(SigDisplayClass)
	hICC.SetColorSpace(SigGrayData)
	hICC.SetPCS(SigXYZData)
	hICC.SetHeaderRenderingIntent(IntentPerceptual)

	if err := hICC.setTextTags("gray built-in"); err != nil {
		return nil, err
	}

	if whitePoint != nil {
		tmp := XyY2XYZ(*whitePoint)
		if err := hICC.WriteTag(SigMediaWhitePointTag, &tmp); err != nil {
			return nil, err
		}
	}

	if transferFunction != nil {
		if err := hICC.WriteTag(SigGrayTRCTag, transferFunction); err != nil {
			return nil, err
		}
	}

	return hICC, nil
}

// CreateGrayProfile builds a gray profile on the default context.
func CreateGrayProfile(whitePoint *CIExyY, transferFunction *ToneCurve) (*Profile, error) {
	return defaultContext.CreateGrayProfile(whitePoint, transferFunction)
}

// ---------------------------------------------------------------------------
// Linearization device link (cmsCreateLinearizationDeviceLinkTHR).
// ---------------------------------------------------------------------------

// CreateLinearizationDeviceLink ports cmsCreateLinearizationDeviceLinkTHR: a
// device link operating in colorSpace with one transfer function per channel.
func (ctx *Context) CreateLinearizationDeviceLink(colorSpace ColorSpaceSignature,
	transferFunctions []*ToneCurve) (*Profile, error) {

	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateLinearizationDeviceLink: cannot allocate profile")
	}

	hICC.SetProfileVersion(4.4)
	hICC.SetDeviceClass(SigLinkClass)
	hICC.SetColorSpace(colorSpace)
	hICC.SetPCS(colorSpace)
	hICC.SetHeaderRenderingIntent(IntentPerceptual)

	nChannels := ChannelsOfColorSpace(colorSpace)

	pipeline, err := ctx.PipelineAlloc(uint32(nChannels), uint32(nChannels))
	if err != nil || pipeline == nil {
		return nil, ctx.signalError(ErrNull, "CreateLinearizationDeviceLink: cannot allocate pipeline")
	}

	curves, err := ctx.StageAllocToneCurves(uint32(nChannels), transferFunctions)
	if err != nil || curves == nil {
		return nil, ctx.signalError(ErrNull, "CreateLinearizationDeviceLink: cannot allocate curves")
	}
	if err := pipeline.InsertStage(AtBegin, curves); err != nil {
		return nil, err
	}

	if err := hICC.setTextTags("Linearization built-in"); err != nil {
		return nil, err
	}
	if err := hICC.WriteTag(SigAToB0Tag, pipeline); err != nil {
		return nil, err
	}
	if err := hICC.setSeqDescTag("Linearization built-in"); err != nil {
		return nil, err
	}

	return hICC, nil
}

// CreateLinearizationDeviceLink builds a linearization device link on the
// default context.
func CreateLinearizationDeviceLink(colorSpace ColorSpaceSignature,
	transferFunctions []*ToneCurve) (*Profile, error) {
	return defaultContext.CreateLinearizationDeviceLink(colorSpace, transferFunctions)
}

// ---------------------------------------------------------------------------
// Ink-limiting device link (cmsCreateInkLimitingDeviceLinkTHR).
// ---------------------------------------------------------------------------

// inkLimitingSampler ports InkLimitingSampler: scale C, M and Y down so the
// total ink (C+M+Y+K) does not exceed the limit; K is untouched.
func inkLimitingSampler(in, out []uint16, cargo any) bool {
	limitPtr, ok := cargo.(*float64)
	if !ok || limitPtr == nil {
		return false
	}
	inkLimit := *limitPtr * 655.35

	sumCMY := float64(in[0]) + float64(in[1]) + float64(in[2])
	sumCMYK := sumCMY + float64(in[3])

	ratio := 1.0
	if sumCMYK > inkLimit && sumCMY > 0 {
		ratio = 1 - ((sumCMYK - inkLimit) / sumCMY)
		if ratio < 0 {
			ratio = 0
		}
	}

	out[0] = quickSaturateWord(float64(in[0]) * ratio) // C
	out[1] = quickSaturateWord(float64(in[1]) * ratio) // M
	out[2] = quickSaturateWord(float64(in[2]) * ratio) // Y
	out[3] = in[3]                                      // K (untouched)

	return true
}

// CreateInkLimitingDeviceLink ports cmsCreateInkLimitingDeviceLinkTHR: a CMYK
// device link that enforces a total-ink limit (percent, 1..400).
func (ctx *Context) CreateInkLimitingDeviceLink(colorSpace ColorSpaceSignature,
	limit float64) (*Profile, error) {

	if colorSpace != SigCmykData {
		return nil, ctx.signalError(ErrColorspaceCheck, "InkLimiting: Only CMYK currently supported")
	}

	if limit < 1.0 || limit > 400 {
		ctx.signalError(ErrRange, "InkLimiting: Limit should be between 1..400")
		if limit < 1 {
			limit = 1
		}
		if limit > 400 {
			limit = 400
		}
	}

	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateInkLimitingDeviceLink: cannot allocate profile")
	}

	hICC.SetProfileVersion(4.4)
	hICC.SetDeviceClass(SigLinkClass)
	hICC.SetColorSpace(colorSpace)
	hICC.SetPCS(colorSpace)
	hICC.SetHeaderRenderingIntent(IntentPerceptual)

	lut, err := ctx.PipelineAlloc(4, 4)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "CreateInkLimitingDeviceLink: cannot allocate pipeline")
	}

	nChannels := ChannelsOf(colorSpace)

	clut, err := ctx.StageAllocCLut16bit(17, nChannels, nChannels, nil)
	if err != nil || clut == nil {
		return nil, ctx.signalError(ErrNull, "CreateInkLimitingDeviceLink: cannot allocate CLUT")
	}
	if !clut.SampleCLut16bit(inkLimitingSampler, &limit, 0) {
		return nil, ctx.signalError(ErrInternal, "CreateInkLimitingDeviceLink: sampling failed")
	}

	pre, err := ctx.stageAllocIdentityCurves(nChannels)
	if err != nil || pre == nil {
		return nil, ctx.signalError(ErrNull, "CreateInkLimitingDeviceLink: cannot allocate identity curves")
	}
	post, err := ctx.stageAllocIdentityCurves(nChannels)
	if err != nil || post == nil {
		return nil, ctx.signalError(ErrNull, "CreateInkLimitingDeviceLink: cannot allocate identity curves")
	}
	if err := lut.InsertStage(AtBegin, pre); err != nil {
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, clut); err != nil {
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, post); err != nil {
		return nil, err
	}

	if err := hICC.setTextTags("ink-limiting built-in"); err != nil {
		return nil, err
	}
	if err := hICC.WriteTag(SigAToB0Tag, lut); err != nil {
		return nil, err
	}
	if err := hICC.setSeqDescTag("ink-limiting built-in"); err != nil {
		return nil, err
	}

	return hICC, nil
}

// CreateInkLimitingDeviceLink builds an ink-limiting device link on the default
// context.
func CreateInkLimitingDeviceLink(colorSpace ColorSpaceSignature, limit float64) (*Profile, error) {
	return defaultContext.CreateInkLimitingDeviceLink(colorSpace, limit)
}

// ---------------------------------------------------------------------------
// Lab v2 / Lab v4 identity profiles.
// ---------------------------------------------------------------------------

// CreateLab2Profile ports cmsCreateLab2ProfileTHR: a fake Lab v2 identity
// abstract profile. whitePoint may be nil (D50).
func (ctx *Context) CreateLab2Profile(whitePoint *CIExyY) (*Profile, error) {
	wp := whitePoint
	if wp == nil {
		d := D50xyY()
		wp = &d
	}

	hProfile, err := ctx.CreateRGBProfile(wp, nil, nil)
	if err != nil {
		return nil, err
	}

	hProfile.SetProfileVersion(2.1)
	hProfile.SetDeviceClass(SigAbstractClass)
	hProfile.SetColorSpace(SigLabData)
	hProfile.SetPCS(SigLabData)

	if err := hProfile.setTextTags("Lab identity built-in"); err != nil {
		return nil, err
	}

	// An identity LUT is all we need.
	lut, err := ctx.PipelineAlloc(3, 3)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "CreateLab2Profile: cannot allocate pipeline")
	}
	id, err := ctx.stageAllocIdentityCLut(3)
	if err != nil || id == nil {
		return nil, ctx.signalError(ErrNull, "CreateLab2Profile: cannot allocate identity CLUT")
	}
	if err := lut.InsertStage(AtBegin, id); err != nil {
		return nil, err
	}
	if err := hProfile.WriteTag(SigAToB0Tag, lut); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// CreateLab2Profile builds a Lab v2 identity profile on the default context.
func CreateLab2Profile(whitePoint *CIExyY) (*Profile, error) {
	return defaultContext.CreateLab2Profile(whitePoint)
}

// CreateLab4Profile ports cmsCreateLab4ProfileTHR: a fake Lab v4 identity
// abstract profile. whitePoint may be nil (D50).
func (ctx *Context) CreateLab4Profile(whitePoint *CIExyY) (*Profile, error) {
	var xyz CIEXYZ
	if whitePoint == nil {
		xyz = D50XYZ()
	} else {
		xyz = XyY2XYZ(*whitePoint)
	}

	hProfile, err := ctx.CreateRGBProfile(nil, nil, nil)
	if err != nil {
		return nil, err
	}

	hProfile.SetProfileVersion(4.4)
	hProfile.SetDeviceClass(SigAbstractClass)
	hProfile.SetColorSpace(SigLabData)
	hProfile.SetPCS(SigLabData)

	if err := hProfile.WriteTag(SigMediaWhitePointTag, &xyz); err != nil {
		return nil, err
	}
	if err := hProfile.setTextTags("Lab identity built-in"); err != nil {
		return nil, err
	}

	// An empty LUT (identity curves) is all we need.
	lut, err := ctx.PipelineAlloc(3, 3)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "CreateLab4Profile: cannot allocate pipeline")
	}
	id, err := ctx.stageAllocIdentityCurves(3)
	if err != nil || id == nil {
		return nil, ctx.signalError(ErrNull, "CreateLab4Profile: cannot allocate identity curves")
	}
	if err := lut.InsertStage(AtBegin, id); err != nil {
		return nil, err
	}
	if err := hProfile.WriteTag(SigAToB0Tag, lut); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// CreateLab4Profile builds a Lab v4 identity profile on the default context.
func CreateLab4Profile(whitePoint *CIExyY) (*Profile, error) {
	return defaultContext.CreateLab4Profile(whitePoint)
}

// ---------------------------------------------------------------------------
// XYZ identity profile (cmsCreateXYZProfileTHR).
// ---------------------------------------------------------------------------

// CreateXYZProfile ports cmsCreateXYZProfileTHR: a fake XYZ identity abstract
// profile.
func (ctx *Context) CreateXYZProfile() (*Profile, error) {
	d50 := D50xyY()
	hProfile, err := ctx.CreateRGBProfile(&d50, nil, nil)
	if err != nil {
		return nil, err
	}

	hProfile.SetProfileVersion(4.4)
	hProfile.SetDeviceClass(SigAbstractClass)
	hProfile.SetColorSpace(SigXYZData)
	hProfile.SetPCS(SigXYZData)

	if err := hProfile.setTextTags("XYZ identity built-in"); err != nil {
		return nil, err
	}

	lut, err := ctx.PipelineAlloc(3, 3)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "CreateXYZProfile: cannot allocate pipeline")
	}
	id, err := ctx.stageAllocIdentityCurves(3)
	if err != nil || id == nil {
		return nil, ctx.signalError(ErrNull, "CreateXYZProfile: cannot allocate identity curves")
	}
	if err := lut.InsertStage(AtBegin, id); err != nil {
		return nil, err
	}
	if err := hProfile.WriteTag(SigAToB0Tag, lut); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// CreateXYZProfile builds an XYZ identity profile on the default context.
func CreateXYZProfile() (*Profile, error) {
	return defaultContext.CreateXYZProfile()
}

// ---------------------------------------------------------------------------
// sRGB profile (cmsCreate_sRGBProfileTHR).
// ---------------------------------------------------------------------------

// build_sRGBGamma ports Build_sRGBGamma: the IEC 61966-2-1 parametric type-4
// curve (2.4 / 1.055 / 0.055 / 12.92 / 0.04045).
func (ctx *Context) build_sRGBGamma() (*ToneCurve, error) {
	params := []float64{
		2.4,
		1.0 / 1.055,
		0.055 / 1.055,
		1.0 / 12.92,
		0.04045,
	}
	return ctx.BuildParametricToneCurve(4, params)
}

// Create_sRGBProfile ports cmsCreate_sRGBProfileTHR: the ICC virtual profile for
// the sRGB color space (Rec.709 primaries, D65 white, the sRGB TRC).
func (ctx *Context) Create_sRGBProfile() (*Profile, error) {
	d65 := CIExyY{X: 0.3127, Y: 0.3290, YY: 1.0}
	rec709 := CIExyYTRIPLE{
		Red:   CIExyY{X: 0.6400, Y: 0.3300, YY: 1.0},
		Green: CIExyY{X: 0.3000, Y: 0.6000, YY: 1.0},
		Blue:  CIExyY{X: 0.1500, Y: 0.0600, YY: 1.0},
	}

	gamma, err := ctx.build_sRGBGamma()
	if err != nil || gamma == nil {
		return nil, err
	}
	gamma22 := []*ToneCurve{gamma, gamma, gamma}

	hsRGB, err := ctx.CreateRGBProfile(&d65, &rec709, gamma22)
	if err != nil {
		return nil, err
	}

	if err := hsRGB.setTextTags("sRGB built-in"); err != nil {
		return nil, err
	}

	return hsRGB, nil
}

// Create_sRGBProfile builds the sRGB profile on the default context.
func Create_sRGBProfile() (*Profile, error) {
	return defaultContext.Create_sRGBProfile()
}

// ---------------------------------------------------------------------------
// OkLab experimental profile (cmsCreate_OkLabProfile).
// ---------------------------------------------------------------------------

// Create_OkLabProfile ports cmsCreate_OkLabProfile: an experimental OkLab color
// space profile. Note (as in the reference) that this virtual profile cannot be
// serialized to an ICC file — it is only usable in-memory for transforms.
func (ctx *Context) Create_OkLabProfile() (*Profile, error) {
	mD65D50 := []float64{
		1.047886, 0.022919, -0.050216,
		0.029582, 0.990484, -0.017079,
		-0.009252, 0.015073, 0.751678,
	}
	mD50D65 := []float64{
		0.955512609517083, -0.023073214184645, 0.063308961782107,
		-0.028324949364887, 1.009942432477107, 0.021054814890112,
		0.012328875695483, -0.020535835374141, 1.330713916450354,
	}
	mD65LMS := []float64{
		0.8189330101, 0.3618667424, -0.1288597137,
		0.0329845436, 0.9293118715, 0.0361456387,
		0.0482003018, 0.2643662691, 0.6338517070,
	}
	mLMSD65 := []float64{
		1.227013851103521, -0.557799980651822, 0.281256148966468,
		-0.040580178423281, 1.112256869616830, -0.071676678665601,
		-0.076381284505707, -0.421481978418013, 1.586163220440795,
	}
	mLMSprimeOkLab := []float64{
		0.2104542553, 0.7936177850, -0.0040720468,
		1.9779984951, -2.4285922050, 0.4505937099,
		0.0259040371, 0.7827717662, -0.8086757660,
	}
	mOkLabLMSprime := []float64{
		0.999999998450520, 0.396337792173768, 0.215803758060759,
		1.000000008881761, -0.105561342323656, -0.063854174771706,
		1.000000054672411, -0.089484182094966, -1.291485537864092,
	}

	fail := func(err error) (*Profile, error) {
		if err != nil {
			return nil, err
		}
		return nil, ctx.signalError(ErrNull, "Create_OkLabProfile: allocation failed")
	}

	xyzPCS, err := ctx.stageNormalizeFromXyzFloat()
	if err != nil {
		return fail(err)
	}
	pcsXYZ, err := ctx.stageNormalizeToXyzFloat()
	if err != nil {
		return fail(err)
	}
	d65toD50, err := ctx.StageAllocMatrix(3, 3, mD65D50, nil)
	if err != nil {
		return fail(err)
	}
	d50toD65, err := ctx.StageAllocMatrix(3, 3, mD50D65, nil)
	if err != nil {
		return fail(err)
	}
	d65toLMS, err := ctx.StageAllocMatrix(3, 3, mD65LMS, nil)
	if err != nil {
		return fail(err)
	}
	lmStoD65, err := ctx.StageAllocMatrix(3, 3, mLMSD65, nil)
	if err != nil {
		return fail(err)
	}

	cubeRoot, err := ctx.BuildGamma(1.0 / 3.0)
	if err != nil {
		return fail(err)
	}
	cube, err := ctx.BuildGamma(3.0)
	if err != nil {
		return fail(err)
	}
	roots := []*ToneCurve{cubeRoot, cubeRoot, cubeRoot}
	cubes := []*ToneCurve{cube, cube, cube}

	nonLinearityFw, err := ctx.StageAllocToneCurves(3, roots)
	if err != nil {
		return fail(err)
	}
	nonLinearityRv, err := ctx.StageAllocToneCurves(3, cubes)
	if err != nil {
		return fail(err)
	}
	lmSprimeOkLab, err := ctx.StageAllocMatrix(3, 3, mLMSprimeOkLab, nil)
	if err != nil {
		return fail(err)
	}
	okLabLMSprime, err := ctx.StageAllocMatrix(3, 3, mOkLabLMSprime, nil)
	if err != nil {
		return fail(err)
	}

	aToB, err := ctx.PipelineAlloc(3, 3)
	if err != nil {
		return fail(err)
	}
	bToA, err := ctx.PipelineAlloc(3, 3)
	if err != nil {
		return fail(err)
	}

	hProfile := ctx.CreateProfilePlaceholder()
	if hProfile == nil {
		return fail(nil)
	}
	hProfile.SetProfileVersion(4.4)
	hProfile.SetDeviceClass(SigColorSpaceClass)
	hProfile.SetColorSpace(Sig3colorData)
	hProfile.SetPCS(SigXYZData)
	hProfile.SetHeaderRenderingIntent(IntentRelativeColorimetric)

	// Conversion PCS (XYZ/D50) -> OkLab.
	for _, st := range []*Stage{pcsXYZ, d50toD65, d65toLMS, nonLinearityFw, lmSprimeOkLab} {
		if err := bToA.InsertStage(AtEnd, st); err != nil {
			return nil, err
		}
	}
	if err := hProfile.WriteTag(SigBToA0Tag, bToA); err != nil {
		return nil, err
	}

	for _, st := range []*Stage{okLabLMSprime, nonLinearityRv, lmStoD65, d65toD50, xyzPCS} {
		if err := aToB.InsertStage(AtEnd, st); err != nil {
			return nil, err
		}
	}
	if err := hProfile.WriteTag(SigAToB0Tag, aToB); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// Create_OkLabProfile builds the OkLab profile on the default context.
func Create_OkLabProfile() (*Profile, error) {
	return defaultContext.Create_OkLabProfile()
}

// ---------------------------------------------------------------------------
// BCHSW abstract profile (cmsCreateBCHSWabstractProfileTHR).
// ---------------------------------------------------------------------------

// bchswAdjusts mirrors BCHSWADJUSTS.
type bchswAdjusts struct {
	brightness float64
	contrast   float64
	hue        float64
	saturation float64
	adjustWP   bool
	wpSrc      CIEXYZ
	wpDest     CIEXYZ
}

// bchswSampler ports bchswSampler: apply brightness/contrast/hue/saturation in
// LCh, with an optional white-point displacement.
func bchswSampler(in, out []uint16, cargo any) bool {
	bchsw, ok := cargo.(*bchswAdjusts)
	if !ok || bchsw == nil {
		return false
	}

	labIn := LabEncoded2Float([3]uint16{in[0], in[1], in[2]})
	lchIn := Lab2LCh(labIn)

	// Do some adjusts on LCh.
	var lchOut CIELCh
	lchOut.L = lchIn.L*bchsw.contrast + bchsw.brightness
	lchOut.C = lchIn.C + bchsw.saturation
	lchOut.H = lchIn.H + bchsw.hue

	labOut := LCh2Lab(lchOut)

	// Move white point in Lab.
	if bchsw.adjustWP {
		xyz := Lab2XYZ(&bchsw.wpSrc, labOut)
		labOut = XYZ2Lab(&bchsw.wpDest, xyz)
	}

	enc := Float2LabEncoded(labOut)
	out[0] = enc[0]
	out[1] = enc[1]
	out[2] = enc[2]

	return true
}

// CreateBCHSWabstractProfile ports cmsCreateBCHSWabstractProfileTHR: an abstract
// Lab profile for brightness, contrast, hue, saturation and white-point
// displacement (via source/destination color temperatures).
func (ctx *Context) CreateBCHSWabstractProfile(nLUTPoints uint32,
	bright, contrast, hue, saturation float64,
	tempSrc, tempDest uint32) (*Profile, error) {

	var bchsw bchswAdjusts
	bchsw.brightness = bright
	bchsw.contrast = contrast
	bchsw.hue = hue
	bchsw.saturation = saturation

	if tempSrc == tempDest {
		bchsw.adjustWP = false
	} else {
		bchsw.adjustWP = true
		wp, err := WhitePointFromTemp(float64(tempSrc))
		if err != nil {
			return nil, err
		}
		bchsw.wpSrc = XyY2XYZ(wp)
		wp, err = WhitePointFromTemp(float64(tempDest))
		if err != nil {
			return nil, err
		}
		bchsw.wpDest = XyY2XYZ(wp)
	}

	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateBCHSWabstractProfile: cannot allocate profile")
	}

	hICC.SetDeviceClass(SigAbstractClass)
	hICC.SetColorSpace(SigLabData)
	hICC.SetPCS(SigLabData)
	hICC.SetHeaderRenderingIntent(IntentPerceptual)

	pipeline, err := ctx.PipelineAlloc(3, 3)
	if err != nil || pipeline == nil {
		return nil, ctx.signalError(ErrNull, "CreateBCHSWabstractProfile: cannot allocate pipeline")
	}

	dimensions := []uint32{nLUTPoints, nLUTPoints, nLUTPoints}
	clut, err := ctx.StageAllocCLut16bitGranular(dimensions, 3, 3, nil)
	if err != nil || clut == nil {
		return nil, ctx.signalError(ErrNull, "CreateBCHSWabstractProfile: cannot allocate CLUT")
	}
	if !clut.SampleCLut16bit(bchswSampler, &bchsw, 0) {
		return nil, ctx.signalError(ErrInternal, "CreateBCHSWabstractProfile: sampling failed")
	}
	if err := pipeline.InsertStage(AtEnd, clut); err != nil {
		return nil, err
	}

	if err := hICC.setTextTags("BCHS built-in"); err != nil {
		return nil, err
	}

	d50 := D50XYZ()
	if err := hICC.WriteTag(SigMediaWhitePointTag, &d50); err != nil {
		return nil, err
	}
	if err := hICC.WriteTag(SigAToB0Tag, pipeline); err != nil {
		return nil, err
	}

	return hICC, nil
}

// CreateBCHSWabstractProfile builds a BCHSW abstract profile on the default
// context.
func CreateBCHSWabstractProfile(nLUTPoints uint32, bright, contrast, hue, saturation float64,
	tempSrc, tempDest uint32) (*Profile, error) {
	return defaultContext.CreateBCHSWabstractProfile(nLUTPoints, bright, contrast, hue, saturation, tempSrc, tempDest)
}

// ---------------------------------------------------------------------------
// NULL profile (cmsCreateNULLProfileTHR).
// ---------------------------------------------------------------------------

// CreateNULLProfile ports cmsCreateNULLProfileTHR: a fake profile whose single
// output channel is always 0. Useful only for gamut-checking tricks.
func (ctx *Context) CreateNULLProfile() (*Profile, error) {
	hProfile := ctx.CreateProfilePlaceholder()
	if hProfile == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot allocate profile")
	}

	hProfile.SetProfileVersion(4.4)

	if err := hProfile.setTextTags("NULL profile built-in"); err != nil {
		return nil, err
	}

	hProfile.SetDeviceClass(SigOutputClass)
	hProfile.SetColorSpace(SigGrayData)
	hProfile.SetPCS(SigLabData)

	lut, err := ctx.PipelineAlloc(3, 1)
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot allocate pipeline")
	}

	emptyTab, err := ctx.BuildTabulatedToneCurve16([]uint16{0, 0})
	if err != nil || emptyTab == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot build curve")
	}
	postLin, err := ctx.StageAllocToneCurves(3, []*ToneCurve{emptyTab, emptyTab, emptyTab})
	if err != nil || postLin == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot allocate curves")
	}
	outLin, err := ctx.StageAllocToneCurves(1, []*ToneCurve{emptyTab})
	if err != nil || outLin == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot allocate curves")
	}

	if err := lut.InsertStage(AtEnd, postLin); err != nil {
		return nil, err
	}
	pickL, err := ctx.StageAllocMatrix(1, 3, []float64{1, 0, 0}, nil)
	if err != nil || pickL == nil {
		return nil, ctx.signalError(ErrNull, "CreateNULLProfile: cannot allocate matrix")
	}
	if err := lut.InsertStage(AtEnd, pickL); err != nil {
		return nil, err
	}
	if err := lut.InsertStage(AtEnd, outLin); err != nil {
		return nil, err
	}

	if err := hProfile.WriteTag(SigBToA0Tag, lut); err != nil {
		return nil, err
	}
	d50 := D50XYZ()
	if err := hProfile.WriteTag(SigMediaWhitePointTag, &d50); err != nil {
		return nil, err
	}

	return hProfile, nil
}

// CreateNULLProfile builds a NULL profile on the default context.
func CreateNULLProfile() (*Profile, error) {
	return defaultContext.CreateNULLProfile()
}

// ---------------------------------------------------------------------------
// Transform -> device link (cmsTransform2DeviceLink).
// ---------------------------------------------------------------------------

// isPCS ports IsPCS.
func isPCS(colorSpace ColorSpaceSignature) bool {
	return colorSpace == SigXYZData || colorSpace == SigLabData
}

// fixColorSpaces ports FixColorSpaces: assign a device class from the entry/exit
// spaces when cmsFLAGS_GUESSDEVICECLASS is set, otherwise a plain link.
func fixColorSpaces(hProfile *Profile, colorSpace, pcs ColorSpaceSignature, dwFlags uint32) {
	if dwFlags&FlagsGuessDeviceClass != 0 {
		if isPCS(colorSpace) && isPCS(pcs) {
			hProfile.SetDeviceClass(SigAbstractClass)
			hProfile.SetColorSpace(colorSpace)
			hProfile.SetPCS(pcs)
			return
		}
		if isPCS(colorSpace) && !isPCS(pcs) {
			hProfile.SetDeviceClass(SigOutputClass)
			hProfile.SetPCS(colorSpace)
			hProfile.SetColorSpace(pcs)
			return
		}
		if isPCS(pcs) && !isPCS(colorSpace) {
			hProfile.SetDeviceClass(SigInputClass)
			hProfile.SetColorSpace(colorSpace)
			hProfile.SetPCS(pcs)
			return
		}
	}

	hProfile.SetDeviceClass(SigLinkClass)
	hProfile.SetColorSpace(colorSpace)
	hProfile.SetPCS(pcs)
}

// allowedLUT mirrors cmsAllowedLUT: which MPE sequence a profile version may
// store for a given destination tag.
type allowedLUT struct {
	isV4        bool
	requiredTag TagSignature
	lutType     TagTypeSignature
	mpeTypes    []StageSignature
}

// allowedLUTTypes mirrors AllowedLUTTypes.
var allowedLUTTypes = []allowedLUT{
	{false, 0, SigLut16Type, []StageSignature{SigMatrixElemType, SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType}},
	{false, 0, SigLut16Type, []StageSignature{SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType}},
	{false, 0, SigLut16Type, []StageSignature{SigCurveSetElemType, SigCLutElemType}},
	{true, 0, SigLutAtoBType, []StageSignature{SigCurveSetElemType}},
	{true, SigAToB0Tag, SigLutAtoBType, []StageSignature{SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType}},
	{true, SigAToB0Tag, SigLutAtoBType, []StageSignature{SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType}},
	{true, SigAToB0Tag, SigLutAtoBType, []StageSignature{SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType}},
	{true, SigBToA0Tag, SigLutBtoAType, []StageSignature{SigCurveSetElemType}},
	{true, SigBToA0Tag, SigLutBtoAType, []StageSignature{SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType}},
	{true, SigBToA0Tag, SigLutBtoAType, []StageSignature{SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType}},
	{true, SigBToA0Tag, SigLutBtoAType, []StageSignature{SigCurveSetElemType, SigMatrixElemType, SigCurveSetElemType, SigCLutElemType, SigCurveSetElemType}},
}

// checkOne ports CheckOne: does the pipeline's stage sequence match tab exactly.
func checkOne(tab *allowedLUT, lut *Pipeline) bool {
	n := 0
	for mpe := lut.GetPtrToFirstStage(); mpe != nil; mpe = mpe.Next() {
		if n >= len(tab.mpeTypes) {
			return false
		}
		if mpe.StageType() != tab.mpeTypes[n] {
			return false
		}
		n++
	}
	return n == len(tab.mpeTypes)
}

// findCombination ports FindCombination: first allowed LUT layout that fits.
func findCombination(lut *Pipeline, isV4 bool, destinationTag TagSignature) *allowedLUT {
	for i := range allowedLUTTypes {
		tab := &allowedLUTTypes[i]
		if isV4 != tab.isV4 {
			continue
		}
		if tab.requiredTag != 0 && tab.requiredTag != destinationTag {
			continue
		}
		if checkOne(tab, lut) {
			return tab
		}
	}
	return nil
}

// Transform2DeviceLink ports cmsTransform2DeviceLink: convert a transform into a
// device-link profile of the requested ICC version.
func Transform2DeviceLink(hTransform *Transform, version float64, dwFlags uint32) (*Profile, error) {
	if hTransform == nil {
		return nil, errorf(ErrNull, "cmsTransform2DeviceLink: nil transform")
	}
	ctx := hTransform.GetTransformContextID()

	// Check if the pipeline being held is valid.
	if hTransform.Lut == nil {
		return nil, ctx.signalError(ErrNull, "cmsTransform2DeviceLink: transform has no pipeline")
	}

	// Named-color transforms take a dedicated path in the reference.
	if mpe := hTransform.Lut.GetPtrToFirstStage(); mpe != nil {
		if mpe.StageType() == SigNamedColorElemType {
			return createNamedColorDevicelink(ctx, hTransform)
		}
	}

	// Get a copy of the transformation.
	lut, err := hTransform.Lut.Dup()
	if err != nil || lut == nil {
		return nil, ctx.signalError(ErrNull, "cmsTransform2DeviceLink: cannot duplicate pipeline")
	}

	// Time to fix the Lab2/Lab4 issue on the input side.
	if hTransform.EntryColorSpace == SigLabData && version < 4.0 {
		st, err := ctx.stageAllocLabV2ToV4curves()
		if err != nil {
			return nil, err
		}
		if err := lut.InsertStage(AtBegin, st); err != nil {
			return nil, err
		}
	}

	// On the output side too.
	if hTransform.ExitColorSpace == SigLabData && version < 4.0 {
		dwFlags |= FlagsNoWhiteOnWhiteFixup
		st, err := ctx.stageAllocLabV4ToV2()
		if err != nil {
			return nil, err
		}
		if err := lut.InsertStage(AtEnd, st); err != nil {
			return nil, err
		}
	}

	hProfile := ctx.CreateProfilePlaceholder()
	if hProfile == nil {
		return nil, ctx.signalError(ErrNull, "cmsTransform2DeviceLink: cannot allocate profile")
	}

	hProfile.SetProfileVersion(version)

	fixColorSpaces(hProfile, hTransform.EntryColorSpace, hTransform.ExitColorSpace, dwFlags)

	// Optimize the LUT and precalculate a devicelink.
	chansIn := ChannelsOfColorSpace(hTransform.EntryColorSpace)
	chansOut := ChannelsOfColorSpace(hTransform.ExitColorSpace)

	colorSpaceBitsIn := uint32(LCMScolorSpace(hTransform.EntryColorSpace))
	colorSpaceBitsOut := uint32(LCMScolorSpace(hTransform.ExitColorSpace))

	frmIn := colorspaceSH(colorSpaceBitsIn) | channelsSH(uint32(chansIn)) | bytesSH(2)
	frmOut := colorspaceSH(colorSpaceBitsOut) | channelsSH(uint32(chansOut)) | bytesSH(2)

	deviceClass := hProfile.GetDeviceClass()

	var destinationTag TagSignature
	if deviceClass == SigOutputClass {
		destinationTag = SigBToA0Tag
	} else {
		destinationTag = SigAToB0Tag
	}

	// Check if the profile/version can store the result.
	var allowed *allowedLUT
	if dwFlags&FlagsForceCLUT != 0 {
		allowed = nil
	} else {
		allowed = findCombination(lut, version >= 4.0, destinationTag)
	}

	if allowed == nil {
		// Try to optimize.
		ctx.optimizePipeline(&lut, hTransform.RenderingIntent, &frmIn, &frmOut, &dwFlags)
		allowed = findCombination(lut, version >= 4.0, destinationTag)
	}

	// If no way, then force a CLUT that can definitely be written.
	if allowed == nil {
		dwFlags |= FlagsForceCLUT
		ctx.optimizePipeline(&lut, hTransform.RenderingIntent, &frmIn, &frmOut, &dwFlags)

		// Put identity curves in if needed.
		if first := lut.GetPtrToFirstStage(); first != nil && first.StageType() != SigCurveSetElemType {
			st, err := ctx.stageAllocIdentityCurves(uint32(chansIn))
			if err != nil {
				return nil, err
			}
			if err := lut.InsertStage(AtBegin, st); err != nil {
				return nil, err
			}
		}
		if last := lut.GetPtrToLastStage(); last != nil && last.StageType() != SigCurveSetElemType {
			st, err := ctx.stageAllocIdentityCurves(uint32(chansOut))
			if err != nil {
				return nil, err
			}
			if err := lut.InsertStage(AtEnd, st); err != nil {
				return nil, err
			}
		}

		allowed = findCombination(lut, version >= 4.0, destinationTag)
	}

	// Something is wrong...
	if allowed == nil {
		return nil, ctx.signalError(ErrNull, "cmsTransform2DeviceLink: cannot find a suitable LUT layout")
	}

	if dwFlags&Flags8BitsDeviceLink != 0 {
		lut.SetSaveAs8bitsFlag(true)
	}

	// Tag profile with information.
	if err := hProfile.setTextTags("devicelink"); err != nil {
		return nil, err
	}

	// Store the result.
	if err := hProfile.WriteTag(destinationTag, lut); err != nil {
		return nil, err
	}

	// Colorant tables have special rules depending on deviceClass.
	if hTransform.InputColorant != nil &&
		(deviceClass == SigLinkClass || deviceClass == SigInputClass) {
		if err := hProfile.WriteTag(SigColorantTableTag, hTransform.InputColorant); err != nil {
			return nil, err
		}
	}

	if hTransform.OutputColorant != nil {
		if deviceClass == SigLinkClass {
			if err := hProfile.WriteTag(SigColorantTableOutTag, hTransform.OutputColorant); err != nil {
				return nil, err
			}
		} else {
			if err := hProfile.WriteTag(SigColorantTableTag, hTransform.OutputColorant); err != nil {
				return nil, err
			}
		}
	}

	if deviceClass == SigLinkClass && hTransform.Sequence != nil {
		if err := hProfile.WriteProfileSequence(hTransform.Sequence); err != nil {
			return nil, err
		}
	}

	// Set the white point.
	if deviceClass == SigInputClass {
		wp := hTransform.EntryWhitePoint
		if err := hProfile.WriteTag(SigMediaWhitePointTag, &wp); err != nil {
			return nil, err
		}
	} else {
		wp := hTransform.ExitWhitePoint
		if err := hProfile.WriteTag(SigMediaWhitePointTag, &wp); err != nil {
			return nil, err
		}
	}

	// Per 7.2.15 in spec 4.3.
	hProfile.SetHeaderRenderingIntent(hTransform.RenderingIntent)

	return hProfile, nil
}

// createNamedColorDevicelink ports CreateNamedColorDevicelink: dump a
// transform's named-color database into a single named-color profile. This lets
// LittleCMS "group" several named-color databases into one profile. PCS is
// always Lab (the normal PCS for named-color profiles). The device colorants of
// the resulting ncl2 are recomputed by running each named-color index through
// the transform into the exit color space.
func createNamedColorDevicelink(ctx *Context, hTransform *Transform) (*Profile, error) {
	hICC := ctx.CreateProfilePlaceholder()
	if hICC == nil {
		return nil, ctx.signalError(ErrNull, "CreateNamedColorDevicelink: cannot allocate profile")
	}

	// Critical information.
	hICC.SetDeviceClass(SigNamedColorClass)
	hICC.SetColorSpace(hTransform.ExitColorSpace)
	hICC.SetPCS(SigLabData)

	// Tag profile with information.
	if err := hICC.setTextTags("Named color devicelink"); err != nil {
		return nil, err
	}

	original := hTransform.GetNamedColorList()
	if original == nil {
		return nil, ctx.signalError(ErrNull, "CreateNamedColorDevicelink: transform has no named-color list")
	}

	nColors := original.Count()
	nc2 := original.Dup()
	if nc2 == nil {
		return nil, ctx.signalError(ErrNull, "CreateNamedColorDevicelink: cannot duplicate named-color list")
	}

	// Colorant count now depends on the output space.
	nc2.colorantCount = hTransform.Lut.OutputChannelsCount()

	// Make sure we have proper formatters: index -> exit colorspace, 16-bit.
	// ChannelsOfColorSpace returns -1 for an unrecognized space; guard before it
	// reaches make([]byte, chans*2), which would otherwise panic on a negative
	// length.
	chans := ChannelsOfColorSpace(hTransform.ExitColorSpace)
	if chans <= 0 || chans > maxChannels {
		return nil, ctx.signalError(ErrColorspaceCheck, "CreateNamedColorDevicelink: invalid exit colorspace channel count %d", chans)
	}
	outFormat := floatSH(0) | colorspaceSH(uint32(LCMScolorSpace(hTransform.ExitColorSpace))) |
		bytesSH(2) | channelsSH(uint32(chans))
	if err := hTransform.ChangeBuffersFormat(psTypeNamedColorIdx, outFormat); err != nil {
		return nil, err
	}

	// Apply the transform to the colorant indices.
	in := make([]byte, 2)
	out := make([]byte, int(chans)*2)
	for i := uint32(0); i < nColors && i < uint32(len(nc2.list)); i++ {
		binary.LittleEndian.PutUint16(in, uint16(i))
		hTransform.DoTransform(in, out, 1)
		for c := int32(0); c < chans && c < maxChannels; c++ {
			nc2.list[i].DeviceColorant[c] = binary.LittleEndian.Uint16(out[c*2:])
		}
	}

	if err := hICC.WriteTag(SigNamedColor2Tag, nc2); err != nil {
		return nil, err
	}

	return hICC, nil
}
