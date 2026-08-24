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

export GOTOOLCHAIN=local
export GOPROXY=off
export GOSUMDB=off
export GOFLAGS='-mod=readonly -buildvcs=false'
mw_build_root=$(mktemp -d "${TMPDIR:-/tmp}/mindweaver-v2-build.XXXXXX")
trap 'rm -rf -- "$mw_build_root"' EXIT HUP INT TERM

cd "$mw_module_root"
"$mw_go" version
mw_unformatted=$(find . -type f -name '*.go' -exec "$mw_gofmt" -l '{}' +)
if [ -n "$mw_unformatted" ]; then
    printf 'Go files require formatting:\n%s\n' "$mw_unformatted" >&2
    exit 1
fi
"$mw_go" test -count=1 ./...
"$mw_go" vet ./...
"$mw_go" build -trimpath -o "$mw_build_root/mindweaver" ./cmd/mindweaver
"$mw_go" build -trimpath -o "$mw_build_root/mindweaver-pdf" ./cmd/mindweaver-pdf
