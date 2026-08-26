// Header field accessors and profile-version codec ported from src/cmsio0.c
// (the cmsGetHeader*/cmsSetHeader* family, cmsGet/SetProfileVersion and the
// BaseToBase helper). Kept separate from io0.go for readability.

package lcms2

import (
	"math"
	"os"
	"time"
)

// removeFile deletes a file, ignoring the result, mirroring the C remove() used
// to clean up a partially written profile.
func removeFile(name string) error { return os.Remove(name) }

// GetHeaderCMM mirrors cmsGetHeaderCMM.
func (p *Profile) GetHeaderCMM() uint32 { return p.CMM }

// SetHeaderCMM mirrors _cmsSetHeaderCMM.
func (p *Profile) SetHeaderCMM(cmm uint32) { p.CMM = cmm }

// GetHeaderRenderingIntent mirrors cmsGetHeaderRenderingIntent.
func (p *Profile) GetHeaderRenderingIntent() uint32 { return p.RenderingIntent }

// SetHeaderRenderingIntent mirrors cmsSetHeaderRenderingIntent.
func (p *Profile) SetHeaderRenderingIntent(intent uint32) { p.RenderingIntent = intent }

// GetHeaderFlags mirrors cmsGetHeaderFlags.
func (p *Profile) GetHeaderFlags() uint32 { return p.flags }

// SetHeaderFlags mirrors cmsSetHeaderFlags.
func (p *Profile) SetHeaderFlags(flags uint32) { p.flags = flags }

// GetHeaderManufacturer mirrors cmsGetHeaderManufacturer.
func (p *Profile) GetHeaderManufacturer() uint32 { return p.manufacturer }

// SetHeaderManufacturer mirrors cmsSetHeaderManufacturer.
func (p *Profile) SetHeaderManufacturer(m uint32) { p.manufacturer = m }

// GetHeaderCreator mirrors cmsGetHeaderCreator.
func (p *Profile) GetHeaderCreator() uint32 { return p.creator }

// GetHeaderModel mirrors cmsGetHeaderModel.
func (p *Profile) GetHeaderModel() uint32 { return p.model }

// SetHeaderModel mirrors cmsSetHeaderModel.
func (p *Profile) SetHeaderModel(m uint32) { p.model = m }

// GetHeaderAttributes mirrors cmsGetHeaderAttributes.
func (p *Profile) GetHeaderAttributes() uint64 { return p.attributes }

// SetHeaderAttributes mirrors cmsSetHeaderAttributes.
func (p *Profile) SetHeaderAttributes(a uint64) { p.attributes = a }

// GetHeaderCreationDateTime mirrors cmsGetHeaderCreationDateTime, returning the
// creation timestamp as a UTC time.Time.
func (p *Profile) GetHeaderCreationDateTime() time.Time {
	d := p.Created
	return time.Date(int(d.Year), time.Month(d.Month), int(d.Day),
		int(d.Hours), int(d.Minutes), int(d.Seconds), 0, time.UTC)
}

// GetPCS mirrors cmsGetPCS.
func (p *Profile) GetPCS() ColorSpaceSignature { return p.PCS }

// SetPCS mirrors cmsSetPCS.
func (p *Profile) SetPCS(pcs ColorSpaceSignature) { p.PCS = pcs }

// GetColorSpace mirrors cmsGetColorSpace.
func (p *Profile) GetColorSpace() ColorSpaceSignature { return p.ColorSpace }

// SetColorSpace mirrors cmsSetColorSpace.
func (p *Profile) SetColorSpace(sig ColorSpaceSignature) { p.ColorSpace = sig }

// GetDeviceClass mirrors cmsGetDeviceClass.
func (p *Profile) GetDeviceClass() ProfileClassSignature { return p.DeviceClass }

// SetDeviceClass mirrors cmsSetDeviceClass.
func (p *Profile) SetDeviceClass(sig ProfileClassSignature) { p.DeviceClass = sig }

// GetEncodedICCversion mirrors cmsGetEncodedICCversion.
func (p *Profile) GetEncodedICCversion() uint32 { return p.Version }

// SetEncodedICCversion mirrors cmsSetEncodedICCversion.
func (p *Profile) SetEncodedICCversion(v uint32) { p.Version = v }

// baseToBase ports BaseToBase: reinterpret the digits of in (written in baseIn)
// as if they were digits in baseOut. Used to convert between the decimal profile
// version and its packed BCD-in-hex header encoding.
func baseToBase(in uint32, baseIn, baseOut int) uint32 {
	var buff [100]byte
	length := 0
	for in > 0 && length < 100 {
		buff[length] = byte(in % uint32(baseIn))
		in /= uint32(baseIn)
		length++
	}
	var out uint32
	for i := length - 1; i >= 0; i-- {
		out = out*uint32(baseOut) + uint32(buff[i])
	}
	return out
}

// SetProfileVersion mirrors cmsSetProfileVersion: 4.2 -> 0x04200000.
func (p *Profile) SetProfileVersion(version float64) {
	p.Version = baseToBase(uint32(math.Floor(version*100.0+0.5)), 10, 16) << 16
}

// GetProfileVersion mirrors cmsGetProfileVersion: 0x02100000 -> 2.10.
func (p *Profile) GetProfileVersion() float64 {
	n := p.Version >> 16
	return float64(baseToBase(n, 16, 10)) / 100.0
}
