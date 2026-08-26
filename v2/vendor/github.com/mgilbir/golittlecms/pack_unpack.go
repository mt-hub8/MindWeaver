package lcms2

// Unpacking routines ported from src/cmspack.c: input pixel bytes -> internal
// channel array (16-bit or float32). Each returns the number of bytes by which
// the accumulator advanced. See pack.go for the shared conventions and the
// bounds-checked buffer accessors.

// rotateWordsLeft ports the (Extra == 0 && SwapFirst) tail rotate on the 16-bit
// working array: tmp = wIn[0]; move wIn[1..] down; wIn[nChan-1] = tmp.
func rotateWordsLeft(values []uint16, nChan int) {
	if nChan < 1 || nChan > len(values) {
		return
	}
	tmp := values[0]
	copy(values[0:nChan-1], values[1:nChan])
	values[nChan-1] = tmp
}

func rotateFloatsLeft(values []float32, nChan int) {
	if nChan < 1 || nChan > len(values) {
		return
	}
	tmp := values[0]
	copy(values[0:nChan-1], values[1:nChan])
	values[nChan-1] = tmp
}

// UnrollChunkyBytes: the generic 8-bit chunky unpacker.
func unrollChunkyBytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	premul := tPremul(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	alphaFactor := uint32(1)

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
		v := uint32(from8to16(rdU8(buf, pos)))
		if reverse {
			v = uint32(reverseFlavor16(uint16(v)))
		}
		if premul && alphaFactor > 0 {
			v = (v << 16) / alphaFactor
			if v > 0xffff {
				v = 0xffff
			}
		}
		values[index] = uint16(v)
		pos++
	}

	if !extraFirst {
		pos += extra
	}

	if extra == 0 && swapFirst {
		rotateWordsLeft(values, nChan)
	}

	return pos
}

// UnrollPlanarBytes: extra channels ignored (they come in the next planes).
func unrollPlanarBytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	extra := int(tExtra(f))
	premul := tPremul(f) != 0
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	alphaFactor := uint32(1)

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
		v := uint32(from8to16(rdU8(buf, pos)))
		if reverse {
			v = uint32(reverseFlavor16(uint16(v)))
		}
		if premul && alphaFactor > 0 {
			v = (v << 16) / alphaFactor
			if v > 0xffff {
				v = 0xffff
			}
		}
		values[index] = uint16(v)
		pos += stride
	}

	return 1
}

func unroll4Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = from8to16(rdU8(buf, 0))
	values[1] = from8to16(rdU8(buf, 1))
	values[2] = from8to16(rdU8(buf, 2))
	values[3] = from8to16(rdU8(buf, 3))
	return 4
}

func unroll4BytesReverse(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = from8to16(reverseFlavor8(rdU8(buf, 0)))
	values[1] = from8to16(reverseFlavor8(rdU8(buf, 1)))
	values[2] = from8to16(reverseFlavor8(rdU8(buf, 2)))
	values[3] = from8to16(reverseFlavor8(rdU8(buf, 3)))
	return 4
}

func unroll4BytesSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[3] = from8to16(rdU8(buf, 0)) // K
	values[0] = from8to16(rdU8(buf, 1)) // C
	values[1] = from8to16(rdU8(buf, 2)) // M
	values[2] = from8to16(rdU8(buf, 3)) // Y
	return 4
}

func unroll4BytesSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[3] = from8to16(rdU8(buf, 0)) // K
	values[2] = from8to16(rdU8(buf, 1)) // Y
	values[1] = from8to16(rdU8(buf, 2)) // M
	values[0] = from8to16(rdU8(buf, 3)) // C
	return 4
}

func unroll4BytesSwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[2] = from8to16(rdU8(buf, 0)) // K
	values[1] = from8to16(rdU8(buf, 1)) // Y
	values[0] = from8to16(rdU8(buf, 2)) // M
	values[3] = from8to16(rdU8(buf, 3)) // C
	return 4
}

func unroll3Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = from8to16(rdU8(buf, 0)) // R
	values[1] = from8to16(rdU8(buf, 1)) // G
	values[2] = from8to16(rdU8(buf, 2)) // B
	return 3
}

func unroll3BytesSkip1Swap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// accum++ (A) then B,G,R
	values[2] = from8to16(rdU8(buf, 1)) // B
	values[1] = from8to16(rdU8(buf, 2)) // G
	values[0] = from8to16(rdU8(buf, 3)) // R
	return 4
}

func unroll3BytesSkip1SwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[2] = from8to16(rdU8(buf, 0)) // B
	values[1] = from8to16(rdU8(buf, 1)) // G
	values[0] = from8to16(rdU8(buf, 2)) // R
	// accum++ (A)
	return 4
}

func unroll3BytesSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// accum++ (A) then R,G,B
	values[0] = from8to16(rdU8(buf, 1)) // R
	values[1] = from8to16(rdU8(buf, 2)) // G
	values[2] = from8to16(rdU8(buf, 3)) // B
	return 4
}

func unroll3BytesSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[2] = from8to16(rdU8(buf, 0)) // B
	values[1] = from8to16(rdU8(buf, 1)) // G
	values[0] = from8to16(rdU8(buf, 2)) // R
	return 3
}

func unrollLabV2_8(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = fomLabV2ToLabV4(from8to16(rdU8(buf, 0)))
	values[1] = fomLabV2ToLabV4(from8to16(rdU8(buf, 1)))
	values[2] = fomLabV2ToLabV4(from8to16(rdU8(buf, 2)))
	return 3
}

func unrollALabV2_8(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// accum++ (A)
	values[0] = fomLabV2ToLabV4(from8to16(rdU8(buf, 1)))
	values[1] = fomLabV2ToLabV4(from8to16(rdU8(buf, 2)))
	values[2] = fomLabV2ToLabV4(from8to16(rdU8(buf, 3)))
	return 4
}

func unrollLabV2_16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = fomLabV2ToLabV4(rdU16(buf, 0))
	values[1] = fomLabV2ToLabV4(rdU16(buf, 2))
	values[2] = fomLabV2ToLabV4(rdU16(buf, 4))
	return 6
}

func unroll2Bytes(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = from8to16(rdU8(buf, 0)) // ch1
	values[1] = from8to16(rdU8(buf, 1)) // ch2
	return 2
}

func unroll1Byte(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := from8to16(rdU8(buf, 0))
	values[0], values[1], values[2] = v, v, v
	return 1
}

func unroll1ByteSkip1(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := from8to16(rdU8(buf, 0))
	values[0], values[1], values[2] = v, v, v
	return 2
}

func unroll1ByteSkip2(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := from8to16(rdU8(buf, 0))
	values[0], values[1], values[2] = v, v, v
	return 3
}

func unroll1ByteReversed(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := reverseFlavor16(from8to16(rdU8(buf, 0)))
	values[0], values[1], values[2] = v, v, v
	return 1
}

func unrollAnyWords(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	swapEndian := tEndian16(f) != 0
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	pos := 0
	if extraFirst {
		pos += extra * 2
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := rdU16(buf, pos)
		if swapEndian {
			v = changeEndian(v)
		}
		if reverse {
			v = reverseFlavor16(v)
		}
		values[index] = v
		pos += 2
	}

	if !extraFirst {
		pos += extra * 2
	}

	if extra == 0 && swapFirst {
		rotateWordsLeft(values, nChan)
	}

	return pos
}

func unrollAnyWordsPremul(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	swapEndian := tEndian16(f) != 0
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	alphaIdx := nChan
	if extraFirst {
		alphaIdx = 0
	}
	alpha := rdU16(buf, alphaIdx*2)
	alphaFactor := uint32(toFixedDomain(int(alpha)))

	pos := 0
	if extraFirst {
		pos += extra * 2
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := uint32(rdU16(buf, pos))
		if swapEndian {
			v = uint32(changeEndian(uint16(v)))
		}
		if alphaFactor > 0 {
			v = (v << 16) / alphaFactor
			if v > 0xffff {
				v = 0xffff
			}
		}
		if reverse {
			v = uint32(reverseFlavor16(uint16(v)))
		}
		values[index] = uint16(v)
		pos += 2
	}

	if !extraFirst {
		pos += extra * 2
	}

	_ = swapFirst // no SwapFirst rotate in the C premul word path
	return pos
}

func unrollPlanarWords(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapEndian := tEndian16(f) != 0

	pos := 0
	if doSwap {
		pos += int(tExtra(f)) * stride
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := rdU16(buf, pos)
		if swapEndian {
			v = changeEndian(v)
		}
		if reverse {
			v = reverseFlavor16(v)
		}
		values[index] = v
		pos += stride
	}

	return 2
}

func unrollPlanarWordsPremul(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	swapFirst := tSwapFirst(f) != 0
	reverse := tFlavor(f) != 0
	swapEndian := tEndian16(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0

	// alpha = ExtraFirst ? word[0] : word[nChan*Stride/2]  (byte offsets)
	alphaByte := nChan * stride
	if extraFirst {
		alphaByte = 0
	}
	alpha := rdU16(buf, alphaByte)
	alphaFactor := uint32(toFixedDomain(int(alpha)))

	pos := 0
	if extraFirst {
		pos += extra * stride
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		v := uint32(rdU16(buf, pos))
		if swapEndian {
			v = uint32(changeEndian(uint16(v)))
		}
		if alphaFactor > 0 {
			v = (v << 16) / alphaFactor
			if v > 0xffff {
				v = 0xffff
			}
		}
		if reverse {
			v = uint32(reverseFlavor16(uint16(v)))
		}
		values[index] = uint16(v)
		pos += stride
	}

	_ = swapFirst
	return 2
}

func unroll4Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = rdU16(buf, 0)
	values[1] = rdU16(buf, 2)
	values[2] = rdU16(buf, 4)
	values[3] = rdU16(buf, 6)
	return 8
}

func unroll4WordsReverse(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = reverseFlavor16(rdU16(buf, 0))
	values[1] = reverseFlavor16(rdU16(buf, 2))
	values[2] = reverseFlavor16(rdU16(buf, 4))
	values[3] = reverseFlavor16(rdU16(buf, 6))
	return 8
}

func unroll4WordsSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[3] = rdU16(buf, 0) // K
	values[0] = rdU16(buf, 2) // C
	values[1] = rdU16(buf, 4) // M
	values[2] = rdU16(buf, 6) // Y
	return 8
}

func unroll4WordsSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[3] = rdU16(buf, 0) // K
	values[2] = rdU16(buf, 2) // Y
	values[1] = rdU16(buf, 4) // M
	values[0] = rdU16(buf, 6) // C
	return 8
}

func unroll4WordsSwapSwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[2] = rdU16(buf, 0) // K
	values[1] = rdU16(buf, 2) // Y
	values[0] = rdU16(buf, 4) // M
	values[3] = rdU16(buf, 6) // C
	return 8
}

func unroll3Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = rdU16(buf, 0)
	values[1] = rdU16(buf, 2)
	values[2] = rdU16(buf, 4)
	return 6
}

func unroll3WordsSwap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[2] = rdU16(buf, 0)
	values[1] = rdU16(buf, 2)
	values[0] = rdU16(buf, 4)
	return 6
}

func unroll3WordsSkip1Swap(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// accum += 2 (A) then R,G,B into wIn[2],[1],[0]
	values[2] = rdU16(buf, 2) // R
	values[1] = rdU16(buf, 4) // G
	values[0] = rdU16(buf, 6) // B
	return 8
}

func unroll3WordsSkip1SwapFirst(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	// accum += 2 (A) then R,G,B into wIn[0],[1],[2]
	values[0] = rdU16(buf, 2) // R
	values[1] = rdU16(buf, 4) // G
	values[2] = rdU16(buf, 6) // B
	return 8
}

func unroll1Word(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := rdU16(buf, 0)
	values[0], values[1], values[2] = v, v, v
	return 2
}

func unroll1WordReversed(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := reverseFlavor16(rdU16(buf, 0))
	values[0], values[1], values[2] = v, v, v
	return 2
}

func unroll1WordSkip3(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := rdU16(buf, 0)
	values[0], values[1], values[2] = v, v, v
	return 8
}

func unroll2Words(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	values[0] = rdU16(buf, 0) // ch1
	values[1] = rdU16(buf, 2) // ch2
	return 4
}

// UnrollLabDoubleTo16.
func unrollLabDoubleTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		lab := CIELab{
			L: rdF64(buf, 0),
			A: rdF64(buf, stride),
			B: rdF64(buf, stride*2),
		}
		enc := Float2LabEncoded(lab)
		values[0], values[1], values[2] = enc[0], enc[1], enc[2]
		return 8
	}
	lab := CIELab{L: rdF64(buf, 0), A: rdF64(buf, 8), B: rdF64(buf, 16)}
	enc := Float2LabEncoded(lab)
	values[0], values[1], values[2] = enc[0], enc[1], enc[2]
	return 24 + int(tExtra(info.InputFormat))*8
}

func unrollLabFloatTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		lab := CIELab{
			L: float64(rdF32(buf, 0)),
			A: float64(rdF32(buf, stride)),
			B: float64(rdF32(buf, stride*2)),
		}
		enc := Float2LabEncoded(lab)
		values[0], values[1], values[2] = enc[0], enc[1], enc[2]
		return 4
	}
	lab := CIELab{
		L: float64(rdF32(buf, 0)),
		A: float64(rdF32(buf, 4)),
		B: float64(rdF32(buf, 8)),
	}
	enc := Float2LabEncoded(lab)
	values[0], values[1], values[2] = enc[0], enc[1], enc[2]
	return (3 + int(tExtra(info.InputFormat))) * 4
}

func unrollXYZDoubleTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		xyz := CIEXYZ{
			X: rdF64(buf, 0),
			Y: rdF64(buf, stride),
			Z: rdF64(buf, stride*2),
		}
		enc := Float2XYZEncoded(xyz)
		values[0], values[1], values[2] = enc[0], enc[1], enc[2]
		return 8
	}
	xyz := CIEXYZ{X: rdF64(buf, 0), Y: rdF64(buf, 8), Z: rdF64(buf, 16)}
	enc := Float2XYZEncoded(xyz)
	values[0], values[1], values[2] = enc[0], enc[1], enc[2]
	return 24 + int(tExtra(info.InputFormat))*8
}

func unrollXYZFloatTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		xyz := CIEXYZ{
			X: float64(rdF32(buf, 0)),
			Y: float64(rdF32(buf, stride)),
			Z: float64(rdF32(buf, stride*2)),
		}
		enc := Float2XYZEncoded(xyz)
		values[0], values[1], values[2] = enc[0], enc[1], enc[2]
		return 4
	}
	xyz := CIEXYZ{
		X: float64(rdF32(buf, 0)),
		Y: float64(rdF32(buf, 4)),
		Z: float64(rdF32(buf, 8)),
	}
	enc := Float2XYZEncoded(xyz)
	values[0], values[1], values[2] = enc[0], enc[1], enc[2]
	return 3*4 + int(tExtra(info.InputFormat))*4
}

func unrollDoubleTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	maximum := 65535.0
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(f) // element stride

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float64
		if planar {
			v = float64(float32(rdF64(buf, (i+start)*str*8)))
		} else {
			v = float64(float32(rdF64(buf, (i+start)*8)))
		}
		vi := quickSaturateWord(v * maximum)
		if reverse {
			vi = reverseFlavor16(vi)
		}
		values[index] = vi
	}

	if extra == 0 && swapFirst {
		rotateWordsLeft(values, nChan)
	}

	if planar {
		return 8
	}
	return (nChan + extra) * 8
}

func unrollFloatTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	maximum := 65535.0
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = rdF32(buf, (i+start)*str*4)
		} else {
			v = rdF32(buf, (i+start)*4)
		}
		vi := quickSaturateWord(float64(v) * maximum)
		if reverse {
			vi = reverseFlavor16(vi)
		}
		values[index] = vi
	}

	if extra == 0 && swapFirst {
		rotateWordsLeft(values, nChan)
	}

	if planar {
		return 4
	}
	return (nChan + extra) * 4
}

func unrollDouble1Chan(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	v := quickSaturateWord(rdF64(buf, 0) * 65535.0)
	values[0], values[1], values[2] = v, v, v
	return 8
}

// ---- float working-array unpackers ---------------------------------------

func unroll8ToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = float32(rdU8(buf, (i+start)*str))
		} else {
			v = float32(rdU8(buf, i+start))
		}
		v /= 255.0
		if reverse {
			v = 1 - v
		}
		values[index] = v
	}

	if extra == 0 && swapFirst {
		rotateFloatsLeft(values, nChan)
	}

	if planar {
		return 1
	}
	return nChan + extra
}

func unroll16ToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = float32(rdU16(buf, (i+start)*str*2))
		} else {
			v = float32(rdU16(buf, (i+start)*2))
		}
		v /= 65535.0
		if reverse {
			v = 1 - v
		}
		values[index] = v
	}

	if extra == 0 && swapFirst {
		rotateFloatsLeft(values, nChan)
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}

func unrollFloatsToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0
	premul := tPremul(f) != 0

	maximum := float32(1.0)
	if isInkSpace(f) {
		maximum = 100.0
	}
	alphaFactor := float32(1.0)

	str := stride / pixelSize(f)

	if premul && extra != 0 {
		if planar {
			if extraFirst {
				alphaFactor = rdF32(buf, 0) / maximum
			} else {
				alphaFactor = rdF32(buf, nChan*str*4) / maximum
			}
		} else {
			if extraFirst {
				alphaFactor = rdF32(buf, 0) / maximum
			} else {
				alphaFactor = rdF32(buf, nChan*4) / maximum
			}
		}
	}

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = rdF32(buf, (i+start)*str*4)
		} else {
			v = rdF32(buf, (i+start)*4)
		}
		if premul && alphaFactor > 0 {
			v /= alphaFactor
		}
		v /= maximum
		if reverse {
			v = 1 - v
		}
		values[index] = v
	}

	if extra == 0 && swapFirst {
		rotateFloatsLeft(values, nChan)
	}

	if planar {
		return 4
	}
	return (nChan + extra) * 4
}

func unrollDoublesToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0
	premul := tPremul(f) != 0

	maximum := 1.0
	if isInkSpace(f) {
		maximum = 100.0
	}
	alphaFactor := 1.0

	str := stride / pixelSize(f)

	if premul && extra != 0 {
		if planar {
			if extraFirst {
				alphaFactor = rdF64(buf, 0) / maximum
			} else {
				alphaFactor = rdF64(buf, nChan*str*8) / maximum
			}
		} else {
			if extraFirst {
				alphaFactor = rdF64(buf, 0) / maximum
			} else {
				alphaFactor = rdF64(buf, nChan*8) / maximum
			}
		}
	}

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float64
		if planar {
			v = rdF64(buf, (i+start)*str*8)
		} else {
			v = rdF64(buf, (i+start)*8)
		}
		if premul && alphaFactor > 0 {
			v /= alphaFactor
		}
		v /= maximum
		if reverse {
			values[index] = float32(1.0 - v)
		} else {
			values[index] = float32(v)
		}
	}

	if extra == 0 && swapFirst {
		rotateFloatsLeft(values, nChan)
	}

	if planar {
		return 8
	}
	return (nChan + extra) * 8
}

func unrollLabDoubleToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		str := stride / pixelSize(info.InputFormat)
		values[0] = float32(rdF64(buf, 0) / 100.0)
		values[1] = float32((rdF64(buf, str*8) + 128) / 255.0)
		values[2] = float32((rdF64(buf, str*2*8) + 128) / 255.0)
		return 8
	}
	values[0] = float32(rdF64(buf, 0) / 100.0)
	values[1] = float32((rdF64(buf, 8) + 128) / 255.0)
	values[2] = float32((rdF64(buf, 16) + 128) / 255.0)
	return 8 * (3 + int(tExtra(info.InputFormat)))
}

func unrollLabFloatToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		str := stride / pixelSize(info.InputFormat)
		values[0] = float32(float64(rdF32(buf, 0)) / 100.0)
		values[1] = float32((float64(rdF32(buf, str*4)) + 128) / 255.0)
		values[2] = float32((float64(rdF32(buf, str*2*4)) + 128) / 255.0)
		return 4
	}
	values[0] = float32(float64(rdF32(buf, 0)) / 100.0)
	values[1] = float32((float64(rdF32(buf, 4)) + 128) / 255.0)
	values[2] = float32((float64(rdF32(buf, 8)) + 128) / 255.0)
	return 4 * (3 + int(tExtra(info.InputFormat)))
}

func unrollXYZDoubleToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		str := stride / pixelSize(info.InputFormat)
		values[0] = float32(rdF64(buf, 0) / maxEncodeableXYZ)
		values[1] = float32(rdF64(buf, str*8) / maxEncodeableXYZ)
		values[2] = float32(rdF64(buf, str*2*8) / maxEncodeableXYZ)
		return 8
	}
	values[0] = float32(rdF64(buf, 0) / maxEncodeableXYZ)
	values[1] = float32(rdF64(buf, 8) / maxEncodeableXYZ)
	values[2] = float32(rdF64(buf, 16) / maxEncodeableXYZ)
	return 8 * (3 + int(tExtra(info.InputFormat)))
}

func unrollXYZFloatToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	if tPlanar(info.InputFormat) != 0 {
		str := stride / pixelSize(info.InputFormat)
		values[0] = float32(float64(rdF32(buf, 0)) / maxEncodeableXYZ)
		values[1] = float32(float64(rdF32(buf, str*4)) / maxEncodeableXYZ)
		values[2] = float32(float64(rdF32(buf, str*2*4)) / maxEncodeableXYZ)
		return 4
	}
	values[0] = float32(float64(rdF32(buf, 0)) / maxEncodeableXYZ)
	values[1] = float32(float64(rdF32(buf, 4)) / maxEncodeableXYZ)
	values[2] = float32(float64(rdF32(buf, 8)) / maxEncodeableXYZ)
	return 4 * (3 + int(tExtra(info.InputFormat)))
}

// lab4ToFloat ports lab4toFloat.
func lab4ToFloat(values []float32, lab4 [3]uint16) {
	L := float32(lab4[0]) / 655.35
	a := float32(lab4[1])/257.0 - 128.0
	b := float32(lab4[2])/257.0 - 128.0
	values[0] = L / 100.0
	values[1] = (a + 128.0) / 255.0
	values[2] = (b + 128.0) / 255.0
}

func unrollLabV2_8ToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	var lab4 [3]uint16
	lab4[0] = fomLabV2ToLabV4(from8to16(rdU8(buf, 0)))
	lab4[1] = fomLabV2ToLabV4(from8to16(rdU8(buf, 1)))
	lab4[2] = fomLabV2ToLabV4(from8to16(rdU8(buf, 2)))
	lab4ToFloat(values, lab4)
	return 3
}

func unrollALabV2_8ToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	var lab4 [3]uint16
	// accum++ (A)
	lab4[0] = fomLabV2ToLabV4(from8to16(rdU8(buf, 1)))
	lab4[1] = fomLabV2ToLabV4(from8to16(rdU8(buf, 2)))
	lab4[2] = fomLabV2ToLabV4(from8to16(rdU8(buf, 3)))
	lab4ToFloat(values, lab4)
	return 4
}

func unrollLabV2_16ToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	var lab4 [3]uint16
	lab4[0] = fomLabV2ToLabV4(rdU16(buf, 0))
	lab4[1] = fomLabV2ToLabV4(rdU16(buf, 2))
	lab4[2] = fomLabV2ToLabV4(rdU16(buf, 4))
	lab4ToFloat(values, lab4)
	return 6
}

// UnrollHalfTo16: NOTE the reference divides Stride by PixelSize(OutputFormat)
// (a quirk; input formatter reading the output format). Ported verbatim.
func unrollHalfTo16(info *FormatterInfo, values []uint16, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	maximum := float32(65535.0)
	if isInkSpace(f) {
		maximum = 655.35
	}

	str := stride / pixelSize(info.OutputFormat)

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = half2Float(rdU16(buf, (i+start)*str*2))
		} else {
			v = half2Float(rdU16(buf, (i+start)*2))
		}
		if reverse {
			v = maximum - v
		}
		values[index] = quickSaturateWord(float64(v) * float64(maximum))
	}

	if extra == 0 && swapFirst {
		rotateWordsLeft(values, nChan)
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}

func unrollHalfToFloat(info *FormatterInfo, values []float32, buf []byte, stride int) int {
	f := info.InputFormat
	nChan := int(tChannels(f))
	doSwap := tDoSwap(f) != 0
	reverse := tFlavor(f) != 0
	swapFirst := tSwapFirst(f) != 0
	extra := int(tExtra(f))
	extraFirst := (tDoSwap(f) ^ tSwapFirst(f)) != 0
	planar := tPlanar(f) != 0

	maximum := float32(1.0)
	if isInkSpace(f) {
		maximum = 100.0
	}

	str := stride / pixelSize(f)

	start := 0
	if extraFirst {
		start = extra
	}

	for i := 0; i < nChan; i++ {
		index := i
		if doSwap {
			index = nChan - i - 1
		}
		var v float32
		if planar {
			v = half2Float(rdU16(buf, (i+start)*str*2))
		} else {
			v = half2Float(rdU16(buf, (i+start)*2))
		}
		v /= maximum
		if reverse {
			v = 1 - v
		}
		values[index] = v
	}

	if extra == 0 && swapFirst {
		rotateFloatsLeft(values, nChan)
	}

	if planar {
		return 2
	}
	return (nChan + extra) * 2
}
