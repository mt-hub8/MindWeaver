//go:build !windows

package ideashook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHookConfigMutationIsUnsupportedWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: filepath.Join(home, "mindweaver")}
	if _, _, err := RenderHookConfig(context.Background(), options); err == nil || CodeOf(err) != CodeUnsupported {
		t.Fatalf("render err=%v code=%s", err, CodeOf(err))
	}
	if _, err := InstallHookConfig(context.Background(), options); err == nil || CodeOf(err) != CodeUnsupported {
		t.Fatalf("install err=%v code=%s", err, CodeOf(err))
	}
	if _, err := os.Stat(filepath.Join(home, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported mutation wrote hooks.json: %v", err)
	}
}
