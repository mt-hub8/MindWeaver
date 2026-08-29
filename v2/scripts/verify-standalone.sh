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
mw_tree_entries="$mw_extraction_root/tree.entries"
mw_tracked_unsorted="$mw_extraction_root/tracked.unsorted"
mw_tracked_paths="$mw_extraction_root/tracked.paths"
mw_extracted_paths="$mw_extraction_root/extracted.paths"
mw_archive="$mw_extraction_root/source.tar"
mkdir "$mw_standalone_root"
trap 'rm -rf -- "$mw_extraction_root"' EXIT HUP INT TERM

git -C "$mw_repository_root" ls-tree -r "$mw_treeish" >"$mw_tree_entries"
mw_ci_mode=
mw_standalone_mode=
: >"$mw_tracked_unsorted"
while IFS="$(printf '\t')" read -r mw_metadata mw_path; do
    set -- $mw_metadata
    [ "$#" -eq 3 ] && [ "$2" = blob ] || { printf 'unsupported tracked tree entry: %s\t%s\n' "$mw_metadata" "$mw_path" >&2; exit 1; }
    case "$1" in
        100644|100755) ;;
        *) printf 'unsupported tracked mode for %s: %s\n' "$mw_path" "$1" >&2; exit 1 ;;
    esac
    case "$mw_path" in
        scripts/ci.sh) mw_ci_mode=$1 ;;
        scripts/verify-standalone.sh) mw_standalone_mode=$1 ;;
        *) [ "$1" != 100755 ] || { printf 'unexpected tracked executable: %s\n' "$mw_path" >&2; exit 1; } ;;
    esac
    printf '%s\n' "$mw_path" >>"$mw_tracked_unsorted"
done <"$mw_tree_entries"
sort "$mw_tracked_unsorted" >"$mw_tracked_paths"
[ "$mw_ci_mode" = 100755 ] && [ "$mw_standalone_mode" = 100755 ] || {
    printf 'tracked executable exact-set mismatch\n' >&2
    exit 1
}

git -C "$mw_repository_root" archive --format=tar --output="$mw_archive" "$mw_treeish"
tar -xf "$mw_archive" -C "$mw_standalone_root"
[ -f "$mw_standalone_root/go.mod" ]
[ ! -e "$mw_standalone_root/.git" ]
find "$mw_standalone_root" -type f -print |
    sed "s#^$mw_standalone_root/##" |
    sort >"$mw_extracted_paths"
cmp "$mw_tracked_paths" "$mw_extracted_paths"
MW_GO=${MW_GO:-go} "$mw_standalone_root/scripts/ci.sh"
