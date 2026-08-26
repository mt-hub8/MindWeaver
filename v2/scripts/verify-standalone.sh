#!/usr/bin/env sh
set -eu

mw_source_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mw_repository_root=$(git -C "$mw_source_root" rev-parse --show-toplevel)
mw_source_prefix=$(git -C "$mw_source_root" rev-parse --show-prefix)
case "$mw_source_prefix" in
    '') mw_status_path=.; mw_treeish=HEAD ;;
    'v2/') mw_status_path=v2; mw_treeish=HEAD:v2 ;;
    *) printf 'standalone source must be repository root or tracked v2 tree, got: %s\n' "$mw_source_prefix" >&2; exit 1 ;;
esac
if [ -n "$(git -C "$mw_repository_root" status --porcelain=v1 --untracked-files=all -- "$mw_status_path")" ]; then
    printf 'tracked-only standalone verification requires a clean source worktree\n' >&2
    exit 1
fi

mw_extraction_root=$(mktemp -d "${TMPDIR:-/tmp}/mindweaver-v2-standalone.XXXXXX")
mw_standalone_root="$mw_extraction_root/source"
mkdir "$mw_standalone_root"
trap 'rm -rf -- "$mw_extraction_root"' EXIT HUP INT TERM

git -C "$mw_repository_root" archive "$mw_treeish" | tar -x -C "$mw_standalone_root"
[ -f "$mw_standalone_root/go.mod" ]
[ ! -e "$mw_standalone_root/.git" ]
MW_GO=${MW_GO:-go} "$mw_standalone_root/scripts/ci.sh"
