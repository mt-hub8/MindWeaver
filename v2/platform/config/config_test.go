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

func TestWriteNewAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", config.DefaultFileName)
	want := config.Default(filepath.Join(t.TempDir(), "vault"))

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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("configuration file is empty")
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
		"unknown field":  `{"schema_version":1,"vault":{"root":"vault","typo":true},"storage":{"driver":"sqlite","database_file":"data/db","busy_timeout":"1s","max_open_conns":1,"max_idle_conns":1,"connection_ttl":"0s"},"runtime":{"lock_file":"lock"}}`,
		"trailing value": `{"schema_version":1,"vault":{"root":"vault"},"storage":{"driver":"sqlite","database_file":"data/db","busy_timeout":"1s","max_open_conns":1,"max_idle_conns":1,"connection_ttl":"0s"},"runtime":{"lock_file":"lock"}} {}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			body = strings.ReplaceAll(body, `\"`, `"`)
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

func TestValidateRejectsEscapingPathsAndWrongVersion(t *testing.T) {
	cfg := config.Default("vault")
	cfg.Storage.DatabaseFile = filepath.Join("..", "outside.db")
	if err := cfg.Validate(); apperror.CodeOf(err) != "config.database_file_invalid" {
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

func TestExampleUsesPortableDurationEncoding(t *testing.T) {
	cfg := config.Default("vault")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.WriteNew(context.Background(), path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"busy_timeout": "5s"`) {
		t.Fatalf("unexpected duration encoding: %s", data)
	}
}
