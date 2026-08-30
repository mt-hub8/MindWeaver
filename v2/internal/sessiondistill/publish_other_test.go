//go:build !windows

package sessiondistill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishBundleFailsClosedBeforeWritingOnUnsupportedPlatform(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	err := PublishBundle(context.Background(), destination, testPublishedBundle(t))
	if CodeOf(err) != CodeUnsupportedPlatform {
		t.Fatalf("err=%v code=%s", err, CodeOf(err))
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("unsupported publication wrote files: entries=%v err=%v", entries, readErr)
	}
}
