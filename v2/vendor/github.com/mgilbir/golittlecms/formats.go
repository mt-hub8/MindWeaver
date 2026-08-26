package lcms2

// This file defines the pixel-format encoding shared with the C reference:
// the *_SH bitfield builders and the TYPE_* named formats from lcms2.h. A
// format is a uint32 whose bitfields describe colour space, channel count,
// bytes per channel, and modifiers (float, planar, byte-swap, extra/alpha,
// premultiplied, ...). Use a Type* constant for a common format, or compose
// one with the *SH builders, e.g. ColorspaceSH(PTRGB)|ChannelsSH(3)|BytesSH(2).
// The numeric encoding is identical to the C TYPE_* macros.

// Pixel-format bitfield builders, mirroring the *_SH macros in lcms2.h.
// Compose them with bitwise OR to build a format value.
// PremulSH encodes the premultiplied-alpha flag.
func PremulSH(m uint32) uint32 { return m << 23 }

// FloatSH encodes the floating-point-sample flag.
func FloatSH(a uint32) uint32 { return a << 22 }

// OptimizedSH encodes the already-optimized flag (internal).
func OptimizedSH(s uint32) uint32 { return s << 21 }

// ColorspaceSH encodes the colour space (a PT* value).
func ColorspaceSH(s uint32) uint32 { return s << 16 }

// SwapFirstSH encodes the swap-first flag (e.g. ARGB vs RGBA).
func SwapFirstSH(s uint32) uint32 { return s << 14 }

// FlavorSH encodes the flavour flag (min-is-white vs min-is-black).
func FlavorSH(s uint32) uint32 { return s << 13 }

// PlanarSH encodes the planar (vs chunky) layout flag.
func PlanarSH(p uint32) uint32 { return p << 12 }

// Endian16SH encodes the 16-bit big-endian flag.
func Endian16SH(e uint32) uint32 { return e << 11 }

// DoSwapSH encodes the channel-reversal flag (e.g. BGR).
func DoSwapSH(e uint32) uint32 { return e << 10 }

// ExtraSH encodes the number of extra (e.g. alpha) channels.
func ExtraSH(e uint32) uint32 { return e << 7 }

// ChannelsSH encodes the number of colour channels.
func ChannelsSH(c uint32) uint32 { return c << 3 }

// BytesSH encodes the bytes per channel (0 encodes 8-byte double).
func BytesSH(b uint32) uint32 { return b }

// Named pixel formats, mirroring the TYPE_* macros in lcms2.h.
const (
	TypeGray8           uint32 = 196617   // TYPE_GRAY_8
	TypeGray8Rev        uint32 = 204809   // TYPE_GRAY_8_REV
	TypeGray16          uint32 = 196618   // TYPE_GRAY_16
	TypeGray16Rev       uint32 = 204810   // TYPE_GRAY_16_REV
	TypeGray16SE        uint32 = 198666   // TYPE_GRAY_16_SE
	TypeGRAYA8          uint32 = 196745   // TYPE_GRAYA_8
	TypeGRAYA8Premul    uint32 = 8585353  // TYPE_GRAYA_8_PREMUL
	TypeGRAYA16         uint32 = 196746   // TYPE_GRAYA_16
	TypeGRAYA16Premul   uint32 = 8585354  // TYPE_GRAYA_16_PREMUL
	TypeGRAYA16SE       uint32 = 198794   // TYPE_GRAYA_16_SE
	TypeGRAYA8Planar    uint32 = 200841   // TYPE_GRAYA_8_PLANAR
	TypeGRAYA16Planar   uint32 = 200842   // TYPE_GRAYA_16_PLANAR
	TypeRGB8            uint32 = 262169   // TYPE_RGB_8
	TypeRGB8Planar      uint32 = 266265   // TYPE_RGB_8_PLANAR
	TypeBGR8            uint32 = 263193   // TYPE_BGR_8
	TypeBGR8Planar      uint32 = 267289   // TYPE_BGR_8_PLANAR
	TypeRGB16           uint32 = 262170   // TYPE_RGB_16
	TypeRGB16Planar     uint32 = 266266   // TYPE_RGB_16_PLANAR
	TypeRGB16SE         uint32 = 264218   // TYPE_RGB_16_SE
	TypeBGR16           uint32 = 263194   // TYPE_BGR_16
	TypeBGR16Planar     uint32 = 267290   // TYPE_BGR_16_PLANAR
	TypeBGR16SE         uint32 = 265242   // TYPE_BGR_16_SE
	TypeRGBA8           uint32 = 262297   // TYPE_RGBA_8
	TypeRGBA8Premul     uint32 = 8650905  // TYPE_RGBA_8_PREMUL
	TypeRGBA8Planar     uint32 = 266393   // TYPE_RGBA_8_PLANAR
	TypeRGBA16          uint32 = 262298   // TYPE_RGBA_16
	TypeRGBA16Premul    uint32 = 8650906  // TYPE_RGBA_16_PREMUL
	TypeRGBA16Planar    uint32 = 266394   // TYPE_RGBA_16_PLANAR
	TypeRGBA16SE        uint32 = 264346   // TYPE_RGBA_16_SE
	TypeARGB8           uint32 = 278681   // TYPE_ARGB_8
	TypeARGB8Premul     uint32 = 8667289  // TYPE_ARGB_8_PREMUL
	TypeARGB8Planar     uint32 = 282777   // TYPE_ARGB_8_PLANAR
	TypeARGB16          uint32 = 278682   // TYPE_ARGB_16
	TypeARGB16Premul    uint32 = 8667290  // TYPE_ARGB_16_PREMUL
	TypeABGR8           uint32 = 263321   // TYPE_ABGR_8
	TypeABGR8Premul     uint32 = 8651929  // TYPE_ABGR_8_PREMUL
	TypeABGR8Planar     uint32 = 267417   // TYPE_ABGR_8_PLANAR
	TypeABGR16          uint32 = 263322   // TYPE_ABGR_16
	TypeABGR16Premul    uint32 = 8651930  // TYPE_ABGR_16_PREMUL
	TypeABGR16Planar    uint32 = 267418   // TYPE_ABGR_16_PLANAR
	TypeABGR16SE        uint32 = 265370   // TYPE_ABGR_16_SE
	TypeBGRA8           uint32 = 279705   // TYPE_BGRA_8
	TypeBGRA8Premul     uint32 = 8668313  // TYPE_BGRA_8_PREMUL
	TypeBGRA8Planar     uint32 = 283801   // TYPE_BGRA_8_PLANAR
	TypeBGRA16          uint32 = 279706   // TYPE_BGRA_16
	TypeBGRA16Premul    uint32 = 8668314  // TYPE_BGRA_16_PREMUL
	TypeBGRA16SE        uint32 = 281754   // TYPE_BGRA_16_SE
	TypeCMY8            uint32 = 327705   // TYPE_CMY_8
	TypeCMY8Planar      uint32 = 331801   // TYPE_CMY_8_PLANAR
	TypeCMY16           uint32 = 327706   // TYPE_CMY_16
	TypeCMY16Planar     uint32 = 331802   // TYPE_CMY_16_PLANAR
	TypeCMY16SE         uint32 = 329754   // TYPE_CMY_16_SE
	TypeCMYK8           uint32 = 393249   // TYPE_CMYK_8
	TypeCMYKA8          uint32 = 393377   // TYPE_CMYKA_8
	TypeCMYK8Rev        uint32 = 401441   // TYPE_CMYK_8_REV
	TypeYUVK8           uint32 = 401441   // TYPE_YUVK_8
	TypeCMYK8Planar     uint32 = 397345   // TYPE_CMYK_8_PLANAR
	TypeCMYK16          uint32 = 393250   // TYPE_CMYK_16
	TypeCMYK16Rev       uint32 = 401442   // TYPE_CMYK_16_REV
	TypeYUVK16          uint32 = 401442   // TYPE_YUVK_16
	TypeCMYK16Planar    uint32 = 397346   // TYPE_CMYK_16_PLANAR
	TypeCMYK16SE        uint32 = 395298   // TYPE_CMYK_16_SE
	TypeKYMC8           uint32 = 394273   // TYPE_KYMC_8
	TypeKYMC16          uint32 = 394274   // TYPE_KYMC_16
	TypeKYMC16SE        uint32 = 396322   // TYPE_KYMC_16_SE
	TypeKCMY8           uint32 = 409633   // TYPE_KCMY_8
	TypeKCMY8Rev        uint32 = 417825   // TYPE_KCMY_8_REV
	TypeKCMY16          uint32 = 409634   // TYPE_KCMY_16
	TypeKCMY16Rev       uint32 = 417826   // TYPE_KCMY_16_REV
	TypeKCMY16SE        uint32 = 411682   // TYPE_KCMY_16_SE
	TypeCMYK58          uint32 = 1245225  // TYPE_CMYK5_8
	TypeCMYK516         uint32 = 1245226  // TYPE_CMYK5_16
	TypeCMYK516SE       uint32 = 1247274  // TYPE_CMYK5_16_SE
	TypeKYMC58          uint32 = 1246249  // TYPE_KYMC5_8
	TypeKYMC516         uint32 = 1246250  // TYPE_KYMC5_16
	TypeKYMC516SE       uint32 = 1248298  // TYPE_KYMC5_16_SE
	TypeCMYK68          uint32 = 1310769  // TYPE_CMYK6_8
	TypeCMYK68Planar    uint32 = 1314865  // TYPE_CMYK6_8_PLANAR
	TypeCMYK616         uint32 = 1310770  // TYPE_CMYK6_16
	TypeCMYK616Planar   uint32 = 1314866  // TYPE_CMYK6_16_PLANAR
	TypeCMYK616SE       uint32 = 1312818  // TYPE_CMYK6_16_SE
	TypeCMYK78          uint32 = 1376313  // TYPE_CMYK7_8
	TypeCMYK716         uint32 = 1376314  // TYPE_CMYK7_16
	TypeCMYK716SE       uint32 = 1378362  // TYPE_CMYK7_16_SE
	TypeKYMC78          uint32 = 1377337  // TYPE_KYMC7_8
	TypeKYMC716         uint32 = 1377338  // TYPE_KYMC7_16
	TypeKYMC716SE       uint32 = 1379386  // TYPE_KYMC7_16_SE
	TypeCMYK88          uint32 = 1441857  // TYPE_CMYK8_8
	TypeCMYK816         uint32 = 1441858  // TYPE_CMYK8_16
	TypeCMYK816SE       uint32 = 1443906  // TYPE_CMYK8_16_SE
	TypeKYMC88          uint32 = 1442881  // TYPE_KYMC8_8
	TypeKYMC816         uint32 = 1442882  // TYPE_KYMC8_16
	TypeKYMC816SE       uint32 = 1444930  // TYPE_KYMC8_16_SE
	TypeCMYK98          uint32 = 1507401  // TYPE_CMYK9_8
	TypeCMYK916         uint32 = 1507402  // TYPE_CMYK9_16
	TypeCMYK916SE       uint32 = 1509450  // TYPE_CMYK9_16_SE
	TypeKYMC98          uint32 = 1508425  // TYPE_KYMC9_8
	TypeKYMC916         uint32 = 1508426  // TYPE_KYMC9_16
	TypeKYMC916SE       uint32 = 1510474  // TYPE_KYMC9_16_SE
	TypeCMYK108         uint32 = 1572945  // TYPE_CMYK10_8
	TypeCMYK1016        uint32 = 1572946  // TYPE_CMYK10_16
	TypeCMYK1016SE      uint32 = 1574994  // TYPE_CMYK10_16_SE
	TypeKYMC108         uint32 = 1573969  // TYPE_KYMC10_8
	TypeKYMC1016        uint32 = 1573970  // TYPE_KYMC10_16
	TypeKYMC1016SE      uint32 = 1576018  // TYPE_KYMC10_16_SE
	TypeCMYK118         uint32 = 1638489  // TYPE_CMYK11_8
	TypeCMYK1116        uint32 = 1638490  // TYPE_CMYK11_16
	TypeCMYK1116SE      uint32 = 1640538  // TYPE_CMYK11_16_SE
	TypeKYMC118         uint32 = 1639513  // TYPE_KYMC11_8
	TypeKYMC1116        uint32 = 1639514  // TYPE_KYMC11_16
	TypeKYMC1116SE      uint32 = 1641562  // TYPE_KYMC11_16_SE
	TypeCMYK128         uint32 = 1704033  // TYPE_CMYK12_8
	TypeCMYK1216        uint32 = 1704034  // TYPE_CMYK12_16
	TypeCMYK1216SE      uint32 = 1706082  // TYPE_CMYK12_16_SE
	TypeKYMC128         uint32 = 1705057  // TYPE_KYMC12_8
	TypeKYMC1216        uint32 = 1705058  // TYPE_KYMC12_16
	TypeKYMC1216SE      uint32 = 1707106  // TYPE_KYMC12_16_SE
	TypeXYZ16           uint32 = 589850   // TYPE_XYZ_16
	TypeLab8            uint32 = 655385   // TYPE_Lab_8
	TypeLabV28          uint32 = 1966105  // TYPE_LabV2_8
	TypeALab8           uint32 = 671897   // TYPE_ALab_8
	TypeALabV28         uint32 = 1982617  // TYPE_ALabV2_8
	TypeLab16           uint32 = 655386   // TYPE_Lab_16
	TypeLabV216         uint32 = 1966106  // TYPE_LabV2_16
	TypeYxy16           uint32 = 917530   // TYPE_Yxy_16
	TypeYCbCr8          uint32 = 458777   // TYPE_YCbCr_8
	TypeYCbCr8Planar    uint32 = 462873   // TYPE_YCbCr_8_PLANAR
	TypeYCbCr16         uint32 = 458778   // TYPE_YCbCr_16
	TypeYCbCr16Planar   uint32 = 462874   // TYPE_YCbCr_16_PLANAR
	TypeYCbCr16SE       uint32 = 460826   // TYPE_YCbCr_16_SE
	TypeYUV8            uint32 = 524313   // TYPE_YUV_8
	TypeYUV8Planar      uint32 = 528409   // TYPE_YUV_8_PLANAR
	TypeYUV16           uint32 = 524314   // TYPE_YUV_16
	TypeYUV16Planar     uint32 = 528410   // TYPE_YUV_16_PLANAR
	TypeYUV16SE         uint32 = 526362   // TYPE_YUV_16_SE
	TypeHLS8            uint32 = 851993   // TYPE_HLS_8
	TypeHLS8Planar      uint32 = 856089   // TYPE_HLS_8_PLANAR
	TypeHLS16           uint32 = 851994   // TYPE_HLS_16
	TypeHLS16Planar     uint32 = 856090   // TYPE_HLS_16_PLANAR
	TypeHLS16SE         uint32 = 854042   // TYPE_HLS_16_SE
	TypeHSV8            uint32 = 786457   // TYPE_HSV_8
	TypeHSV8Planar      uint32 = 790553   // TYPE_HSV_8_PLANAR
	TypeHSV16           uint32 = 786458   // TYPE_HSV_16
	TypeHSV16Planar     uint32 = 790554   // TYPE_HSV_16_PLANAR
	TypeHSV16SE         uint32 = 788506   // TYPE_HSV_16_SE
	TypeNAMEDCOLORINDEX uint32 = 10       // TYPE_NAMED_COLOR_INDEX
	TypeXYZFlt          uint32 = 4784156  // TYPE_XYZ_FLT
	TypeLabFlt          uint32 = 4849692  // TYPE_Lab_FLT
	TypeLabAFlt         uint32 = 4849820  // TYPE_LabA_FLT
	TypeGrayFlt         uint32 = 4390924  // TYPE_GRAY_FLT
	TypeGRAYAFlt        uint32 = 4391052  // TYPE_GRAYA_FLT
	TypeGRAYAFltPremul  uint32 = 12779660 // TYPE_GRAYA_FLT_PREMUL
	TypeRGBFlt          uint32 = 4456476  // TYPE_RGB_FLT
	TypeRGBAFlt         uint32 = 4456604  // TYPE_RGBA_FLT
	TypeRGBAFltPremul   uint32 = 12845212 // TYPE_RGBA_FLT_PREMUL
	TypeARGBFlt         uint32 = 4472988  // TYPE_ARGB_FLT
	TypeARGBFltPremul   uint32 = 12861596 // TYPE_ARGB_FLT_PREMUL
	TypeBGRFlt          uint32 = 4457500  // TYPE_BGR_FLT
	TypeBGRAFlt         uint32 = 4474012  // TYPE_BGRA_FLT
	TypeBGRAFltPremul   uint32 = 12862620 // TYPE_BGRA_FLT_PREMUL
	TypeABGRFlt         uint32 = 4457628  // TYPE_ABGR_FLT
	TypeABGRFltPremul   uint32 = 12846236 // TYPE_ABGR_FLT_PREMUL
	TypeCMYKFlt         uint32 = 4587556  // TYPE_CMYK_FLT
	TypeXYZDbl          uint32 = 4784152  // TYPE_XYZ_DBL
	TypeLabDbl          uint32 = 4849688  // TYPE_Lab_DBL
	TypeGrayDbl         uint32 = 4390920  // TYPE_GRAY_DBL
	TypeRGBDbl          uint32 = 4456472  // TYPE_RGB_DBL
	TypeBGRDbl          uint32 = 4457496  // TYPE_BGR_DBL
	TypeCMYKDbl         uint32 = 4587552  // TYPE_CMYK_DBL
	TypeOKLABDbl        uint32 = 5308440  // TYPE_OKLAB_DBL
	TypeGrayHalfFlt     uint32 = 4390922  // TYPE_GRAY_HALF_FLT
	TypeRGBHalfFlt      uint32 = 4456474  // TYPE_RGB_HALF_FLT
	TypeCMYKHalfFlt     uint32 = 4587554  // TYPE_CMYK_HALF_FLT
	TypeRGBAHalfFlt     uint32 = 4456602  // TYPE_RGBA_HALF_FLT
	TypeARGBHalfFlt     uint32 = 4472986  // TYPE_ARGB_HALF_FLT
	TypeBGRHalfFlt      uint32 = 4457498  // TYPE_BGR_HALF_FLT
	TypeBGRAHalfFlt     uint32 = 4474010  // TYPE_BGRA_HALF_FLT
	TypeABGRHalfFlt     uint32 = 4457498  // TYPE_ABGR_HALF_FLT
)
