// Package provider defines the versioned, provider-neutral contracts used by
// MindWeaver inference adapters.
//
// The package deliberately contains no transport implementation and performs
// no network access. Provider credentials, prompt bodies, generated content,
// and HTTP headers are intentionally outside these serializable contracts.
package provider
