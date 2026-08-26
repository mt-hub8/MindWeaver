// Port of the built-in tag descriptor table (SupportedTags[]) and the write-type
// selectors (DecideXYZtype, DecideCurveType, DecideTextType, DecideTextDescType,
// DecideLUTtypeA2B, DecideLUTtypeB2A) from the end of src/cmstypes.c (lcms2
// 2.19). Registering these descriptors activates the container's read-path type
// check and the SaveTags write-path type selection (including link collapsing of
// shared tags via CompatibleTypes).
//
// The descriptors reference tag-type signatures only (all defined in sigs.go),
// so this table is independent of which type handlers have been registered:
// several referenced types (text family, MLU, XYZ, named colour, measurement,
// screening, ...) are implemented by the parallel worker W8a and register into
// the same built-in table at init.

package lcms2

// Broken/vendor tag-type aliases the reference recognises on read only
// (cmstypes.c). Their descriptors list them as secondary supported types so a
// profile that stores XYZ as Corbis' broken type, or a curve as Monaco's broken
// type, is still accepted. The alias TYPE HANDLERS (which reuse the XYZ / Curve
// readers under these signatures) are registered by W8a alongside the XYZ and
// Curve handlers; see the integration note in the worker report.
const (
	sigCorbisBrokenXYZType   TagTypeSignature = 0x17A505B8
	sigMonacoBrokenCurveType TagTypeSignature = 0x9478ee00
)

// ------------------------------------------------------------- write selectors

// decideXYZtype ports DecideXYZtype: always cmsSigXYZType.
func decideXYZtype(iccVersion float64, data any) TagTypeSignature {
	return SigXYZType
}

// decideCurveType ports DecideCurveType: parametric only for a single-segment,
// non-inverted ICC-parametric curve on a V4+ profile; otherwise tabulated.
func decideCurveType(iccVersion float64, data any) TagTypeSignature {
	curve, ok := data.(*ToneCurve)
	if !ok || curve == nil {
		return SigCurveType
	}
	if iccVersion < 4.0 {
		return SigCurveType
	}
	if curve.nSegments != 1 { // Only 1-segment curves can be saved as parametric
		return SigCurveType
	}
	if len(curve.segments) < 1 {
		return SigCurveType
	}
	if curve.segments[0].Type < 0 { // Only non-inverted curves
		return SigCurveType
	}
	if curve.segments[0].Type > 5 { // Only ICC parametric curves
		return SigCurveType
	}
	return SigParametricCurveType
}

// decideTextType ports DecideTextType: MLU on V4+, else plain text.
func decideTextType(iccVersion float64, data any) TagTypeSignature {
	if iccVersion >= 4.0 {
		return SigMultiLocalizedUnicodeType
	}
	return SigTextType
}

// decideTextDescType ports DecideTextDescType: MLU on V4+, else textDescription.
func decideTextDescType(iccVersion float64, data any) TagTypeSignature {
	if iccVersion >= 4.0 {
		return SigMultiLocalizedUnicodeType
	}
	return SigTextDescriptionType
}

// decideLUTtypeA2B ports DecideLUTtypeA2B.
func decideLUTtypeA2B(iccVersion float64, data any) TagTypeSignature {
	lut, ok := data.(*Pipeline)
	if iccVersion < 4.0 {
		if ok && lut != nil && lut.saveAs8Bits {
			return SigLut8Type
		}
		return SigLut16Type
	}
	return SigLutAtoBType
}

// decideLUTtypeB2A ports DecideLUTtypeB2A.
func decideLUTtypeB2A(iccVersion float64, data any) TagTypeSignature {
	lut, ok := data.(*Pipeline)
	if iccVersion < 4.0 {
		if ok && lut != nil && lut.saveAs8Bits {
			return SigLut8Type
		}
		return SigLut16Type
	}
	return SigLutBtoAType
}

// ------------------------------------------------------------- descriptor table

func init() {
	reg := func(sig TagSignature, elemCount uint32, types []TagTypeSignature, decide func(float64, any) TagTypeSignature) {
		registerBuiltinTag(sig, &TagDescriptor{
			ElemCount:      elemCount,
			SupportedTypes: types,
			DecideType:     decide,
		})
	}

	reg(SigAToB0Tag, 1, []TagTypeSignature{SigLut16Type, SigLutAtoBType, SigLut8Type}, decideLUTtypeA2B)
	reg(SigAToB1Tag, 1, []TagTypeSignature{SigLut16Type, SigLutAtoBType, SigLut8Type}, decideLUTtypeA2B)
	reg(SigAToB2Tag, 1, []TagTypeSignature{SigLut16Type, SigLutAtoBType, SigLut8Type}, decideLUTtypeA2B)
	reg(SigBToA0Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)
	reg(SigBToA1Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)
	reg(SigBToA2Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)

	// Allow Corbis and its broken XYZ type.
	reg(SigRedColorantTag, 1, []TagTypeSignature{SigXYZType, sigCorbisBrokenXYZType}, decideXYZtype)
	reg(SigGreenColorantTag, 1, []TagTypeSignature{SigXYZType, sigCorbisBrokenXYZType}, decideXYZtype)
	reg(SigBlueColorantTag, 1, []TagTypeSignature{SigXYZType, sigCorbisBrokenXYZType}, decideXYZtype)

	reg(SigRedTRCTag, 1, []TagTypeSignature{SigCurveType, SigParametricCurveType, sigMonacoBrokenCurveType}, decideCurveType)
	reg(SigGreenTRCTag, 1, []TagTypeSignature{SigCurveType, SigParametricCurveType, sigMonacoBrokenCurveType}, decideCurveType)
	reg(SigBlueTRCTag, 1, []TagTypeSignature{SigCurveType, SigParametricCurveType, sigMonacoBrokenCurveType}, decideCurveType)

	reg(SigCalibrationDateTimeTag, 1, []TagTypeSignature{SigDateTimeType}, nil)
	reg(SigCharTargetTag, 1, []TagTypeSignature{SigTextType}, nil)

	reg(SigChromaticAdaptationTag, 9, []TagTypeSignature{SigS15Fixed16ArrayType}, nil)
	reg(SigChromaticityTag, 1, []TagTypeSignature{SigChromaticityType}, nil)
	reg(SigColorantOrderTag, 1, []TagTypeSignature{SigColorantOrderType}, nil)
	reg(SigColorantTableTag, 1, []TagTypeSignature{SigColorantTableType}, nil)
	reg(SigColorantTableOutTag, 1, []TagTypeSignature{SigColorantTableType}, nil)

	reg(SigCopyrightTag, 1, []TagTypeSignature{SigTextType, SigMultiLocalizedUnicodeType, SigTextDescriptionType}, decideTextType)
	reg(SigDateTimeTag, 1, []TagTypeSignature{SigDateTimeType}, nil)

	reg(SigDeviceMfgDescTag, 1, []TagTypeSignature{SigTextDescriptionType, SigMultiLocalizedUnicodeType, SigTextType}, decideTextDescType)
	reg(SigDeviceModelDescTag, 1, []TagTypeSignature{SigTextDescriptionType, SigMultiLocalizedUnicodeType, SigTextType}, decideTextDescType)

	reg(SigGamutTag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)

	reg(SigGrayTRCTag, 1, []TagTypeSignature{SigCurveType, SigParametricCurveType}, decideCurveType)
	reg(SigLuminanceTag, 1, []TagTypeSignature{SigXYZType}, nil)

	reg(SigMediaBlackPointTag, 1, []TagTypeSignature{SigXYZType, sigCorbisBrokenXYZType}, nil)
	reg(SigMediaWhitePointTag, 1, []TagTypeSignature{SigXYZType, sigCorbisBrokenXYZType}, nil)

	reg(SigNamedColor2Tag, 1, []TagTypeSignature{SigNamedColor2Type}, nil)

	reg(SigPreview0Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)
	reg(SigPreview1Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)
	reg(SigPreview2Tag, 1, []TagTypeSignature{SigLut16Type, SigLutBtoAType, SigLut8Type}, decideLUTtypeB2A)

	reg(SigProfileDescriptionTag, 1, []TagTypeSignature{SigTextDescriptionType, SigMultiLocalizedUnicodeType, SigTextType}, decideTextDescType)
	reg(SigProfileSequenceDescTag, 1, []TagTypeSignature{SigProfileSequenceDescType}, nil)
	reg(SigTechnologyTag, 1, []TagTypeSignature{SigSignatureType}, nil)

	reg(SigColorimetricIntentImageStateTag, 1, []TagTypeSignature{SigSignatureType}, nil)
	reg(SigPerceptualRenderingIntentGamutTag, 1, []TagTypeSignature{SigSignatureType}, nil)
	reg(SigSaturationRenderingIntentGamutTag, 1, []TagTypeSignature{SigSignatureType}, nil)

	reg(SigMeasurementTag, 1, []TagTypeSignature{SigMeasurementType}, nil)

	reg(SigPs2CRD0Tag, 1, []TagTypeSignature{SigDataType}, nil)
	reg(SigPs2CRD1Tag, 1, []TagTypeSignature{SigDataType}, nil)
	reg(SigPs2CRD2Tag, 1, []TagTypeSignature{SigDataType}, nil)
	reg(SigPs2CRD3Tag, 1, []TagTypeSignature{SigDataType}, nil)
	reg(SigPs2CSATag, 1, []TagTypeSignature{SigDataType}, nil)
	reg(SigPs2RenderingIntentTag, 1, []TagTypeSignature{SigDataType}, nil)

	reg(SigViewingCondDescTag, 1, []TagTypeSignature{SigTextDescriptionType, SigMultiLocalizedUnicodeType, SigTextType}, decideTextDescType)

	reg(SigUcrBgTag, 1, []TagTypeSignature{SigUcrBgType}, nil)
	reg(SigCrdInfoTag, 1, []TagTypeSignature{SigCrdInfoType}, nil)

	reg(SigDToB0Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigDToB1Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigDToB2Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigDToB3Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigBToD0Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigBToD1Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigBToD2Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)
	reg(SigBToD3Tag, 1, []TagTypeSignature{SigMultiProcessElementType}, nil)

	reg(SigScreeningDescTag, 1, []TagTypeSignature{SigTextDescriptionType}, nil)
	reg(SigViewingConditionsTag, 1, []TagTypeSignature{SigViewingConditionsType}, nil)

	reg(SigScreeningTag, 1, []TagTypeSignature{SigScreeningType}, nil)
	reg(SigVcgtTag, 1, []TagTypeSignature{SigVcgtType}, nil)
	reg(SigMetaTag, 1, []TagTypeSignature{SigDictType}, nil)
	reg(SigProfileSequenceIdTag, 1, []TagTypeSignature{SigProfileSequenceIdType}, nil)

	reg(SigProfileDescriptionMLTag, 1, []TagTypeSignature{SigMultiLocalizedUnicodeType}, nil)
	reg(SigcicpTag, 1, []TagTypeSignature{SigcicpType}, nil)

	reg(SigArgyllArtsTag, 9, []TagTypeSignature{SigS15Fixed16ArrayType}, nil)
	reg(SigMHC2Tag, 1, []TagTypeSignature{SigMHC2Type}, nil)
}
