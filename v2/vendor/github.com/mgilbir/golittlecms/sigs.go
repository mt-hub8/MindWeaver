package lcms2

// This file defines the signature and pixel-type constants ported from
// include/lcms2.h. The names follow the PLAN convention of dropping the "cms"
// prefix. The colorimetry code (pcs.go) needs the ColorSpaceSignature subset;
// Phase 3 (profile I/O) grows the set to the full tag, tag-type, device-class
// and platform signatures, plus the ICC container constants.

// ICC container constants (include/lcms2.h).
const (
	// MagicNumber is the ICC profile magic 'acsp' found at offset 36 of every
	// profile header (cmsMagicNumber).
	MagicNumber uint32 = 0x61637370 // 'acsp'
	// LcmsSignature is Little CMS's own four-byte signature 'lcms'
	// (lcmsSignature), used as the default CMM and creator.
	LcmsSignature uint32 = 0x6c636d73 // 'lcms'
	// MaxChannels mirrors cmsMAXCHANNELS.
	MaxChannels = maxChannels
	// maxTableTag mirrors MAX_TABLE_TAG (lcms2_internal.h): the maximum number
	// of tags a profile directory may hold.
	maxTableTag = 100
	// maxTypesInPlugin mirrors MAX_TYPES_IN_LCMS_PLUGIN (lcms2_plugin.h).
	maxTypesInPlugin = 20
)

// sigToString renders a four-byte signature as its ASCII characters, most
// significant byte first, mirroring _cmsTagSignature2String (cmserr.c). Bytes
// are emitted verbatim, so non-printable signatures render their raw bytes.
func sigToString(sig uint32) string {
	return string([]byte{byte(sig >> 24), byte(sig >> 16), byte(sig >> 8), byte(sig)})
}

// TagSignature mirrors cmsTagSignature: the four-byte signature that names an
// entry in a profile's tag directory.
type TagSignature uint32

// String renders the signature as four ASCII characters (big-endian order).
func (s TagSignature) String() string { return sigToString(uint32(s)) }

// ICC tag signatures (cmsTagSignature, include/lcms2.h). Values are the
// big-endian four-character codes.
const (
	SigAToB0Tag                          TagSignature = 0x41324230 // 'A2B0'
	SigAToB1Tag                          TagSignature = 0x41324231 // 'A2B1'
	SigAToB2Tag                          TagSignature = 0x41324232 // 'A2B2'
	SigBlueColorantTag                   TagSignature = 0x6258595A // 'bXYZ'
	SigBlueMatrixColumnTag               TagSignature = 0x6258595A // 'bXYZ'
	SigBlueTRCTag                        TagSignature = 0x62545243 // 'bTRC'
	SigBToA0Tag                          TagSignature = 0x42324130 // 'B2A0'
	SigBToA1Tag                          TagSignature = 0x42324131 // 'B2A1'
	SigBToA2Tag                          TagSignature = 0x42324132 // 'B2A2'
	SigCalibrationDateTimeTag            TagSignature = 0x63616C74 // 'calt'
	SigCharTargetTag                     TagSignature = 0x74617267 // 'targ'
	SigChromaticAdaptationTag            TagSignature = 0x63686164 // 'chad'
	SigChromaticityTag                   TagSignature = 0x6368726D // 'chrm'
	SigColorantOrderTag                  TagSignature = 0x636C726F // 'clro'
	SigColorantTableTag                  TagSignature = 0x636C7274 // 'clrt'
	SigColorantTableOutTag               TagSignature = 0x636C6F74 // 'clot'
	SigColorimetricIntentImageStateTag   TagSignature = 0x63696973 // 'ciis'
	SigCopyrightTag                      TagSignature = 0x63707274 // 'cprt'
	SigCrdInfoTag                        TagSignature = 0x63726469 // 'crdi'
	SigDataTag                           TagSignature = 0x64617461 // 'data'
	SigDateTimeTag                       TagSignature = 0x6474696D // 'dtim'
	SigDeviceMfgDescTag                  TagSignature = 0x646D6E64 // 'dmnd'
	SigDeviceModelDescTag                TagSignature = 0x646D6464 // 'dmdd'
	SigDeviceSettingsTag                 TagSignature = 0x64657673 // 'devs'
	SigDToB0Tag                          TagSignature = 0x44324230 // 'D2B0'
	SigDToB1Tag                          TagSignature = 0x44324231 // 'D2B1'
	SigDToB2Tag                          TagSignature = 0x44324232 // 'D2B2'
	SigDToB3Tag                          TagSignature = 0x44324233 // 'D2B3'
	SigBToD0Tag                          TagSignature = 0x42324430 // 'B2D0'
	SigBToD1Tag                          TagSignature = 0x42324431 // 'B2D1'
	SigBToD2Tag                          TagSignature = 0x42324432 // 'B2D2'
	SigBToD3Tag                          TagSignature = 0x42324433 // 'B2D3'
	SigGamutTag                          TagSignature = 0x67616D74 // 'gamt'
	SigGrayTRCTag                        TagSignature = 0x6b545243 // 'kTRC'
	SigGreenColorantTag                  TagSignature = 0x6758595A // 'gXYZ'
	SigGreenMatrixColumnTag              TagSignature = 0x6758595A // 'gXYZ'
	SigGreenTRCTag                       TagSignature = 0x67545243 // 'gTRC'
	SigLuminanceTag                      TagSignature = 0x6C756d69 // 'lumi'
	SigMeasurementTag                    TagSignature = 0x6D656173 // 'meas'
	SigMediaBlackPointTag                TagSignature = 0x626B7074 // 'bkpt'
	SigMediaWhitePointTag                TagSignature = 0x77747074 // 'wtpt'
	SigNamedColorTag                     TagSignature = 0x6E636f6C // 'ncol'
	SigNamedColor2Tag                    TagSignature = 0x6E636C32 // 'ncl2'
	SigOutputResponseTag                 TagSignature = 0x72657370 // 'resp'
	SigPerceptualRenderingIntentGamutTag TagSignature = 0x72696730 // 'rig0'
	SigPreview0Tag                       TagSignature = 0x70726530 // 'pre0'
	SigPreview1Tag                       TagSignature = 0x70726531 // 'pre1'
	SigPreview2Tag                       TagSignature = 0x70726532 // 'pre2'
	SigProfileDescriptionTag             TagSignature = 0x64657363 // 'desc'
	SigProfileDescriptionMLTag           TagSignature = 0x6473636d // 'dscm'
	SigProfileSequenceDescTag            TagSignature = 0x70736571 // 'pseq'
	SigProfileSequenceIdTag              TagSignature = 0x70736964 // 'psid'
	SigPs2CRD0Tag                        TagSignature = 0x70736430 // 'psd0'
	SigPs2CRD1Tag                        TagSignature = 0x70736431 // 'psd1'
	SigPs2CRD2Tag                        TagSignature = 0x70736432 // 'psd2'
	SigPs2CRD3Tag                        TagSignature = 0x70736433 // 'psd3'
	SigPs2CSATag                         TagSignature = 0x70733273 // 'ps2s'
	SigPs2RenderingIntentTag             TagSignature = 0x70733269 // 'ps2i'
	SigRedColorantTag                    TagSignature = 0x7258595A // 'rXYZ'
	SigRedMatrixColumnTag                TagSignature = 0x7258595A // 'rXYZ'
	SigRedTRCTag                         TagSignature = 0x72545243 // 'rTRC'
	SigSaturationRenderingIntentGamutTag TagSignature = 0x72696732 // 'rig2'
	SigScreeningDescTag                  TagSignature = 0x73637264 // 'scrd'
	SigScreeningTag                      TagSignature = 0x7363726E // 'scrn'
	SigTechnologyTag                     TagSignature = 0x74656368 // 'tech'
	SigUcrBgTag                          TagSignature = 0x62666420 // 'bfd '
	SigViewingCondDescTag                TagSignature = 0x76756564 // 'vued'
	SigViewingConditionsTag              TagSignature = 0x76696577 // 'view'
	SigVcgtTag                           TagSignature = 0x76636774 // 'vcgt'
	SigMetaTag                           TagSignature = 0x6D657461 // 'meta'
	SigcicpTag                           TagSignature = 0x63696370 // 'cicp'
	SigArgyllArtsTag                     TagSignature = 0x61727473 // 'arts'
	SigMHC2Tag                           TagSignature = 0x4D484332 // 'MHC2'
)

// TagTypeSignature mirrors cmsTagTypeSignature: the four-byte signature that
// identifies the serialized type of a tag's contents.
type TagTypeSignature uint32

// String renders the signature as four ASCII characters (big-endian order).
func (s TagTypeSignature) String() string { return sigToString(uint32(s)) }

// ICC tag-type signatures (cmsTagTypeSignature, include/lcms2.h).
const (
	SigChromaticityType          TagTypeSignature = 0x6368726D // 'chrm'
	SigcicpType                  TagTypeSignature = 0x63696370 // 'cicp'
	SigColorantOrderType         TagTypeSignature = 0x636C726F // 'clro'
	SigColorantTableType         TagTypeSignature = 0x636C7274 // 'clrt'
	SigCrdInfoType               TagTypeSignature = 0x63726469 // 'crdi'
	SigCurveType                 TagTypeSignature = 0x63757276 // 'curv'
	SigDataType                  TagTypeSignature = 0x64617461 // 'data'
	SigDictType                  TagTypeSignature = 0x64696374 // 'dict'
	SigDateTimeType              TagTypeSignature = 0x6474696D // 'dtim'
	SigDeviceSettingsType        TagTypeSignature = 0x64657673 // 'devs'
	SigLut16Type                 TagTypeSignature = 0x6d667432 // 'mft2'
	SigLut8Type                  TagTypeSignature = 0x6d667431 // 'mft1'
	SigLutAtoBType               TagTypeSignature = 0x6d414220 // 'mAB '
	SigLutBtoAType               TagTypeSignature = 0x6d424120 // 'mBA '
	SigMeasurementType           TagTypeSignature = 0x6D656173 // 'meas'
	SigMultiLocalizedUnicodeType TagTypeSignature = 0x6D6C7563 // 'mluc'
	SigMultiProcessElementType   TagTypeSignature = 0x6D706574 // 'mpet'
	SigNamedColorType            TagTypeSignature = 0x6E636f6C // 'ncol'
	SigNamedColor2Type           TagTypeSignature = 0x6E636C32 // 'ncl2'
	SigParametricCurveType       TagTypeSignature = 0x70617261 // 'para'
	SigProfileSequenceDescType   TagTypeSignature = 0x70736571 // 'pseq'
	SigProfileSequenceIdType     TagTypeSignature = 0x70736964 // 'psid'
	SigResponseCurveSet16Type    TagTypeSignature = 0x72637332 // 'rcs2'
	SigS15Fixed16ArrayType       TagTypeSignature = 0x73663332 // 'sf32'
	SigScreeningType             TagTypeSignature = 0x7363726E // 'scrn'
	SigSignatureType             TagTypeSignature = 0x73696720 // 'sig '
	SigTextType                  TagTypeSignature = 0x74657874 // 'text'
	SigTextDescriptionType       TagTypeSignature = 0x64657363 // 'desc'
	SigU16Fixed16ArrayType       TagTypeSignature = 0x75663332 // 'uf32'
	SigUcrBgType                 TagTypeSignature = 0x62666420 // 'bfd '
	SigUInt16ArrayType           TagTypeSignature = 0x75693136 // 'ui16'
	SigUInt32ArrayType           TagTypeSignature = 0x75693332 // 'ui32'
	SigUInt64ArrayType           TagTypeSignature = 0x75693634 // 'ui64'
	SigUInt8ArrayType            TagTypeSignature = 0x75693038 // 'ui08'
	SigVcgtType                  TagTypeSignature = 0x76636774 // 'vcgt'
	SigViewingConditionsType     TagTypeSignature = 0x76696577 // 'view'
	SigXYZType                   TagTypeSignature = 0x58595A20 // 'XYZ '
	SigMHC2Type                  TagTypeSignature = 0x4D484332 // 'MHC2'
)

// ProfileClassSignature mirrors cmsProfileClassSignature: the profile/device
// class in the header.
type ProfileClassSignature uint32

// String renders the signature as four ASCII characters (big-endian order).
func (s ProfileClassSignature) String() string { return sigToString(uint32(s)) }

// ICC profile-class signatures (cmsProfileClassSignature, include/lcms2.h).
const (
	SigInputClass                   ProfileClassSignature = 0x73636E72 // 'scnr'
	SigDisplayClass                 ProfileClassSignature = 0x6D6E7472 // 'mntr'
	SigOutputClass                  ProfileClassSignature = 0x70727472 // 'prtr'
	SigLinkClass                    ProfileClassSignature = 0x6C696E6B // 'link'
	SigAbstractClass                ProfileClassSignature = 0x61627374 // 'abst'
	SigColorSpaceClass              ProfileClassSignature = 0x73706163 // 'spac'
	SigNamedColorClass              ProfileClassSignature = 0x6e6d636c // 'nmcl'
	SigColorEncodingSpaceClass      ProfileClassSignature = 0x63656E63 // 'cenc'
	SigMultiplexIdentificationClass ProfileClassSignature = 0x6D696420 // 'mid '
	SigMultiplexLinkClass           ProfileClassSignature = 0x6d6c6e6b // 'mlnk'
	SigMultiplexVisualizationClass  ProfileClassSignature = 0x6d766973 // 'mvis'
)

// PlatformSignature mirrors cmsPlatformSignature: the primary platform in the
// header.
type PlatformSignature uint32

// String renders the signature as four ASCII characters (big-endian order).
func (s PlatformSignature) String() string { return sigToString(uint32(s)) }

// ICC platform signatures (cmsPlatformSignature, include/lcms2.h).
const (
	SigMacintosh PlatformSignature = 0x4150504C // 'APPL'
	SigMicrosoft PlatformSignature = 0x4D534654 // 'MSFT'
	SigSolaris   PlatformSignature = 0x53554E57 // 'SUNW'
	SigSGI       PlatformSignature = 0x53474920 // 'SGI '
	SigTaligent  PlatformSignature = 0x54474E54 // 'TGNT'
	SigUnices    PlatformSignature = 0x2A6E6978 // '*nix'
)

// SigNamedData ('nmcl') mirrors cmsSigNamedData in the color-space enumeration;
// it is defined here to complete the set started below.
const SigNamedData ColorSpaceSignature = 0x6e6d636c // 'nmcl'

// String renders the color-space signature as four ASCII characters (big-endian
// order).
func (s ColorSpaceSignature) String() string { return sigToString(uint32(s)) }

// ColorSpaceSignature mirrors cmsColorSpaceSignature: an ICC four-byte color
// space signature.
type ColorSpaceSignature uint32

// ICC color space signatures (subset). Values are the big-endian four-character
// codes exactly as defined in include/lcms2.h.
const (
	SigXYZData     ColorSpaceSignature = 0x58595A20 // 'XYZ '
	SigLabData     ColorSpaceSignature = 0x4C616220 // 'Lab '
	SigLuvData     ColorSpaceSignature = 0x4C757620 // 'Luv '
	SigYCbCrData   ColorSpaceSignature = 0x59436272 // 'YCbr'
	SigYxyData     ColorSpaceSignature = 0x59787920 // 'Yxy '
	SigRgbData     ColorSpaceSignature = 0x52474220 // 'RGB '
	SigGrayData    ColorSpaceSignature = 0x47524159 // 'GRAY'
	SigHsvData     ColorSpaceSignature = 0x48535620 // 'HSV '
	SigHlsData     ColorSpaceSignature = 0x484C5320 // 'HLS '
	SigCmykData    ColorSpaceSignature = 0x434D594B // 'CMYK'
	SigCmyData     ColorSpaceSignature = 0x434D5920 // 'CMY '
	SigMCH1Data    ColorSpaceSignature = 0x4D434831 // 'MCH1'
	SigMCH2Data    ColorSpaceSignature = 0x4D434832 // 'MCH2'
	SigMCH3Data    ColorSpaceSignature = 0x4D434833 // 'MCH3'
	SigMCH4Data    ColorSpaceSignature = 0x4D434834 // 'MCH4'
	SigMCH5Data    ColorSpaceSignature = 0x4D434835 // 'MCH5'
	SigMCH6Data    ColorSpaceSignature = 0x4D434836 // 'MCH6'
	SigMCH7Data    ColorSpaceSignature = 0x4D434837 // 'MCH7'
	SigMCH8Data    ColorSpaceSignature = 0x4D434838 // 'MCH8'
	SigMCH9Data    ColorSpaceSignature = 0x4D434839 // 'MCH9'
	SigMCHAData    ColorSpaceSignature = 0x4D434841 // 'MCHA'
	SigMCHBData    ColorSpaceSignature = 0x4D434842 // 'MCHB'
	SigMCHCData    ColorSpaceSignature = 0x4D434843 // 'MCHC'
	SigMCHDData    ColorSpaceSignature = 0x4D434844 // 'MCHD'
	SigMCHEData    ColorSpaceSignature = 0x4D434845 // 'MCHE'
	SigMCHFData    ColorSpaceSignature = 0x4D434846 // 'MCHF'
	Sig1colorData  ColorSpaceSignature = 0x31434C52 // '1CLR'
	Sig2colorData  ColorSpaceSignature = 0x32434C52 // '2CLR'
	Sig3colorData  ColorSpaceSignature = 0x33434C52 // '3CLR'
	Sig4colorData  ColorSpaceSignature = 0x34434C52 // '4CLR'
	Sig5colorData  ColorSpaceSignature = 0x35434C52 // '5CLR'
	Sig6colorData  ColorSpaceSignature = 0x36434C52 // '6CLR'
	Sig7colorData  ColorSpaceSignature = 0x37434C52 // '7CLR'
	Sig8colorData  ColorSpaceSignature = 0x38434C52 // '8CLR'
	Sig9colorData  ColorSpaceSignature = 0x39434C52 // '9CLR'
	Sig10colorData ColorSpaceSignature = 0x41434C52 // 'ACLR'
	Sig11colorData ColorSpaceSignature = 0x42434C52 // 'BCLR'
	Sig12colorData ColorSpaceSignature = 0x43434C52 // 'CCLR'
	Sig13colorData ColorSpaceSignature = 0x44434C52 // 'DCLR'
	Sig14colorData ColorSpaceSignature = 0x45434C52 // 'ECLR'
	Sig15colorData ColorSpaceSignature = 0x46434C52 // 'FCLR'
	SigLuvKData    ColorSpaceSignature = 0x4C75764B // 'LuvK'
)

// Pixel-type notations, mirroring the PT_* macros in include/lcms2.h. These are
// lcms2's internal color-space enumeration used by _cmsICCcolorSpace /
// _cmsLCMScolorSpace.
const (
	PTANY   = 0 // PT_ANY: matches any color space (T_COLORSPACE wildcard)
	PTGray  = 3
	PTRGB   = 4
	PTCMY   = 5
	PTCMYK  = 6
	PTYCbCr = 7
	PTYUV   = 8 // Lu'v'
	PTXYZ   = 9
	PTLab   = 10
	PTYUVK  = 11 // Lu'v'K
	PTHSV   = 12
	PTHLS   = 13
	PTYxy   = 14
	PTMCH1  = 15
	PTMCH2  = 16
	PTMCH3  = 17
	PTMCH4  = 18
	PTMCH5  = 19
	PTMCH6  = 20
	PTMCH7  = 21
	PTMCH8  = 22
	PTMCH9  = 23
	PTMCH10 = 24
	PTMCH11 = 25
	PTMCH12 = 26
	PTMCH13 = 27
	PTMCH14 = 28
	PTMCH15 = 29
	PTLabV2 = 30 // Identical to PT_Lab, but using the V2 old encoding
)

// Transform flags (subset) used by _cmsReasonableGridpointsByColorspace.
const (
	FlagsHighResPrecalc = 0x0400 // cmsFLAGS_HIGHRESPRECALC
	FlagsLowResPrecalc  = 0x0800 // cmsFLAGS_LOWRESPRECALC
)
