package lcms2

// This file ports the plug-in registration core from src/cmsplugin.c
// (cmsPlugin / cmsPluginTHR / cmsUnregisterPlugins) and the plug-in foundation
// constants from include/lcms2_plugin.h.
//
// Only the registration dispatch is ported here. The per-family register
// functions in C (_cmsRegisterInterpPlugin, _cmsRegisterTagTypePlugin, ...)
// each interpret a family-specific plug-in struct and either replace a single
// factory (interpolation, parametric curves, mutex, parallelization) or prepend
// an entry to a linked list (tag types, tags, formatters, intents, MPE,
// optimization, transform). The families' concrete plug-in payloads are defined
// by later phases; this phase provides the generic chain machinery with
// newest-first ordering and fallback-to-builtins lookup, and validates magic,
// version and type exactly as cmsPluginTHR does.

// Plug-in foundation, mirroring include/lcms2_plugin.h.
const (
	// pluginMagicNumber is 'acpp'; every plug-in header must carry it.
	pluginMagicNumber = 0x61637070

	pluginMemHandlerSig          = 0x6D656D48 // 'memH'
	pluginInterpolationSig       = 0x696E7048 // 'inpH'
	pluginParametricCurveSig     = 0x70617248 // 'parH'
	pluginFormattersSig          = 0x66726D48 // 'frmH'
	pluginTagTypeSig             = 0x74797048 // 'typH'
	pluginTagSig                 = 0x74616748 // 'tagH'
	pluginRenderingIntentSig     = 0x696E7448 // 'intH'
	pluginMultiProcessElementSig = 0x6D706548 // 'mpeH'
	pluginOptimizationSig        = 0x6F707448 // 'optH'
	pluginTransformSig           = 0x7A666D48 // 'xfmH'
	pluginMutexSig               = 0x6D747A48 // 'mtxH'
	pluginParalellizationSig     = 0x70726C48 // 'prlH'
)

// PluginBase mirrors cmsPluginBase (the _cmsPluginBaseStruct header). Every
// concrete plug-in embeds it as its first field and is reached through the
// Plugin interface. Next chains multiple plug-ins so a single RegisterPlugins
// call can install several at once, exactly like the C Next pointer.
type PluginBase struct {
	Magic           uint32 // must equal pluginMagicNumber ('acpp')
	ExpectedVersion uint32 // minimum library version the plug-in needs
	Type            uint32 // one of the pluginXxxSig family signatures
	Next            Plugin // next plug-in in the bundle, or nil
}

// Base returns the plug-in header. It lets a *PluginBase (and, by embedding,
// any concrete plug-in) satisfy the Plugin interface.
func (b *PluginBase) Base() *PluginBase { return b }

// Plugin is the Go analogue of a pointer to cmsPluginBase. Concrete plug-in
// types embed PluginBase, so a pointer to them satisfies this interface and
// carries the family-specific payload that later phases interpret.
type Plugin interface {
	// Base returns the embedded plug-in header (magic, version, type, next).
	Base() *PluginBase
}

// pluginList is the generic per-family registry. Registered plug-ins are held
// newest-first, matching the C list insertion (pt->Next = head; head = pt), so
// that lookup consults the most recently registered entry first and falls back
// to the built-in behaviour when the list is exhausted. The built-in list is
// empty until later phases populate their families. Access is serialised by the
// owning Context's mutex.
type pluginList struct {
	entries []Plugin // newest-first
}

// register prepends p, so the most recently registered plug-in wins, matching
// the C behaviour for both the list families and the single-factory families.
func (l *pluginList) register(p Plugin) {
	l.entries = append([]Plugin{p}, l.entries...)
}

// reset drops all registered plug-ins, reverting the family to its built-in
// defaults. Mirrors the Data==NULL branch of each _cmsRegisterXxxPlugin.
func (l *pluginList) reset() { l.entries = nil }

// clone returns an independent copy so a duplicated context shares no registry
// storage with its parent.
func (l pluginList) clone() pluginList {
	if len(l.entries) == 0 {
		return pluginList{}
	}
	c := make([]Plugin, len(l.entries))
	copy(c, l.entries)
	return pluginList{entries: c}
}

// registryFor maps a plug-in type signature to the owning registry field, or
// nil if the type has no registry (an unknown type). The memory-handler
// signature deliberately has no registry: it is handled as an accepted no-op in
// RegisterPlugins because the Go garbage collector replaces the C allocator.
func (ctx *Context) registryFor(typ uint32) *pluginList {
	switch typ {
	case pluginInterpolationSig:
		return &ctx.interp
	case pluginParametricCurveSig:
		return &ctx.parametricCurves
	case pluginFormattersSig:
		return &ctx.formatters
	case pluginTagTypeSig:
		return &ctx.tagType
	case pluginMultiProcessElementSig:
		return &ctx.mpeType
	case pluginTagSig:
		return &ctx.tag
	case pluginRenderingIntentSig:
		return &ctx.intents
	case pluginOptimizationSig:
		return &ctx.optimization
	case pluginTransformSig:
		return &ctx.transform
	case pluginMutexSig:
		return &ctx.mutex
	case pluginParalellizationSig:
		return &ctx.parallelization
	default:
		return nil
	}
}

// RegisterPlugins installs the plug-in chain starting at plugin into ctx,
// mirroring cmsPluginTHR. It walks the Next chain and, for each entry,
// validates the magic number and the expected version, then dispatches on the
// type. A malformed header (bad magic, a version newer than this library, or an
// unrecognised type) aborts the whole call and returns the corresponding
// *Error, exactly as the C code returns FALSE after logging
// cmsERROR_UNKNOWN_EXTENSION. A nil plugin is accepted as a no-op.
func (ctx *Context) RegisterPlugins(plugin Plugin) error {
	for p := plugin; p != nil; {
		b := p.Base()
		if b == nil {
			return ctx.signalError(ErrUnknownExtension, "Unrecognized plugin")
		}

		if b.Magic != pluginMagicNumber {
			return ctx.signalError(ErrUnknownExtension, "Unrecognized plugin")
		}

		if b.ExpectedVersion > Version {
			return ctx.signalError(ErrUnknownExtension,
				"plugin needs Little CMS %d, current version is %d",
				b.ExpectedVersion, Version)
		}

		switch b.Type {
		case pluginMemHandlerSig:
			// The custom memory manager is replaced by the Go garbage
			// collector; accept the plug-in but do nothing with it.
		default:
			reg := ctx.registryFor(b.Type)
			if reg == nil {
				return ctx.signalError(ErrUnknownExtension,
					"Unrecognized plugin type '%X'", b.Type)
			}
			ctx.mu.Lock()
			reg.register(p)
			ctx.mu.Unlock()
		}

		p = b.Next
	}
	return nil
}

// RegisterPlugin installs a plug-in chain into the default context, mirroring
// cmsPlugin.
func RegisterPlugin(plugin Plugin) error {
	return defaultContext.RegisterPlugins(plugin)
}

// UnregisterPlugins reverts every plug-in family of ctx to its pristine,
// built-in state, mirroring cmsUnregisterPluginsTHR. As in C there is no way to
// remove a single plug-in, since one RegisterPlugins call may install many.
func (ctx *Context) UnregisterPlugins() {
	ctx.mu.Lock()
	ctx.interp.reset()
	ctx.parametricCurves.reset()
	ctx.formatters.reset()
	ctx.tagType.reset()
	ctx.mpeType.reset()
	ctx.tag.reset()
	ctx.intents.reset()
	ctx.optimization.reset()
	ctx.transform.reset()
	ctx.mutex.reset()
	ctx.parallelization.reset()
	ctx.mu.Unlock()
}

// UnregisterPlugins reverts the default context's plug-in families to their
// built-in state, mirroring cmsUnregisterPlugins.
func UnregisterPlugins() {
	defaultContext.UnregisterPlugins()
}
