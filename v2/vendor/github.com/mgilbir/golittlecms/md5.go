// Port of src/cmsmd5.c — MD5 used to compute the ICC profile ID (cmsProfileID).
//
// The reference ships its own MD5 implementation, but it is bit-for-bit the
// standard RFC 1321 MD5: cmsMD5_Transform is the canonical transform, and on
// little-endian targets byteReverse is a no-op while on big-endian it swaps the
// message/digest words so the algorithm behaves as standard (little-endian)
// MD5 everywhere. cmsMD5finish copies the four state words out in little-endian
// byte order into cmsProfileID.ID8 — exactly the standard 16-byte MD5 digest.
//
// Therefore this port uses crypto/md5 from the standard library. The public
// C surface used by cmsMD5computeID — _cmsMD5alloc / _cmsMD5add / _cmsMD5finish
// — is mirrored by the md5Ctx accumulator below. cmsMD5computeID itself (which
// serializes the profile) is not ported here; that belongs to the profile-I/O
// work (Phase 3).

package lcms2

import (
	"crypto/md5"
	"hash"
)

// profileID mirrors cmsProfileID.ID8: the 16-byte MD5 profile identifier as it
// is laid out in the ICC header. The byte order is the standard MD5 digest
// order, which is what the reference stores.
type profileID [16]byte

// md5Ctx is an MD5 accumulator mirroring the reference _cmsMD5 object. It wraps
// the standard-library MD5 hash.
type md5Ctx struct {
	// h is never nil once constructed via md5Alloc.
	h hash.Hash
}

// md5Alloc creates an MD5 accumulator. Port of cmsMD5alloc (the ContextID is not
// needed in Go: allocation cannot fail here, so there is no error return).
func md5Alloc() *md5Ctx {
	return &md5Ctx{h: md5.New()}
}

// add feeds bytes into the accumulator. Port of cmsMD5add. hash.Hash.Write never
// returns an error, so none is surfaced.
func (m *md5Ctx) add(buf []byte) {
	m.h.Write(buf)
}

// finish returns the 16-byte digest as an ICC profile ID. Port of cmsMD5finish;
// unlike the C version it does not free the object (Go is garbage collected) and
// it returns the ID rather than writing through a pointer.
func (m *md5Ctx) finish() profileID {
	var id profileID
	copy(id[:], m.h.Sum(nil))
	return id
}
