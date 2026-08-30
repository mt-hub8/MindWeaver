//go:build !windows

package ideashook

import (
	"context"
	"io"
)

func ensureHookConfigMutationSupported() error { return errHookConfigMutationUnsupported }

func acquireHookConfigMutationLock(context.Context, string) (io.Closer, error) {
	return nil, errHookConfigMutationUnsupported
}

func writeHookConfigAtomic(context.Context, string, []byte) error {
	return errHookConfigMutationUnsupported
}
