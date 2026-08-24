package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
)

func TestWriteLoadAndResolveVault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", config.DefaultFileName)
	want := config.Default(filepath.Join("..", "vault"))

	if err := config.WriteNew(context.Background(), path, want); err != nil {
		t.Fatalf("WriteNew() error = %v", err)
	}
	got, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	resolved, err := config.ResolveVault(path, got)
	if err != nil {
		t.Fatalf("ResolveVault() error = %v", err)
	}
	if wantPath := filepath.Join(root, "vault"); resolved != wantPath {
		t.Fatalf("ResolveVault() = %q, want %q", resolved, wantPath)
	}
}

func TestWriteNewRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.DefaultFileName)
	cfg := config.Default("vault")
	if err := config.WriteNew(context.Background(), path, cfg); err != nil {
		t.Fatal(err)
	}
	err := config.WriteNew(context.Background(), path, cfg)
	if !apperror.IsKind(err, apperror.KindConflict) || apperror.CodeOf(err) != "config.already_exists" {
		t.Fatalf("WriteNew() error = %v", err)
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	tests := map[string]string{
		"unknown field":                  `{"schema_version":1,"vault":{"root":"vault","typo":true}}`,
		"trailing value":                 `{"schema_version":1,"vault":{"root":"vault"}} {}`,
		"case alias":                     `{"SCHEMA_VERSION":1,"vault":{"root":"vault"}}`,
		"case alias alongside canonical": `{"schema_version":1,"SCHEMA_VERSION":999,"vault":{"root":"vault"}}`,
		"missing version":                `{"vault":{"root":"vault"}}`,
		"null vault":                     `{"schema_version":1,"vault":null}`,
		"nested case alias":              `{"schema_version":1,"vault":{"ROOT":"vault"}}`,
		"missing nested root":            `{"schema_version":1,"vault":{}}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(context.Background(), path)
			if !apperror.IsKind(err, apperror.KindInvalid) {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	tests := map[string]string{
		"root":   `{"schema_version":1,"schema_version":1,"vault":{"root":"vault"}}`,
		"nested": `{"schema_version":1,"vault":{"root":"first","root":"second"}}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(context.Background(), path)
			if apperror.CodeOf(err) != "config.duplicate_key" {
				t.Fatalf("Load() error = %v; want config.duplicate_key", err)
			}
		})
	}
}

func TestValidateRejectsEmptyRootAndWrongVersion(t *testing.T) {
	cfg := config.Default("   ")
	if err := cfg.Validate(); apperror.CodeOf(err) != "config.vault_root_required" {
		t.Fatalf("Validate() error = %v", err)
	}
	cfg = config.Default("vault")
	cfg.SchemaVersion++
	if err := cfg.Validate(); apperror.CodeOf(err) != "config.unsupported_version" {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestIOHonorsAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "config.json")

	if err := config.WriteNew(ctx, path, config.Default("vault")); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteNew() error = %v, want context.Canceled", err)
	}
	if _, err := config.Load(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled write created file: %v", err)
	}
}

func TestEncodingContainsOnlyRealChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.WriteNew(context.Background(), path, config.Default("vault")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, absent := range []string{"max_open_conns", "database_file", "lock_file", "busy_timeout"} {
		if strings.Contains(text, absent) {
			t.Fatalf("implementation option %q leaked into config: %s", absent, text)
		}
	}
}
