package sessiondistill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type publishOps struct {
	writeFile     func(string, []byte) error
	syncDirectory func(string) error
	rename        func(string, string) error
}

func PublishBundle(ctx context.Context, destination string, bundle Bundle) error {
	if err := canceled(ctx); err != nil {
		return err
	}
	if destination == "" || len(bundle.json) == 0 || len(bundle.markdown) == 0 ||
		len(bundle.json) > maxRenderedBytes || len(bundle.markdown) > maxRenderedBytes || !validBundle(bundle) {
		return invalid()
	}
	if !atomicDirectoryPublicationSupported {
		return newError(CodeUnsupportedPlatform, errPublishUnsupported)
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	guard, err := retainPublicationParent(filepath.Dir(absolute))
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	defer guard.Close()
	return publishBundlePlatform(ctx, absolute, bundle, guard)
}

func publishBundleWithOps(ctx context.Context, destination string, bundle Bundle, ops publishOps) error {
	if err := canceled(ctx); err != nil {
		return err
	}
	if destination == "" || len(bundle.json) == 0 || len(bundle.markdown) == 0 ||
		len(bundle.json) > maxRenderedBytes || len(bundle.markdown) > maxRenderedBytes ||
		!validBundle(bundle) || ops.writeFile == nil || ops.syncDirectory == nil || ops.rename == nil {
		return invalid()
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	parent, name := filepath.Dir(absolute), filepath.Base(absolute)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return invalid()
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return newError(CodeOutputFailed, errors.Join(err, fmt.Errorf("destination parent")))
	}
	staging, err := os.MkdirTemp(parent, ".mindweaver-ideas-")
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	stagingOwned := true
	defer func() {
		if stagingOwned {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := ops.writeFile(filepath.Join(staging, "report.json"), bundle.json); err != nil {
		return newError(CodeOutputFailed, err)
	}
	if err := canceled(ctx); err != nil {
		return err
	}
	if err := ops.writeFile(filepath.Join(staging, "report.md"), bundle.markdown); err != nil {
		return newError(CodeOutputFailed, err)
	}
	if err := ops.syncDirectory(staging); err != nil {
		return newError(CodeOutputFailed, err)
	}
	if err := canceled(ctx); err != nil {
		return err
	}
	if err := ops.rename(staging, absolute); err != nil {
		if errors.Is(err, errPublishUnsupported) {
			return newError(CodeUnsupportedPlatform, err)
		}
		if errors.Is(err, os.ErrExist) {
			return newError(CodeDestinationExists, err)
		}
		return newError(CodeOutputFailed, err)
	}
	stagingOwned = false
	if err := ops.syncDirectory(parent); err != nil {
		return newError(CodePublicationUncertain, err)
	}
	return nil
}

func writeBundleFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := io.Copy(file, bytesReader(content))
	if writeErr == nil && written != int64(len(content)) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

type byteReader struct {
	content []byte
	offset  int
}

func bytesReader(content []byte) *byteReader { return &byteReader{content: content} }

func (reader *byteReader) Read(output []byte) (int, error) {
	if reader.offset == len(reader.content) {
		return 0, io.EOF
	}
	written := copy(output, reader.content[reader.offset:])
	reader.offset += written
	return written, nil
}
