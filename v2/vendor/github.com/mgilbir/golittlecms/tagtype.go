// Tag-type dispatch and registry lookup, ported from the plug-in surface of
// src/cmstypes.c (_cmsGetTagTypeHandler, _cmsGetTagDescriptor,
// _cmsAvoidTypeCheckOnTags) and include/lcms2_plugin.h (cmsTagTypeHandler,
// cmsTagDescriptor, cmsPluginTagType, cmsPluginTag).
//
// This file provides ONLY the dispatch machinery and the empty built-in tables.
// The concrete per-type read/write handlers live in cmstypes.c and are the
// responsibility of W8, who populates the built-in tables (registerBuiltinType /
// registerBuiltinTag below) and implements the TagTypeHandler function fields.
//
// Contract W8 must honour
// ------------------------
//   - Implement each handler as a *TagTypeHandler whose Read/Write/Dup/Free
//     fields have the signatures documented on the struct, and register it once
//     at init time with registerBuiltinType(handler). Read returns the parsed
//     Go value (any), the element count, and an error; a nil value with a nil
//     error is treated by the container as "corrupted tag" exactly as the C code
//     treats a NULL ReadPtr result.
//   - For every tag, register a *TagDescriptor via registerBuiltinTag(sig, desc)
//     describing its ElemCount, the ordered SupportedTypes list, and an optional
//     DecideType selector. The container's read path checks the parsed base type
//     against SupportedTypes; the write path (SaveTags) picks SupportedTypes[0]
//     or the DecideType result.
//   - MPE element handlers share the TagTypeHandler shape but live in the
//     separate mpeType registry; the container does not consult them directly.

package lcms2

// TagTypeHandler is the pure-Go analogue of cmsTagTypeHandler. It serializes and
// deserializes one tag type. The container copies the handler and stamps
// ContextID/ICCVersion before each call, exactly as cmsio0.c does with its
// LocalTypeHandler, so a handler's function fields must be reentrant and read
// those two fields from the self pointer rather than from captured state.
type TagTypeHandler struct {
	// Signature is the tag-type signature this handler serializes.
	Signature TagTypeSignature

	// Read allocates and reads a value from io. sizeOfTag is the number of bytes
	// available for the tag body (the 8-byte type base has already been
	// consumed). It returns the parsed value, the number of elements it
	// represents, and an error. Mirrors ReadPtr.
	Read func(self *TagTypeHandler, io *IOHandler, sizeOfTag uint32) (value any, count uint32, err error)

	// Write serializes nItems elements of value to io. Mirrors WritePtr.
	Write func(self *TagTypeHandler, io *IOHandler, value any, nItems uint32) error

	// Dup returns an independent copy of value (n elements). Mirrors DupPtr. In
	// Go, immutable values may return themselves. A nil return signals failure.
	Dup func(self *TagTypeHandler, value any, n uint32) (any, error)

	// Free releases any resources held by value. Mirrors FreePtr. Under the Go
	// garbage collector it is usually a no-op and may be nil.
	Free func(self *TagTypeHandler, value any)

	// ContextID and ICCVersion are stamped per call by the container.
	ContextID  *Context
	ICCVersion uint32
}

// TagDescriptor is the pure-Go analogue of cmsTagDescriptor. It describes how a
// tag maps to serialized types.
type TagDescriptor struct {
	// ElemCount is how many elements the tag's value array holds.
	ElemCount uint32
	// SupportedTypes lists, most-preferred first, the tag-type signatures this
	// tag may be stored as. At most maxTypesInPlugin are consulted.
	SupportedTypes []TagTypeSignature
	// DecideType, if non-nil, chooses the write type from the ICC version and
	// the value, overriding SupportedTypes[0]. Mirrors the DecideType pointer.
	DecideType func(iccVersion float64, data any) TagTypeSignature
}

// PluginTagType is the plug-in payload that registers one tag-type handler,
// mirroring cmsPluginTagType. A pointer to it satisfies Plugin.
type PluginTagType struct {
	PluginBase
	Handler TagTypeHandler
}

// PluginTag is the plug-in payload that registers one tag descriptor, mirroring
// cmsPluginTag. A pointer to it satisfies Plugin.
type PluginTag struct {
	PluginBase
	Signature  TagSignature
	Descriptor TagDescriptor
}

// Built-in tables. These are intentionally empty in this phase; W8 fills them
// from cmstypes.c via the register helpers below. They are keyed by signature
// for O(1) lookup, mirroring the linear scan of the C static arrays but with the
// same "first match wins" outcome since signatures are unique within a table.
var (
	builtinTagTypes = map[TagTypeSignature]*TagTypeHandler{}
	builtinTags     = map[TagSignature]*TagDescriptor{}
)

// registerBuiltinType installs h as the built-in handler for its signature. W8
// calls this from init() for each type it implements. Later registration for
// the same signature overwrites earlier, matching the C convention that the
// last static entry wins.
func registerBuiltinType(h *TagTypeHandler) {
	builtinTagTypes[h.Signature] = h
}

// registerBuiltinTag installs desc as the built-in descriptor for sig.
func registerBuiltinTag(sig TagSignature, desc *TagDescriptor) {
	builtinTags[sig] = desc
}

// getTagTypeHandler ports _cmsGetTagTypeHandler: consult the context's
// registered tag-type plug-ins (newest first) then the built-in table. Returns
// nil when no handler exists.
func (ctx *Context) getTagTypeHandler(sig TagTypeSignature) *TagTypeHandler {
	if ctx != nil {
		ctx.mu.Lock()
		entries := ctx.tagType.entries
		ctx.mu.Unlock()
		for _, p := range entries {
			if pt, ok := p.(*PluginTagType); ok && pt.Handler.Signature == sig {
				return &pt.Handler
			}
		}
	}
	if h, ok := builtinTagTypes[sig]; ok {
		return h
	}
	return nil
}

// getTagDescriptor ports _cmsGetTagDescriptor: consult the context's registered
// tag plug-ins (newest first) then the built-in table. Returns nil when the tag
// is unknown.
func (ctx *Context) getTagDescriptor(sig TagSignature) *TagDescriptor {
	if ctx != nil {
		ctx.mu.Lock()
		entries := ctx.tag.entries
		ctx.mu.Unlock()
		for _, p := range entries {
			if pt, ok := p.(*PluginTag); ok && pt.Signature == sig {
				return &pt.Descriptor
			}
		}
	}
	if d, ok := builtinTags[sig]; ok {
		return d
	}
	return nil
}

// avoidTypeCheckOnTags ports _cmsAvoidTypeCheckOnTags. The corresponding flag is
// set only through an internal, undocumented hook in the C code that no public
// API exercises; the port keeps it always false, matching a freshly created
// context.
func (ctx *Context) avoidTypeCheckOnTags() bool { return false }

// isTypeSupported ports IsTypeSupported (cmsio0.c): reports whether a descriptor
// lists typ among its supported types, honouring the maxTypesInPlugin cap.
func isTypeSupported(desc *TagDescriptor, typ TagTypeSignature) bool {
	n := len(desc.SupportedTypes)
	if n > maxTypesInPlugin {
		n = maxTypesInPlugin
	}
	for i := 0; i < n; i++ {
		if desc.SupportedTypes[i] == typ {
			return true
		}
	}
	return false
}

// compatibleTypes ports CompatibleTypes (cmsio0.c): two tags may be linked only
// if their descriptors declare the same element count and the same ordered set
// of supported types.
func compatibleTypes(d1, d2 *TagDescriptor) bool {
	if d1 == nil || d2 == nil {
		return false
	}
	if len(d1.SupportedTypes) != len(d2.SupportedTypes) {
		return false
	}
	if d1.ElemCount != d2.ElemCount {
		return false
	}
	for i := range d1.SupportedTypes {
		if d1.SupportedTypes[i] != d2.SupportedTypes[i] {
			return false
		}
	}
	return true
}
