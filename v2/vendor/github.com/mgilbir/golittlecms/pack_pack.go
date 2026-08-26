package lcms2

// Packing routines ported from src/cmspack.c: internal channel array -> output
// pixel bytes. Each returns the number of bytes by which the output pointer
// advanced. See pack.go for shared conventions and bounds-checked accessors.

// ---- helpers for the (Extra == 0 && SwapFirst) output rotate --------------

// shiftBytesRight1 moves buf[0:n-1] to buf[1:n] (memmove(swap1+1, swap1, n-1)).
func shiftBytesRight1(buf []byte, n int) {
	if n < 1 || n > len(buf) {
		return
	}
	copy(buf[1:n], buf[0:n-1])
}

func shiftWordsRight1(buf []byte, n int) {
	if n < 1 || 2*n > len(buf) {
		return
	}
	copy(buf[2:2*n], buf[0:2*(n-1)])
}

func shiftF32Right1(buf []byte, n int) {
	if n < 1 || 4*n > len(buf) {
		return
	}
	copy(buf[4:4*n], buf[0:4*(n-1)])
}

func shiftF64Right1(buf []byte, n int) {
	if n < 1 || 8*n > len(buf) {
		return
	}
	copy(buf[8:8*n], buf[0:8*(n-1)])
}

// premulScale ports v = (cmsUInt16Number)((cmsUInt32Number)v * alpha_factor +
// 0x8000) >> 16, computed in uint32 (which wraps identically to C).
func premulScale(v uint16, alphaFactor uint32) uint16 {
	return uint16((uint32(v)*alphaFactor + 0x8000) >> 16)
}

// ---- generic 16-bit packers ------------------------------------------------

func packChunkyBytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	premul := tPremul(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	var v uint16
	var alphaFactor uint32

	if extraFirst {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(from8to16(rdU8(buf, 0)))))
		}
		pos += extra
	} else {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(from8to16(rdU8(buf, nChan)))))
		}
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = values[index]
		if reverse {
			v = reverseFlavor16(v)
		}
		if premul {
			v = premulScale(v, alphaFactor)
		}
		wrU8(buf, pos, from16to8(v))
		pos++
	}

	if !extraFirst {
		pos += extra
	}

	if extra == 0 && swapFirst {
		shiftBytesRight1(buf, nChan)
		wrU8(buf, 0, from16to8(v))
	}

	return pos
}

func packChunkyWords(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	swapEndian := tEndian16(f) != 0
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	premul := tPremul(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	var v uint16
	var alphaFactor uint32

	if extraFirst {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(rdU16(buf, 0))))
		}
		pos += extra * 2
	} else {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(rdU16(buf, nChan*2))))
		}
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = values[index]
		if swapEndian {
			v = changeEndian(v)
		}
		if reverse {
			v = reverseFlavor16(v)
		}
		if premul {
			v = premulScale(v, alphaFactor)
		}
		wrU16(buf, pos, v)
		pos += 2
	}

	if !extraFirst {
		pos += extra * 2
	}

	if extra == 0 && swapFirst {
		shiftWordsRight1(buf, nChan)
		wrU16(buf, 0, v)
	}

	return pos
}

func packPlanarBytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	swapFirst := tSwapFirst(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	premul := tPremul(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	var alphaFactor uint32

	if extraFirst {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(from8to16(rdU8(buf, 0)))))
		}
		pos += extra * stride
	} else {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(from8to16(rdU8(buf, nChan*stride)))))
		}
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := values[index]
		if reverse {
			v = reverseFlavor16(v)
		}
		if premul {
			v = premulScale(v, alphaFactor)
		}
		wrU8(buf, pos, from16to8(v))
		pos += stride
	}

	_ = swapFirst
	return 1
}

func packPlanarWords(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	swapFirst := tSwapFirst(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	premul := tPremul(f) != 0
	swapEndian := tEndian16(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	var alphaFactor uint32

	if extraFirst {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(rdU16(buf, 0))))
		}
		pos += extra * stride
	} else {
		if premul && extra != 0 {
			alphaFactor = uint32(toFixedDomain(int(rdU16(buf, nChan*stride))))
		}
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := values[index]
		if swapEndian {
			v = changeEndian(v)
		}
		if reverse {
			v = reverseFlavor16(v)
		}
		if premul {
			v = premulScale(v, alphaFactor)
		}
		wrU16(buf, pos, v)
		pos += stride
	}

	_ = swapFirst
	return 2
}

// ---- fixed-arity byte/word packers ----------------------------------------

func pack6Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[2]))
	wrU8(buf, 3, from16to8(values[3]))
	wrU8(buf, 4, from16to8(values[4]))
	wrU8(buf, 5, from16to8(values[5]))
	return 6
}

func pack6BytesSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[5]))
	wrU8(buf, 1, from16to8(values[4]))
	wrU8(buf, 2, from16to8(values[3]))
	wrU8(buf, 3, from16to8(values[2]))
	wrU8(buf, 4, from16to8(values[1]))
	wrU8(buf, 5, from16to8(values[0]))
	return 6
}

func pack6Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[2])
	wrU16(buf, 6, values[3])
	wrU16(buf, 8, values[4])
	wrU16(buf, 10, values[5])
	return 12
}

func pack6WordsSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[5])
	wrU16(buf, 2, values[4])
	wrU16(buf, 4, values[3])
	wrU16(buf, 6, values[2])
	wrU16(buf, 8, values[1])
	wrU16(buf, 10, values[0])
	return 12
}

func pack4Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[2]))
	wrU8(buf, 3, from16to8(values[3]))
	return 4
}

func pack4BytesReverse(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, reverseFlavor8(from16to8(values[0])))
	wrU8(buf, 1, reverseFlavor8(from16to8(values[1])))
	wrU8(buf, 2, reverseFlavor8(from16to8(values[2])))
	wrU8(buf, 3, reverseFlavor8(from16to8(values[3])))
	return 4
}

func pack4BytesSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[3]))
	wrU8(buf, 1, from16to8(values[0]))
	wrU8(buf, 2, from16to8(values[1]))
	wrU8(buf, 3, from16to8(values[2]))
	return 4
}

func pack4BytesSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[3]))
	wrU8(buf, 1, from16to8(values[2]))
	wrU8(buf, 2, from16to8(values[1]))
	wrU8(buf, 3, from16to8(values[0]))
	return 4
}

func pack4BytesSwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[2]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[0]))
	wrU8(buf, 3, from16to8(values[3]))
	return 4
}

func pack4Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[2])
	wrU16(buf, 6, values[3])
	return 8
}

func pack4WordsReverse(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, reverseFlavor16(values[0]))
	wrU16(buf, 2, reverseFlavor16(values[1]))
	wrU16(buf, 4, reverseFlavor16(values[2]))
	wrU16(buf, 6, reverseFlavor16(values[3]))
	return 8
}

func pack4WordsSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[3])
	wrU16(buf, 2, values[2])
	wrU16(buf, 4, values[1])
	wrU16(buf, 6, values[0])
	return 8
}

func pack4WordsBigEndian(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, changeEndian(values[0]))
	wrU16(buf, 2, changeEndian(values[1]))
	wrU16(buf, 4, changeEndian(values[2]))
	wrU16(buf, 6, changeEndian(values[3]))
	return 8
}

func packLabV2_8(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(fomLabV4ToLabV2(values[0])))
	wrU8(buf, 1, from16to8(fomLabV4ToLabV2(values[1])))
	wrU8(buf, 2, from16to8(fomLabV4ToLabV2(values[2])))
	return 3
}

func packALabV2_8(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// output++ (A)
	wrU8(buf, 1, from16to8(fomLabV4ToLabV2(values[0])))
	wrU8(buf, 2, from16to8(fomLabV4ToLabV2(values[1])))
	wrU8(buf, 3, from16to8(fomLabV4ToLabV2(values[2])))
	return 4
}

func packLabV2_16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, fomLabV4ToLabV2(values[0]))
	wrU16(buf, 2, fomLabV4ToLabV2(values[1]))
	wrU16(buf, 4, fomLabV4ToLabV2(values[2]))
	return 6
}

func pack3Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[2]))
	return 3
}

func pack3BytesOptimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, uint8(values[0]&0xFF))
	wrU8(buf, 1, uint8(values[1]&0xFF))
	wrU8(buf, 2, uint8(values[2]&0xFF))
	return 3
}

func pack3BytesSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[2]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[0]))
	return 3
}

func pack3BytesSwapOptimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, uint8(values[2]&0xFF))
	wrU8(buf, 1, uint8(values[1]&0xFF))
	wrU8(buf, 2, uint8(values[0]&0xFF))
	return 3
}

func pack3Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[2])
	return 6
}

func pack3WordsSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[2])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[0])
	return 6
}

func pack3WordsBigEndian(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, changeEndian(values[0]))
	wrU16(buf, 2, changeEndian(values[1]))
	wrU16(buf, 4, changeEndian(values[2]))
	return 6
}

func pack3BytesAndSkip1(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[2]))
	return 4
}

func pack3BytesAndSkip1Optimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, uint8(values[0]&0xFF))
	wrU8(buf, 1, uint8(values[1]&0xFF))
	wrU8(buf, 2, uint8(values[2]&0xFF))
	return 4
}

func pack3BytesAndSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// output++ then 3 bytes
	wrU8(buf, 1, from16to8(values[0]))
	wrU8(buf, 2, from16to8(values[1]))
	wrU8(buf, 3, from16to8(values[2]))
	return 4
}

func pack3BytesAndSkip1SwapFirstOptimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 1, uint8(values[0]&0xFF))
	wrU8(buf, 2, uint8(values[1]&0xFF))
	wrU8(buf, 3, uint8(values[2]&0xFF))
	return 4
}

func pack3BytesAndSkip1Swap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// output++ then B,G,R
	wrU8(buf, 1, from16to8(values[2]))
	wrU8(buf, 2, from16to8(values[1]))
	wrU8(buf, 3, from16to8(values[0]))
	return 4
}

func pack3BytesAndSkip1SwapOptimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 1, uint8(values[2]&0xFF))
	wrU8(buf, 2, uint8(values[1]&0xFF))
	wrU8(buf, 3, uint8(values[0]&0xFF))
	return 4
}

func pack3BytesAndSkip1SwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[2]))
	wrU8(buf, 1, from16to8(values[1]))
	wrU8(buf, 2, from16to8(values[0]))
	return 4
}

func pack3BytesAndSkip1SwapSwapFirstOptimized(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, uint8(values[2]&0xFF))
	wrU8(buf, 1, uint8(values[1]&0xFF))
	wrU8(buf, 2, uint8(values[0]&0xFF))
	return 4
}

func pack3WordsAndSkip1(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[2])
	return 8
}

func pack3WordsAndSkip1Swap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// output += 2 then B,G,R
	wrU16(buf, 2, values[2])
	wrU16(buf, 4, values[1])
	wrU16(buf, 6, values[0])
	return 8
}

func pack3WordsAndSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// output += 2 then R,G,B
	wrU16(buf, 2, values[0])
	wrU16(buf, 4, values[1])
	wrU16(buf, 6, values[2])
	return 8
}

func pack3WordsAndSkip1SwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[2])
	wrU16(buf, 2, values[1])
	wrU16(buf, 4, values[0])
	return 8
}

func pack1Byte(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	return 1
}

func pack1ByteReversed(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(reverseFlavor16(values[0])))
	return 1
}

func pack1ByteSkip1(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 0, from16to8(values[0]))
	return 2
}

func pack1ByteSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU8(buf, 1, from16to8(values[0]))
	return 2
}

func pack1Word(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	return 2
}

func pack1WordReversed(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, reverseFlavor16(values[0]))
	return 2
}

func pack1WordBigEndian(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, changeEndian(values[0]))
	return 2
}

func pack1WordSkip1(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 0, values[0])
	return 4
}

func pack1WordSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	wrU16(buf, 2, values[0])
	return 4
}

// ---- Lab/XYZ float encodings from the 16-bit engine -----------------------

func packLabDoubleFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	lab := LabEncoded2Float([3]uint16{values[0], values[1], values[2]})
	if tPlanar(info.OutputFormat) != 0 {
		// NOTE: verbatim quirk — no Stride /= PixelSize here (unlike XYZ), so
		// the element index is the raw byte stride.
		wrF64(buf, 0, lab.L)
		wrF64(buf, stride*8, lab.A)
		wrF64(buf, stride*2*8, lab.B)
		return 8
	}
	wrF64(buf, 0, lab.L)
	wrF64(buf, 8, lab.A)
	wrF64(buf, 16, lab.B)
	return 24 + int(tExtra(info.OutputFormat))*8
}

func packLabFloatFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	lab := LabEncoded2Float([3]uint16{values[0], values[1], values[2]})
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF32(buf, 0, float32(lab.L))
		wrF32(buf, str*4, float32(lab.A))
		wrF32(buf, str*2*4, float32(lab.B))
		return 4
	}
	wrF32(buf, 0, float32(lab.L))
	wrF32(buf, 4, float32(lab.A))
	wrF32(buf, 8, float32(lab.B))
	return (3 + int(tExtra(info.OutputFormat))) * 4
}

func packXYZDoubleFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	xyz := XYZEncoded2Float([3]uint16{values[0], values[1], values[2]})
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF64(buf, 0, xyz.X)
		wrF64(buf, str*8, xyz.Y)
		wrF64(buf, str*2*8, xyz.Z)
		return 8
	}
	wrF64(buf, 0, xyz.X)
	wrF64(buf, 8, xyz.Y)
	wrF64(buf, 16, xyz.Z)
	return 24 + int(tExtra(info.OutputFormat))*8
}

func packXYZFloatFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	xyz := XYZEncoded2Float([3]uint16{values[0], values[1], values[2]})
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF32(buf, 0, float32(xyz.X))
		wrF32(buf, str*4, float32(xyz.Y))
		wrF32(buf, str*2*4, float32(xyz.Z))
		return 4
	}
	wrF32(buf, 0, float32(xyz.X))
	wrF32(buf, 4, float32(xyz.Y))
	wrF32(buf, 8, float32(xyz.Z))
	return 3*4 + int(tExtra(info.OutputFormat))*4
}

func packDoubleFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := 65535.0
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float64
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = float64(values[index]) / maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrF64(buf, (i+start)*str*8, v)
		} else {
			wrF64(buf, (i+start)*8, v)
		}
	}

	if extra == 0 && swapFirst {
		shiftF64Right1(buf, nChan)
		wrF64(buf, 0, v)
	}

	if planar {
		return 8
	}
	return (nChan + extra) * 8
}

func packFloatFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := 65535.0
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float64
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = float64(values[index]) / maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrF32(buf, (i+start)*str*4, float32(v))
		} else {
			wrF32(buf, (i+start)*4, float32(v))
		}
	}

	if extra == 0 && swapFirst {
		shiftF32Right1(buf, nChan)
		wrF32(buf, 0, float32(v))
	}

	if planar {
		return 4
	}
	return (nChan + extra) * 4
}

// ---- float engine packers --------------------------------------------------

func packBytesFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	start := 0
	if extraFirst {
		start = extra
	}

	var vv uint8
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := float64(values[index]) * 65535.0
		if reverse {
			v = 65535.0 - v
		}
		vv = from16to8(quickSaturateWord(v))
		if planar {
			wrU8(buf, (i+start)*stride, vv)
		} else {
			wrU8(buf, i+start, vv)
		}
	}

	if extra == 0 && swapFirst {
		shiftBytesRight1(buf, nChan)
		wrU8(buf, 0, vv)
	}

	if planar {
		return 1
	}
	return nChan + extra
}

func packWordsFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	start := 0
	if extraFirst {
		start = extra
	}

	str := stride / 2

	var vv uint16
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := float64(values[index]) * 65535.0
		if reverse {
			v = 65535.0 - v
		}
		vv = quickSaturateWord(v)
		if planar {
			wrU16(buf, (i+start)*str*2, vv)
		} else {
			wrU16(buf, (i+start)*2, vv)
		}
	}

	if extra == 0 && swapFirst {
		shiftWordsRight1(buf, nChan)
		wrU16(buf, 0, vv)
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}

func packFloatsFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := 1.0
	if isInkSpace(f) {
		maximum = 100.0
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float64
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = float64(values[index]) * maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrF32(buf, (i+start)*str*4, float32(v))
		} else {
			wrF32(buf, (i+start)*4, float32(v))
		}
	}

	if extra == 0 && swapFirst {
		shiftF32Right1(buf, nChan)
		wrF32(buf, 0, float32(v))
	}

	if planar {
		return 4
	}
	return (nChan + extra) * 4
}

func packDoublesFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := 1.0
	if isInkSpace(f) {
		maximum = 100.0
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float64
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = float64(values[index]) * maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrF64(buf, (i+start)*str*8, v)
		} else {
			wrF64(buf, (i+start)*8, v)
		}
	}

	if extra == 0 && swapFirst {
		shiftF64Right1(buf, nChan)
		wrF64(buf, 0, v)
	}

	if planar {
		return 8
	}
	return (nChan + extra) * 8
}

func packLabFloatFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF32(buf, 0, float32(float64(values[0])*100.0))
		wrF32(buf, str*4, float32(float64(values[1])*255.0-128.0))
		wrF32(buf, str*2*4, float32(float64(values[2])*255.0-128.0))
		return 4
	}
	wrF32(buf, 0, float32(float64(values[0])*100.0))
	wrF32(buf, 4, float32(float64(values[1])*255.0-128.0))
	wrF32(buf, 8, float32(float64(values[2])*255.0-128.0))
	return 4*3 + int(tExtra(info.OutputFormat))*4
}

func packLabDoubleFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF64(buf, 0, float64(values[0])*100.0)
		wrF64(buf, str*8, float64(values[1])*255.0-128.0)
		wrF64(buf, str*2*8, float64(values[2])*255.0-128.0)
		return 8
	}
	wrF64(buf, 0, float64(values[0])*100.0)
	wrF64(buf, 8, float64(values[1])*255.0-128.0)
	wrF64(buf, 16, float64(values[2])*255.0-128.0)
	return 8*3 + int(tExtra(info.OutputFormat))*8
}

func packEncodedBytesLabV2FromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	lab := CIELab{
		L: float64(values[0]) * 100.0,
		A: float64(values[1])*255.0 - 128.0,
		B: float64(values[2])*255.0 - 128.0,
	}
	wlab := Float2LabEncoded(lab)
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrU8(buf, 0, uint8(wlab[0]>>8))
		wrU8(buf, str, uint8(wlab[1]>>8))
		wrU8(buf, str*2, uint8(wlab[2]>>8))
		return 1
	}
	wrU8(buf, 0, uint8(wlab[0]>>8))
	wrU8(buf, 1, uint8(wlab[1]>>8))
	wrU8(buf, 2, uint8(wlab[2]>>8))
	return 3 + int(tExtra(info.OutputFormat))
}

func packEncodedWordsLabV2FromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	lab := CIELab{
		L: float64(values[0]) * 100.0,
		A: float64(values[1])*255.0 - 128.0,
		B: float64(values[2])*255.0 - 128.0,
	}
	wlab := Float2LabEncodedV2(lab)
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrU16(buf, 0, wlab[0])
		wrU16(buf, str*2, wlab[1])
		wrU16(buf, str*2*2, wlab[2])
		return 2
	}
	wrU16(buf, 0, wlab[0])
	wrU16(buf, 2, wlab[1])
	wrU16(buf, 4, wlab[2])
	return (3 + int(tExtra(info.OutputFormat))) * 2
}

func packXYZFloatFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF32(buf, 0, float32(float64(values[0])*maxEncodeableXYZ))
		wrF32(buf, str*4, float32(float64(values[1])*maxEncodeableXYZ))
		wrF32(buf, str*2*4, float32(float64(values[2])*maxEncodeableXYZ))
		return 4
	}
	wrF32(buf, 0, float32(float64(values[0])*maxEncodeableXYZ))
	wrF32(buf, 4, float32(float64(values[1])*maxEncodeableXYZ))
	wrF32(buf, 8, float32(float64(values[2])*maxEncodeableXYZ))
	return 4*3 + int(tExtra(info.OutputFormat))*4
}

func packXYZDoubleFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.OutputFormat) != 0 {
		str := stride / pixelSize(info.OutputFormat)
		wrF64(buf, 0, float64(values[0])*maxEncodeableXYZ)
		wrF64(buf, str*8, float64(values[1])*maxEncodeableXYZ)
		wrF64(buf, str*2*8, float64(values[2])*maxEncodeableXYZ)
		return 8
	}
	wrF64(buf, 0, float64(values[0])*maxEncodeableXYZ)
	wrF64(buf, 8, float64(values[1])*maxEncodeableXYZ)
	wrF64(buf, 16, float64(values[2])*maxEncodeableXYZ)
	return 8*3 + int(tExtra(info.OutputFormat))*8
}

// ---- half float -----------------------------------------------------------

func packHalfFrom16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := float32(65535.0)
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float32
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = float32(values[index]) / maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrU16(buf, (i+start)*str*2, float2Half(v))
		} else {
			wrU16(buf, (i+start)*2, float2Half(v))
		}
	}

	if extra == 0 && swapFirst {
		shiftWordsRight1(buf, nChan)
		wrU16(buf, 0, float2Half(v))
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}

func packHalfFromFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.OutputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	swapFirst := tSwapFirst(f) != 0
	planar := tPlanar(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	maximum := float32(1.0)
	if isInkSpace(f) {
		maximum = 100.0
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	var v float32
	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v = values[index] * maximum
		if reverse {
			v = maximum - v
		}
		if planar {
			wrU16(buf, (i+start)*str*2, float2Half(v))
		} else {
			wrU16(buf, (i+start)*2, float2Half(v))
		}
	}

	if extra == 0 && swapFirst {
		shiftWordsRight1(buf, nChan)
		wrU16(buf, 0, float2Half(v))
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}
