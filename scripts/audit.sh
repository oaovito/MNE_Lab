#!/usr/bin/env bash
# Pre-publication audit. Every check must come back clean before a push or a
# release. Three independent sources are examined:
#
#   1. the tracked files and their names (git's own index search);
#   2. the whole history: every revision's files, every commit's author,
#      committer, message and trailers;
#   3. the built artifacts, read as text runs without git (binaries, the
#      embedded interface bundle, release packages).
#
# Two lists are searched:
#   - unwanted terms: one case-insensitive extended regular expression per
#     line, in the file named by AUDIT_TERMS_FILE (kept outside the
#     repository);
#   - secrets and private material (built in below).
#
# Usage: scripts/audit.sh [artifact-file-or-dir ...]
set -u
# Accented terms only match under a UTF-8 locale.
export LC_ALL=C.UTF-8
cd "$(git rev-parse --show-toplevel)"

fail=0
report() { # report <check> <findings>
	if [ -n "$2" ]; then
		echo "FAIL  $1"
		printf '%s\n' "$2" | head -n 40 | sed 's/^/      /'
		fail=1
	else
		echo "ok    $1"
	fi
}

secrets=$(mktemp)
trap 'rm -f "$secrets"' EXIT
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

# System and runtime names (as regular expressions) that contain a term by
# chance; removed before built files are searched.
known='GetLargePage[M]inimum'

scan_tree() { git grep -I -n -i -E -f "$1" 2>/dev/null; }
scan_names() { git ls-files | grep -i -E -f "$1"; }
scan_history_files() {
	for rev in $(git rev-list --all); do
		git grep -I -l -i -E -f "$1" "$rev" 2>/dev/null
	done | sort -u
}
scan_history_meta() { git log --all --format='%H%n%an <%ae>%n%cn <%ce>%n%B%n' | grep -n -i -E -f "$1"; }

# Private material must never be tracked, whatever its content.
report "no private files tracked" "$(git ls-files | grep -i -E '(^|/)(\.env(\..*)?|.*\.pem|.*\.key|release-signing.*|providers\.json)$')"

report "secrets: tracked files" "$(scan_tree "$secrets")"
report "secrets: history" "$(scan_history_files "$secrets")"

if [ -n "${AUDIT_TERMS_FILE:-}" ] && [ -s "$AUDIT_TERMS_FILE" ]; then
	terms=$AUDIT_TERMS_FILE
	report "terms: tracked files (1)" "$(scan_tree "$terms")"
	report "terms: file names (1)" "$(scan_names "$terms")"
	report "terms: every revision (2)" "$(scan_history_files "$terms")"
	report "terms: commit metadata and trailers (2)" "$(scan_history_meta "$terms")"
	if [ "$#" -gt 0 ]; then
		# Built files are read as text runs (8-bit and UTF-16, so Windows
		# resources are included). In them a short word also turns up inside
		# minified names, file-type tables and compressed data, so there a
		# word-bounded term (\b) only counts where text does: after a space,
		# a quote or a tag, and before a space, punctuation or a tag.
		art=$(mktemp)
		sed -E -e 's/^\\b/(^|[[:space:]"'"'"'`>])/' -e 's/\\b$/([[:space:]"'"'"'`<.,;:!?)-]|$)/' "$terms" >"$art"
		found=""
		for p in "$@"; do
			[ -e "$p" ] || continue
			while IFS= read -r -d '' f; do
				if { strings -a -e S -n 6 "$f"; strings -a -e l -n 6 "$f"; } | sed -E "s/($known)//g" | grep -a -q -i -E -f "$art"; then found+="$f"$'\n'; fi
			done < <(find "$p" -type f -print0)
		done
		rm -f "$art"
		report "terms: built artifacts (3)" "${found%$'\n'}"
	else
		echo "skip  terms: built artifacts (3) (no artifacts given)"
	fi
else
	echo "skip  unwanted terms (AUDIT_TERMS_FILE not set)"
fi

# Builds must not carry the build machine's paths.
if [ "$#" -gt 0 ]; then
	home=${HOME:-/nonexistent}
	found=""
	for p in "$@"; do
		[ -e "$p" ] || continue
		while IFS= read -r -d '' f; do
			if grep -a -q -F "$home/" "$f"; then found+="$f"$'\n'; fi
		done < <(find "$p" -type f -print0)
	done
	report "no build-machine paths in artifacts" "${found%$'\n'}"
fi

exit $fail
