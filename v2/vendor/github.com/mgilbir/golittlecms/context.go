package lcms2

import (
	"sync"
	"unicode/utf8"
)

// This file ports the context, error-logging and per-context state machinery
// from the reference implementation's src/cmserr.c and src/cmsplugin.c
// (lcms2 2.19). The plugin registration dispatch lives in plugin.go.
//
// Intentional deviations from the C reference, all a consequence of Go's
// runtime model rather than of behaviour:
//
//   - The custom memory manager (malloc/realloc/free plug-in) and the
//     sub-allocator (_cmsSubAlloc*) are omitted: Go's garbage collector owns
//     memory. cmsPluginMemHandlerSig plug-ins are still accepted by the
//     dispatcher (so code that registers one keeps working) but have no effect.
//     The one behaviour of the default manager that is *not* a memory-model
//     detail — its refusal to serve any single allocation over 512 MB — is
//     observable through the public API (a constructor asking for more fails)
//     and is therefore kept, as allocSizeOK below.
//   - The mutex plug-in is likewise inert: Go's sync package provides locking
//     natively. The registry field is kept for API parity and observability.
//   - The C "chunk" indirection (an array of void* client chunks per context,
//     each NULL entry falling back to the global Context0) is replaced by typed
//     fields. The fallback-to-defaults semantics is realised by seeding every
//     new context with the default values (see resetState) and by deep-copying
//     them in Dup, which is behaviourally identical.
//   - The C endian/IO helpers in cmsplugin.c (_cmsReadUInt*, _cmsWriteUInt*,
//     alignment, _cmsIOPrintf, date/time and fixed-point encoders) belong to
//     the profile-IO layer and are deferred to Phase 3 (W7).

// maxErrorMessageLen mirrors MAX_ERROR_MESSAGE_LEN in cmserr.c. Formatted error
// messages are truncated to one byte less, matching the C vsnprintf call.
const maxErrorMessageLen = 1024

// maxMemoryForAlloc mirrors MAX_MEMORY_FOR_ALLOC in cmserr.c: the largest
// single block the reference's default memory manager will hand out. The
// reference raises this to 2 GB under CMS_LARGE_FILE_SUPPORT, which neither the
// oracle build (scripts/build-oracle.sh) nor this port defines, so the 512 MB
// arm is the one to match.
const maxMemoryForAlloc = 1024 * 1024 * 512

// allocSizeOK reports whether an array of num elements of size bytes each may be
// allocated, mirroring the guards in _cmsCallocDefaultFn (cmserr.c): a total of
// zero bytes is refused (preserving calloc's NULL-for-nothing behaviour), as is
// any total that overflows or exceeds maxMemoryForAlloc. The C overflow test
// (num >= UINT_MAX/size, plus the wrapped-product checks) is subsumed here by
// computing the product in uint64: every product it rejects is at least 4 GB and
// so already over the cap.
//
// Go's make() has no such ceiling, so ports of C code whose allocation size
// derives from caller-supplied scalars must consult this before make() to keep
// parity: where the C constructor gets a NULL from _cmsCalloc and fails, the Go
// one must fail too rather than commit gigabytes of RAM.
func allocSizeOK(num, size uint32) bool {
	if size == 0 {
		return false
	}
	total := uint64(num) * uint64(size)
	return total != 0 && total <= maxMemoryForAlloc
}

// maxChannels mirrors cmsMAXCHANNELS (lcms2.h): the maximum number of channels
// an ICC profile may carry, and the width of the alarm-code array.
const maxChannels = 16

// defaultObserverAdaptationState mirrors DEFAULT_OBSERVER_ADAPTATION_STATE
// (cmsxform.c): full adaptation.
const defaultObserverAdaptationState = 1.0

// defaultAlarmCodes mirrors DEFAULT_ALARM_CODES_VALUE (cmsxform.c): the first
// three 16-bit channels are 0x7F00, the rest zero.
var defaultAlarmCodes = [maxChannels]uint16{0x7F00, 0x7F00, 0x7F00}

// LogErrorHandlerFunc is the optional per-context error logger. It mirrors the
// C cmsLogErrorHandlerFunction callback: it is invoked with the originating
// context, the error class, and an English description whenever the library
// signals an error. The returned *Error remains the primary error channel; a
// handler is a passive observer and must not terminate the program.
type LogErrorHandlerFunc func(ctx *Context, code ErrorCode, text string)

// Context is the pure-Go replacement for the C cmsContext. It holds all
// per-context state: the user data pointer, the error logger, the alarm codes
// and adaptation state used by transforms, and one registry per plug-in family.
//
// A Context is safe for concurrent use by multiple goroutines: all mutable
// state is guarded by mu. Registries are copied on Dup, so a duplicated context
// is fully isolated from its parent. Callers that never create an explicit
// context can use the package-level functions, which operate on a shared
// default context.
type Context struct {
	mu sync.Mutex

	userData any                 // cmsContext UserPtr chunk
	logger   LogErrorHandlerFunc // Logger chunk; nil means the no-op default

	alarmCodes      [maxChannels]uint16 // AlarmCodesContext chunk
	adaptationState float64             // AdaptationStateContext chunk

	// One registry per plug-in family (see plugin.go). The memory-manager
	// family is intentionally absent (replaced by the Go GC).
	interp           pluginList // InterpPlugin
	parametricCurves pluginList // CurvesPlugin
	formatters       pluginList // FormattersPlugin
	tagType          pluginList // TagTypePlugin
	mpeType          pluginList // MPEPlugin (shares the chunk type with TagType in C)
	tag              pluginList // TagPlugin
	intents          pluginList // IntentPlugin
	optimization     pluginList // OptimizationPlugin
	transform        pluginList // TransformPlugin
	mutex            pluginList // MutexPlugin (inert; kept for parity)
	parallelization  pluginList // ParallelizationPlugin
}

// defaultContext is the package-level context that the package-level functions
// operate on. It replaces the C global Context0 (globalContext in cmsplugin.c).
var defaultContext = func() *Context {
	c := &Context{}
	c.resetState()
	return c
}()

// DefaultContext returns the shared context used by all package-level
// functions. It is the analogue of passing a NULL cmsContext to the C API.
func DefaultContext() *Context { return defaultContext }

// resetState seeds the non-registry per-context state with the library
// defaults. It is used when creating the default context and when creating a
// fresh context without a source to copy from.
func (ctx *Context) resetState() {
	ctx.logger = nil
	ctx.alarmCodes = defaultAlarmCodes
	ctx.adaptationState = defaultObserverAdaptationState
}

// NewContext creates a new context with an optional plug-in chain and optional
// user data, mirroring cmsCreateContext(Plugin, UserData). Pass a nil plugin to
// create a context with only the built-in behaviour. If any plug-in in the
// chain is malformed (bad magic, or a required newer library version, or an
// unknown type) NewContext returns the corresponding *Error and no context.
func NewContext(plugin Plugin, userData any) (*Context, error) {
	ctx := &Context{userData: userData}
	ctx.resetState()

	if plugin != nil {
		if err := ctx.RegisterPlugins(plugin); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

// Dup duplicates ctx together with all of its per-context state and registered
// plug-ins, mirroring cmsDupContext(ContextID, NewUserData). If newUserData is
// non-nil it becomes the new context's user data; otherwise the parent's user
// data pointer is inherited. The returned context is fully isolated: mutating
// its state or registries does not affect ctx.
func (ctx *Context) Dup(newUserData any) (*Context, error) {
	ctx.mu.Lock()
	dup := &Context{
		userData:        ctx.userData,
		logger:          ctx.logger,
		alarmCodes:      ctx.alarmCodes,
		adaptationState: ctx.adaptationState,

		interp:           ctx.interp.clone(),
		parametricCurves: ctx.parametricCurves.clone(),
		formatters:       ctx.formatters.clone(),
		tagType:          ctx.tagType.clone(),
		mpeType:          ctx.mpeType.clone(),
		tag:              ctx.tag.clone(),
		intents:          ctx.intents.clone(),
		optimization:     ctx.optimization.clone(),
		transform:        ctx.transform.clone(),
		mutex:            ctx.mutex.clone(),
		parallelization:  ctx.parallelization.clone(),
	}
	ctx.mu.Unlock()

	if newUserData != nil {
		dup.userData = newUserData
	}
	return dup, nil
}

// Delete releases ctx. Under the Go garbage collector there is nothing to free,
// so this only reverts the context to its pristine, plug-in-free state; it
// exists for symmetry with cmsDeleteContext. The context must not be used
// concurrently with Delete.
func (ctx *Context) Delete() {
	ctx.UnregisterPlugins()
}

// UserData returns the user data associated with ctx, or nil if none was
// supplied. Mirrors cmsGetContextUserData.
func (ctx *Context) UserData() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.userData
}

// GetContextUserData returns the user data of the default context.
func GetContextUserData() any { return defaultContext.UserData() }

// SetLogErrorHandler installs fn as the error logger for ctx, mirroring
// cmsSetLogErrorHandlerTHR. A nil fn reverts to the no-op default handler.
func (ctx *Context) SetLogErrorHandler(fn LogErrorHandlerFunc) {
	ctx.mu.Lock()
	ctx.logger = fn
	ctx.mu.Unlock()
}

// SetLogErrorHandler installs fn as the error logger for the default context,
// mirroring cmsSetLogErrorHandler.
func SetLogErrorHandler(fn LogErrorHandlerFunc) {
	defaultContext.SetLogErrorHandler(fn)
}

// signalError builds an *Error and, if ctx has a log handler installed, invokes
// it with the error class and message before returning. It is the internal
// analogue of cmsSignalError: the message is delivered to the optional logger
// and also returned so that callers propagate it. The formatted message is
// truncated to maxErrorMessageLen-1 bytes, matching the C buffer limit.
func (ctx *Context) signalError(code ErrorCode, format string, args ...any) *Error {
	err := errorf(code, format, args...)
	// C's vsnprintf(Buffer, MAX_ERROR_MESSAGE_LEN-1, ...) reserves one byte for
	// the terminator, so the content is capped at MAX_ERROR_MESSAGE_LEN-2. Trim
	// on a UTF-8 rune boundary so we never emit an invalid partial rune.
	if maxContent := maxErrorMessageLen - 2; len(err.Msg) > maxContent {
		cut := maxContent
		for cut > 0 && !utf8.RuneStart(err.Msg[cut]) {
			cut--
		}
		err.Msg = err.Msg[:cut]
	}

	// A nil context resolves to the default context, whose logger still fires —
	// mirroring the reference, where cmsSignalError(NULL, ...) logs through the
	// global context chunk.
	if ctx == nil {
		ctx = defaultContext
	}
	if ctx != nil {
		ctx.mu.Lock()
		h := ctx.logger
		ctx.mu.Unlock()
		if h != nil {
			h(ctx, code, err.Msg)
		}
	}
	return err
}

// SetAdaptationState sets the observer adaptation state used by absolute
// colorimetric intent for ctx and returns the previous value, mirroring
// cmsSetAdaptationStateTHR. A negative d leaves the state unchanged (used to
// query the current value); the previous value is always returned.
func (ctx *Context) SetAdaptationState(d float64) float64 {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	prev := ctx.adaptationState
	if d >= 0.0 {
		ctx.adaptationState = d
	}
	return prev
}

// SetAdaptationState sets the adaptation state of the default context, mirroring
// cmsSetAdaptationState.
func SetAdaptationState(d float64) float64 {
	return defaultContext.SetAdaptationState(d)
}

// SetAlarmCodes sets the 16 out-of-gamut alarm codes for ctx, mirroring
// cmsSetAlarmCodesTHR. Values are meant to be encoded in 16 bits.
func (ctx *Context) SetAlarmCodes(codes [maxChannels]uint16) {
	ctx.mu.Lock()
	ctx.alarmCodes = codes
	ctx.mu.Unlock()
}

// GetAlarmCodes returns the current 16 out-of-gamut alarm codes for ctx,
// mirroring cmsGetAlarmCodesTHR.
func (ctx *Context) GetAlarmCodes() [maxChannels]uint16 {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.alarmCodes
}

// SetAlarmCodes sets the alarm codes of the default context, mirroring
// cmsSetAlarmCodes.
func SetAlarmCodes(codes [maxChannels]uint16) {
	defaultContext.SetAlarmCodes(codes)
}

// GetAlarmCodes returns the alarm codes of the default context, mirroring
// cmsGetAlarmCodes.
func GetAlarmCodes() [maxChannels]uint16 {
	return defaultContext.GetAlarmCodes()
}
