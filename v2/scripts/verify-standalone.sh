#!/usr/bin/env sh
set -eu

mw_source_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mw_standalone_root=$(mktemp -d "${TMPDIR:-/tmp}/mindweaver-v2-standalone.XXXXXX")
trap 'rm -rf -- "$mw_standalone_root"' EXIT HUP INT TERM

cp -R "$mw_source_root/." "$mw_standalone_root/"
MW_GO=${MW_GO:-go} "$mw_standalone_root/scripts/ci.sh"
