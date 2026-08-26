#!/usr/bin/env sh
set -eu

mw_module_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mw_go_input=${MW_GO:-go}
case "$mw_go_input" in
    */*) mw_go=$mw_go_input ;;
    *) mw_go=$(command -v "$mw_go_input") ;;
esac
mw_go_dir=$(CDPATH= cd -- "$(dirname -- "$mw_go")" && pwd)
mw_go="$mw_go_dir/$(basename -- "$mw_go")"
if [ ! -f "$mw_go" ] || [ ! -x "$mw_go" ]; then
    printf 'Go executable is not an executable regular file: %s\n' "$mw_go" >&2
    exit 1
fi
export MW_GO="$mw_go"
mw_go_dir=$(dirname -- "$mw_go")
mw_gofmt="$mw_go_dir/gofmt"
if [ ! -x "$mw_gofmt" ]; then
    mw_gofmt=$(command -v gofmt)
fi

mw_build_root=$(mktemp -d "${TMPDIR:-/tmp}/mindweaver-v2-build.XXXXXX")
trap 'rm -rf -- "$mw_build_root"' EXIT HUP INT TERM
mkdir "$mw_build_root/gomodcache" "$mw_build_root/gocache" "$mw_build_root/gotmp"

export CGO_ENABLED=0
export GOARCH=amd64
export GOAMD64=v1
export GOENV=off
export GOEXPERIMENT=
export GOFIPS140=off
export GOFLAGS='-mod=vendor -trimpath -buildvcs=false'
export GOCACHE="$mw_build_root/gocache"
export GOMODCACHE="$mw_build_root/gomodcache"
export GOOS=windows
export GOPROXY=off
export GOSUMDB=off
export GOTOOLCHAIN=local
export GOTELEMETRY=off
export GOTMPDIR="$mw_build_root/gotmp"
export GOVCS='*:off'
export GOWORK=off

cd "$mw_module_root"
mw_go_version=$("$mw_go" version)
case "$mw_go_version" in
    'go version go1.27.0 '*) ;;
    *) printf 'unsupported Go toolchain: %s\n' "$mw_go_version" >&2; exit 1 ;;
esac
printf '%s\n' "$mw_go_version"
mw_unformatted=$(find . -path ./vendor -prune -o -type f -name '*.go' -exec "$mw_gofmt" -l '{}' +)
if [ -n "$mw_unformatted" ]; then
    printf 'Go files require formatting:\n%s\n' "$mw_unformatted" >&2
    exit 1
fi
"$mw_go" test -count=1 ./...
"$mw_go" vet ./...
"$mw_go" build -trimpath -buildvcs=false -o "$mw_build_root/mindweaver.exe" ./cmd/mindweaver
"$mw_go" build -trimpath -buildvcs=false -o "$mw_build_root/mindweaver-pdf.exe" ./cmd/mindweaver-pdf
if find "$GOMODCACHE" -mindepth 1 -print -quit | grep -q .; then
    printf 'vendored build wrote to the empty module cache\n' >&2
    exit 1
fi
