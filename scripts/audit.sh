#!/usr/bin/env bash
# Pre-publication audit of the current index, every revision and build artifacts.
# Optional unwanted expressions belong in AUDIT_TERMS_FILE outside the checkout.
# Findings name files or commits only: matched private contents are never printed.
# Usage: scripts/audit.sh [artifact-file-or-dir ...]
set -u
export LC_ALL=C.UTF-8
cd "$(git rev-parse --show-toplevel)" || exit 1

fail=0
report() { # report <check> <file-or-commit-findings>
	if [ -n "$2" ]; then
		echo "FAIL  $1"
		printf '%s\n' "$2" | sed -E -f "$scratch/redactions" | head -n 40 | sed 's/^/      /'
		fail=1
	else
		echo "ok    $1"
	fi
}

scratch=$(mktemp -d) || exit 1
trap 'rm -rf "$scratch"' EXIT
secrets=$scratch/secrets
cat >"$secrets" <<'PATTERNS'
-----BEGIN [A-Z ]*PRIVATE KEY-----
\bgh[pousr]_[A-Za-z0-9]{36,}
\bgithub_pat_[A-Za-z0-9_]{40,}
\bAKIA[0-9A-Z]{16}\b
\bAIza[0-9A-Za-z_-]{35}\b
\bxox[abprs]-[A-Za-z0-9-]{10,}
\bsk-[A-Za-z0-9]{32,}
"(client_secret|clientSecret|refresh_token|access_token)"[[:space:]]*:[[:space:]]*"[^"]{8,}"
MNELAB_SIGNING_KEY=[A-Za-z0-9+/=]{20,}
PATTERNS
# Even an identifier can itself contain a credential-shaped value.
sed -e 's|^|s~|' -e 's|$|~[redacted]~gI|' "$secrets" >"$scratch/redactions"

# Private categories are rejected regardless of their contents. Generic public
# documentation such as recovery.md is intentionally outside these categories.
private_re='(^|/)(\.env(\..*)?|[^/]*\.pem|[^/]*\.key|release-signing[^/]*|providers\.json|AGENTS\.md|CLAUDE\.md|Zeta\.xlsx)$|(^|/)(C-(RT|SR)|F-(RT|SR)|\.private-fixtures|\.dev-data)(/|$)|(^|/)[^/]*pacote[-_ .]*mestre[^/]*(/|$)|(^|/)[^/]*(prompt[-_ .]*mestre|mestre[-_ .]*prompt)[^/]*$|(^|/)[^/]*(master[-_ .]*(continuity|recovery|prompt)|continuity[-_ .]*(recovery[-_ .]*)?prompt|recovery[-_ .]*prompt)[^/]*$|(^|/)[^/]*((claude|codex)[-_ .]*(coordination|transcript|conversation|handoff|chat|session|prompt)|(coordination|transcript|conversation|handoff|chat|session|prompt)[-_ .]*(claude|codex))[^/]*$|(^|/)[^/]*(claude[-_ .]*codex|codex[-_ .]*claude)[^/]*$|(^|/)example[-_ .]*results\.dts$|(^|/)[^/]*pacote[-_ .]*implementa(cao|ção)[^/]*(/|$)'
private_content=$scratch/private-content
cat >"$private_content" <<'PATTERNS'
^#{1,6}[[:space:]]+.*master[[:space:]_-]+continuity[[:space:][:punct:]]+recovery[[:space:]_-]+prompt
^[[:space:]]*master[[:space:]_-]+continuity[[:space:][:punct:]]+recovery[[:space:]_-]+prompt([[:space:]]|$)
^#{1,6}[[:space:]]+.*prompt[[:space:]_-]+mestre([[:space:]]|$)
^#{1,6}[[:space:]]*((claude|codex)[[:space:][:punct:]]+){1,2}(coordination|transcript|conversation|handoff|session|chat|instructions)
PATTERNS

git rev-list --all >"$scratch/revisions" || exit 1
if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
	report "complete Git history is available" "shallow checkout: fetch the complete history before publication"
else
	report "complete Git history is available" ""
fi
: >"$scratch/history-names"
: >"$scratch/private-content-paths"
while IFS= read -r rev; do
	git ls-tree -r --name-only "$rev" >>"$scratch/history-names" || exit 1
	git grep -I -l -i -E -f "$private_content" "$rev" 2>/dev/null | sed "s/^$rev://" >>"$scratch/private-content-paths"
done <"$scratch/revisions"
sort -u "$scratch/history-names" >"$scratch/all-names"
grep -i -E "$private_re" "$scratch/all-names" >"$scratch/private-paths" || :
# Retain historical basenames even after deletion from the current branch.
cat "$scratch/private-paths" "$scratch/private-content-paths" | sed 's|.*/||' | sort -u >"$scratch/private-names"

git_matches() {
	git grep "$@" 2>"$scratch/grep-errors"
	local result=$?
	if [ "$result" -gt 1 ] || [ -s "$scratch/grep-errors" ]; then printf '%s\n' '[file content inspection failed]'; fi
}
scan_tree() {
	{ git_matches --cached -I -l -i -E -f "$1"; git_matches -I -l -i -E -f "$1"; } | sort -u
}
scan_names() { git ls-files | grep -i -E -f "$1"; }
scan_history_files() {
	while IFS= read -r rev; do
		git_matches -I -l -i -E -f "$1" "$rev"
	done <"$scratch/revisions" | sort -u
}
scan_history_meta() {
	while IFS= read -r rev; do
		if ! git show -s --format='%an <%ae>%n%cn <%ce>%n%B%n' "$rev" >"$scratch/metadata" 2>/dev/null; then
			printf '%s: [metadata inspection failed]\n' "$rev"
		elif grep -i -q -E -f "$1" "$scratch/metadata"; then
			printf '%s\n' "$rev"
		fi
	done <"$scratch/revisions"
}

report "no private files tracked" "$(git ls-files | grep -i -E "$private_re")"
report "no private file names in history" "$(cat "$scratch/private-paths")"
report "no private coordination content tracked" "$(scan_tree "$private_content")"
report "no private coordination content in history" "$(scan_history_files "$private_content")"
report "secrets: tracked files" "$(scan_tree "$secrets")"
report "secrets: tracked file names" "$(scan_names "$secrets")"
report "secrets: history" "$(scan_history_files "$secrets")"
report "secrets: historical file names" "$(grep -i -E -f "$secrets" "$scratch/all-names")"
report "secrets: commit metadata and trailers" "$(scan_history_meta "$secrets")"

terms=""
if [ -n "${AUDIT_TERMS_FILE:-}" ]; then
	if [ ! -r "$AUDIT_TERMS_FILE" ] || [ ! -s "$AUDIT_TERMS_FILE" ]; then
		report "unwanted terms file is readable and nonempty" "$AUDIT_TERMS_FILE"
	else
		grep -E -f "$AUDIT_TERMS_FILE" /dev/null >/dev/null 2>&1
		pattern_status=$?
		if [ "$pattern_status" -gt 1 ]; then
			report "unwanted term expressions are valid" "$AUDIT_TERMS_FILE"
		else
			terms=$AUDIT_TERMS_FILE
			report "terms: tracked files (1)" "$(scan_tree "$terms")"
			report "terms: file names (1)" "$(scan_names "$terms")"
			report "terms: every revision (2)" "$(scan_history_files "$terms")"
			report "terms: historical file names (2)" "$(grep -i -E -f "$terms" "$scratch/all-names")"
			report "terms: commit metadata and trailers (2)" "$(scan_history_meta "$terms")"
		fi
	fi
else
	echo "skip  unwanted terms (AUDIT_TERMS_FILE not set)"
fi

text_runs() { strings -a -e S -n 6 "$1" && strings -a -e l -n 6 "$1"; }
inspect_zip() {
	# Python's standard library handles literal member names, nesting, checksums
	# and empty archives. Limits fail the audit rather than skipping any content.
	python3 - "$1" "$scratch/zip-names" "$scratch/zip-data" <<'PY'
import io
import pathlib
import sys
import zipfile

limit = 512 * 1024 * 1024
total = 0
try:
    source = pathlib.Path(sys.argv[1])
    if source.stat().st_size > limit:
        raise ValueError("archive size limit")
    with open(sys.argv[2], "wb") as names, open(sys.argv[3], "wb") as data:
        def inspect(raw, depth=0):
            global total
            if depth > 16:
                raise ValueError("archive nesting limit")
            with zipfile.ZipFile(io.BytesIO(raw)) as archive:
                for entry in archive.infolist():
                    names.write(entry.filename.encode("utf-8") + b"\n")
                    if entry.is_dir():
                        continue
                    total += entry.file_size
                    if total > limit:
                        raise ValueError("archive expansion limit")
                    content = archive.read(entry)
                    data.write(b"\n" + content + b"\n")
                    if content.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")) or entry.filename.lower().endswith((".zip", ".xlsx")):
                        inspect(content, depth + 1)
        inspect(source.read_bytes())
except (OSError, ValueError, RuntimeError, NotImplementedError, zipfile.BadZipFile):
    sys.exit(1)
PY
}
private_basename_text() {
	[ -s "$scratch/private-names" ] && grep -a -i -q -F -f "$scratch/private-names" "$1"
}
private_entry_names() {
	if grep -a -i -q -E "$private_re" "$1"; then return 0; fi
	private_basename_text "$1"
}
known='GetLargePage[M]inimum'
if [ -n "$terms" ]; then
	# Use explicit text boundaries to avoid accidental minified-symbol matches.
	sed -E -e 's/^\\b/(^|[[:space:]"'"'"'>])/' -e 's/\\b$/([[:space:]"'"'"'<.,;:!?)-]|$)/' "$terms" >"$scratch/artifact-terms"
fi

private_artifacts="" secret_artifacts="" term_artifacts="" path_artifacts="" unreadable_artifacts=""
if [ "$#" -gt 0 ]; then
	if ! command -v strings >/dev/null 2>&1; then
		report "artifact text inspection is available" "strings is required"
		exit "$fail"
	fi
	build_home=${HOME:-/nonexistent}
	for p in "$@"; do
		if [ ! -e "$p" ]; then unreadable_artifacts+="$p"$'\n'; continue; fi
		if ! find -L "$p" -type f -print0 >"$scratch/artifact-files" 2>/dev/null; then unreadable_artifacts+="$p"$'\n'; fi
		while IFS= read -r -d '' f; do
			if [ ! -r "$f" ]; then unreadable_artifacts+="$f"$'\n'; continue; fi
			printf '%s\n' "$f" >"$scratch/entry-names"
			if ! text_runs "$f" >"$scratch/artifact-text" 2>/dev/null; then unreadable_artifacts+="$f"$'\n'; continue; fi
			# Inspect ZIP entries and decompressed bytes without extracting files.
			magic=$(od -An -tx1 -N4 "$f" | tr -d ' \n')
			case "$magic:$f" in
				504b0304:*|504b0506:*|504b0708:*|*:*.zip|*:*.ZIP|*:*.xlsx|*:*.XLSX)
					if ! command -v python3 >/dev/null 2>&1 || ! inspect_zip "$f"; then
						unreadable_artifacts+="$f"$'\n'
					else
						cat "$scratch/zip-names" >>"$scratch/entry-names"
						if ! text_runs "$scratch/zip-data" >>"$scratch/artifact-text" 2>/dev/null; then unreadable_artifacts+="$f"$'\n'; fi
						if grep -a -q -F "$build_home/" "$scratch/zip-data"; then path_artifacts+="$f"$'\n'; fi
					fi
					;;
			esac
			if private_entry_names "$scratch/entry-names" || private_basename_text "$scratch/artifact-text" || grep -a -i -q -E -f "$private_content" "$scratch/artifact-text"; then private_artifacts+="$f"$'\n'; fi
			if grep -a -i -q -E -f "$secrets" "$scratch/artifact-text" || grep -a -i -q -E -f "$secrets" "$scratch/entry-names"; then secret_artifacts+="$f"$'\n'; fi
			if [ -n "$terms" ] && sed -E "s/($known)//g" "$scratch/artifact-text" | grep -a -i -q -E -f "$scratch/artifact-terms"; then term_artifacts+="$f"$'\n'; fi
			if grep -a -q -F "$build_home/" "$f"; then path_artifacts+="$f"$'\n'; fi
		done <"$scratch/artifact-files"
	done
	report "artifacts can be inspected" "${unreadable_artifacts%$'\n'}"
	report "no private file names in artifacts or ZIP entries" "${private_artifacts%$'\n'}"
	report "secrets: built artifacts" "${secret_artifacts%$'\n'}"
	if [ -n "$terms" ]; then report "terms: built artifacts (3)" "${term_artifacts%$'\n'}"; fi
	report "no build-machine paths in artifacts" "$(printf '%s' "$path_artifacts" | sort -u)"
elif [ -n "$terms" ]; then
	echo "skip  terms: built artifacts (3) (no artifacts given)"
fi

exit "$fail"
