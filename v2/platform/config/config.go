// Package config owns the versioned, transport-neutral runtime configuration.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

const (
	// CurrentSchemaVersion is incremented only when the on-disk contract changes.
	CurrentSchemaVersion = 1
	// DefaultFileName includes the schema version so incompatible files can coexist.
	DefaultFileName = "mindweaver.v1.json"
	maxConfigBytes  = 1 << 20
)

// Config is the version 1 configuration contract.
type Config struct {
	SchemaVersion int           `json:"schema_version"`
	Vault         VaultConfig   `json:"vault"`
	Storage       StorageConfig `json:"storage"`
	Runtime       RuntimeConfig `json:"runtime"`
}

type VaultConfig struct {
	Root string `json:"root"`
}

type StorageConfig struct {
	Driver        string   `json:"driver"`
	DatabaseFile  string   `json:"database_file"`
	BusyTimeout   Duration `json:"busy_timeout"`
	MaxOpenConns  int      `json:"max_open_conns"`
	MaxIdleConns  int      `json:"max_idle_conns"`
	ConnectionTTL Duration `json:"connection_ttl"`
}

type RuntimeConfig struct {
	LockFile string `json:"lock_file"`
}

// Duration uses Go duration strings on disk (for example "5s") while
// retaining a strongly typed value in memory.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("nil duration receiver")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("duration must be a string such as \"5s\"")
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}
	d.Duration = parsed
	return nil
}

// Default returns a complete v1 configuration for a vault root.
func Default(vaultRoot string) Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Vault: VaultConfig{
			Root: vaultRoot,
		},
		Storage: StorageConfig{
			Driver:        "sqlite",
			DatabaseFile:  filepath.Join("data", "mindweaver.db"),
			BusyTimeout:   Duration{Duration: 5 * time.Second},
			MaxOpenConns:  1,
			MaxIdleConns:  1,
			ConnectionTTL: Duration{},
		},
		Runtime: RuntimeConfig{
			LockFile: ".mindweaver.lock",
		},
	}
}

// Validate checks the complete on-disk contract before any component uses it.
func (c Config) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return apperror.New(apperror.KindInvalid, "config.unsupported_version", fmt.Sprintf("configuration schema_version must be %d", CurrentSchemaVersion))
	}
	if strings.TrimSpace(c.Vault.Root) == "" {
		return apperror.New(apperror.KindInvalid, "config.vault_root_required", "vault.root is required")
	}
	if c.Storage.Driver != "sqlite" {
		return apperror.New(apperror.KindInvalid, "config.storage_driver_unsupported", "storage.driver must be sqlite")
	}
	if err := validateRelativePath(c.Storage.DatabaseFile); err != nil {
		return apperror.Wrap(err, apperror.KindInvalid, "config.database_file_invalid", "config.validate", "storage.database_file must stay within the vault")
	}
	if c.Storage.BusyTimeout.Duration <= 0 || c.Storage.BusyTimeout.Duration > 5*time.Minute {
		return apperror.New(apperror.KindInvalid, "config.busy_timeout_invalid", "storage.busy_timeout must be between 1ns and 5m")
	}
	if c.Storage.MaxOpenConns != 1 || c.Storage.MaxIdleConns < 0 || c.Storage.MaxIdleConns > c.Storage.MaxOpenConns {
		return apperror.New(apperror.KindInvalid, "config.connection_pool_invalid", "SQLite requires max_open_conns=1 and max_idle_conns between 0 and 1")
	}
	if c.Storage.ConnectionTTL.Duration < 0 {
		return apperror.New(apperror.KindInvalid, "config.connection_ttl_invalid", "storage.connection_ttl cannot be negative")
	}
	if err := validateRelativePath(c.Runtime.LockFile); err != nil {
		return apperror.Wrap(err, apperror.KindInvalid, "config.lock_file_invalid", "config.validate", "runtime.lock_file must stay within the vault")
	}
	return nil
}

func validateRelativePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is empty")
	}
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return errors.New("path is absolute")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("path escapes root")
	}
	return nil
}

// Load reads and strictly validates a configuration file. Unknown fields and
// trailing JSON values are rejected so configuration typos cannot be ignored.
func Load(ctx context.Context, path string) (Config, error) {
	const op = "config.load"
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		kind := apperror.KindUnavailable
		code := "config.open_failed"
		public := "configuration could not be opened"
		if errors.Is(err, os.ErrNotExist) {
			kind = apperror.KindNotFound
			code = "config.not_found"
			public = "configuration file was not found"
		}
		return Config{}, apperror.Wrap(err, kind, code, op, public)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return Config{}, apperror.Wrap(err, apperror.KindUnavailable, "config.read_failed", op, "configuration could not be read")
	}
	if len(data) > maxConfigBytes {
		return Config{}, apperror.New(apperror.KindInvalid, "config.too_large", "configuration file exceeds 1 MiB")
	}
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, apperror.Wrap(err, apperror.KindInvalid, "config.invalid_json", op, "configuration is not valid versioned JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Config{}, apperror.Wrap(err, apperror.KindInvalid, "config.trailing_data", op, "configuration contains trailing data")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// WriteNew durably creates a new configuration without overwriting an
// existing file.
func WriteNew(ctx context.Context, path string, cfg Config) (err error) {
	const op = "config.write_new"
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return apperror.Wrap(err, apperror.KindInternal, "config.encode_failed", op, "configuration could not be encoded")
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.directory_failed", op, "configuration directory could not be created")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return apperror.Wrap(err, apperror.KindConflict, "config.already_exists", op, "configuration file already exists")
		}
		return apperror.Wrap(err, apperror.KindUnavailable, "config.create_failed", op, "configuration file could not be created")
	}
	complete := false
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = apperror.Wrap(closeErr, apperror.KindUnavailable, "config.close_failed", op, "configuration file could not be finalized")
		}
		if !complete {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.Write(data); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.write_failed", op, "configuration file could not be written")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "config.sync_failed", op, "configuration file could not be synchronized")
	}
	complete = true
	return nil
}
