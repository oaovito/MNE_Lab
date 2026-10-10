#!/usr/bin/env bash
# Synthetic privacy-audit regression cases. All fixtures stay outside the checkout.
set -euo pipefail
audit=$(cd "$(dirname "$0")" && pwd)/audit.sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
for command_name in git strings zip python3; do
	command -v "$command_name" >/dev/null || { echo "missing test prerequisite: $command_name"; exit 1; }
done

case_number=0
test_terms=""
new_repo() {
	case_number=$((case_number + 1))
	repo=$scratch/repo-$case_number
	log=$scratch/log-$case_number
	mkdir "$repo"
	git -C "$repo" init -q -b main
	git -C "$repo" config user.name 'Audit fixture'
	git -C "$repo" config user.email 'audit@example.invalid'
	printf 'Public synthetic fixture.\n' >"$repo/README.md"
}
commit() { git -C "$repo" add -A; git -C "$repo" commit -qm "$1"; }
run_audit() {
	status=0
	(cd "$repo" && AUDIT_TERMS_FILE="$test_terms" bash "$audit" "$@") >"$log" 2>&1 || status=$?
}
expect_status() {
	if [ "$status" -ne "$1" ]; then echo "FAIL: $2 (unexpected audit exit $status)"; exit 1; fi
}
expect_line() {
	if ! grep -q -F "$1" "$log"; then echo "FAIL: missing expected check: $1"; exit 1; fi
}
pass() { echo "ok    $1"; }

new_repo
mkdir "$repo/docs"
printf 'Public recovery documentation.\n' >"$repo/docs/recovery.md"
commit 'Public documentation'
printf 'Harmless public build bytes.\n' >"$scratch/release.bin"
run_audit "$scratch/release.bin"
expect_status 0 'clean repository and artifact'
pass 'clean public files and generic recovery documentation pass'

git clone -q --depth 1 "file://$repo" "$scratch/shallow"
repo=$scratch/shallow
run_audit
expect_status 1 'incomplete shallow history'
expect_line 'FAIL  complete Git history is available'
pass 'a shallow checkout cannot certify complete history'

new_repo
commit 'Synthetic repository integrity fixture'
blob=$(git -C "$repo" rev-parse HEAD:README.md)
rm "$repo/.git/objects/${blob:0:2}/${blob:2}"
run_audit
expect_status 1 'unreadable committed content'
expect_line '[file content inspection failed]'
pass 'unreadable Git objects cannot silently pass content checks'

new_repo
for group in C-RT C-SR F-RT F-SR; do
	mkdir "$repo/$group"
	printf 'Synthetic placeholder, no experimental measurements.\n' >"$repo/$group/sample-record.txt"
done
for name in Zeta.xlsx AGENTS.md CLAUDE.md MASTER_CONTINUITY_RECOVERY_PROMPT.txt Codex_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts; do
	printf 'Synthetic private-category placeholder.\n' >"$repo/$name"
done
commit 'Synthetic private filename categories'
run_audit
expect_status 1 'tracked private filenames'
expect_line 'FAIL  no private files tracked'
for name in C-RT/sample-record.txt C-SR/sample-record.txt F-RT/sample-record.txt F-SR/sample-record.txt Zeta.xlsx AGENTS.md CLAUDE.md MASTER_CONTINUITY_RECOVERY_PROMPT.txt Codex_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts; do expect_line "$name"; done
pass 'all known data and coordination filename categories are rejected'
git -C "$repo" rm -rq -- C-RT C-SR F-RT F-SR Zeta.xlsx AGENTS.md CLAUDE.md MASTER_CONTINUITY_RECOVERY_PROMPT.txt Codex_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts
commit 'Remove synthetic private fixtures'
run_audit
expect_status 1 'deleted private filenames remain in history'
expect_line 'ok    no private files tracked'
expect_line 'FAIL  no private file names in history'
pass 'deleting current files cannot conceal private history'

printf 'sample-record.txt\n' >"$scratch/release.bin"
run_audit "$scratch/release.bin"
expect_status 1 'historical basename in artifact'
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'artifacts retain the historical private-basename check'

mkdir -p "$scratch/archive/renamed"
printf 'Synthetic harmless bytes.\n' >"$scratch/archive/renamed/sample-record.txt"
(cd "$scratch/archive" && zip -q "$scratch/renamed.zip" renamed/sample-record.txt)
run_audit "$scratch/renamed.zip"
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'ZIP entries are checked even after their original directory is renamed'

new_repo
# Construct the synthetic token at runtime so this test source has no token.
secret_prefix=ghp
synthetic_secret="${secret_prefix}_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
printf '%s\n' "$synthetic_secret" >"$repo/public-name.txt"
commit "Synthetic metadata $synthetic_secret"
run_audit
expect_status 1 'secret in tracked file and metadata'
expect_line 'FAIL  secrets: tracked files'
expect_line 'FAIL  secrets: history'
expect_line 'FAIL  secrets: commit metadata and trailers'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed matched secret contents'; exit 1; fi
git -C "$repo" rm -q public-name.txt
commit 'Delete synthetic token file'
run_audit
expect_status 1 'secret retained in history'
expect_line 'ok    secrets: tracked files'
expect_line 'FAIL  secrets: history'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a historical secret'; exit 1; fi
pass 'secret file and metadata findings never print matched values'

new_repo
commit 'Clean source'
printf '%s\n' "$synthetic_secret" >"$repo/staged-only.txt"
git -C "$repo" add staged-only.txt
printf 'Clean working-tree bytes.\n' >"$repo/staged-only.txt"
run_audit
expect_status 1 'index-only secret'
expect_line 'FAIL  secrets: tracked files'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a staged secret'; exit 1; fi
pass 'staged index contents are checked independently of working-tree bytes'

new_repo
printf 'Harmless synthetic bytes.\n' >"$repo/$synthetic_secret.txt"
commit 'Synthetic credential-shaped identifier'
run_audit
expect_status 1 'secret-shaped filename'
expect_line 'FAIL  secrets: tracked file names'
expect_line 'FAIL  secrets: historical file names'
expect_line '[redacted].txt'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a secret-shaped filename'; exit 1; fi
pass 'credential-shaped filenames are rejected and redacted in reports'

new_repo
commit 'Clean source'
mkdir "$scratch/secret-archive"
printf '%s\n' "$synthetic_secret" >"$scratch/secret-archive/public.txt"
(cd "$scratch/secret-archive" && zip -q "$scratch/secret.zip" public.txt)
run_audit "$scratch/secret.zip"
expect_status 1 'secret in compressed artifact contents'
expect_line 'FAIL  secrets: built artifacts'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a compressed secret'; exit 1; fi
pass 'compressed artifact contents are checked without exposing values'

mkdir "$scratch/nested-archive"
cp "$scratch/secret.zip" "$scratch/nested-archive/public.zip"
(cd "$scratch/nested-archive" && zip -q "$scratch/nested.zip" public.zip)
run_audit "$scratch/nested.zip"
expect_status 1 'secret in nested compressed contents'
expect_line 'FAIL  secrets: built artifacts'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a nested compressed secret'; exit 1; fi
pass 'nested archives are inspected recursively'

printf '%s\n' "$synthetic_secret" >"$scratch/actual.bin"
ln -s "$scratch/actual.bin" "$scratch/latest.bin"
run_audit "$scratch/latest.bin"
expect_status 1 'secret behind artifact symlink'
expect_line 'FAIL  secrets: built artifacts'
pass 'artifact symlinks cannot skip secret inspection'

printf 'Harmless public artifact bytes.\n' >"$scratch/$synthetic_secret.bin"
run_audit "$scratch/$synthetic_secret.bin"
expect_status 1 'credential-shaped artifact filename'
expect_line 'FAIL  secrets: built artifacts'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a credential-shaped artifact filename'; exit 1; fi
pass 'artifact identifiers are redacted too'

mkdir "$scratch/entry-archive"
printf 'Harmless synthetic placeholder.\n' >"$scratch/entry-archive/AGENTS.md"
(cd "$scratch/entry-archive" && zip -q "$scratch/entry.zip" AGENTS.md)
run_audit "$scratch/entry.zip"
expect_status 1 'private ZIP entry with clean source history'
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'private ZIP entries are rejected independently of source history'

new_repo
printf 'Harmless public fixture.\n' >"$repo/blockedword.txt"
commit 'Synthetic blockedword metadata'
git -C "$repo" rm -q blockedword.txt
commit 'Remove named fixture'
test_terms=$scratch/terms
printf 'blockedword\n' >"$test_terms"
run_audit
expect_status 1 'optional term in historical filename and metadata'
expect_line 'FAIL  terms: historical file names (2)'
expect_line 'FAIL  terms: commit metadata and trailers (2)'
pass 'optional unwanted terms include historical filenames and commit metadata'
printf '[\n' >"$test_terms"
run_audit
expect_status 1 'invalid optional regular expression'
expect_line 'FAIL  unwanted term expressions are valid'
pass 'malformed unwanted-term expressions cannot silently pass'
test_terms=""

new_repo
# Assemble only a generic signature; no real recovery-prompt text is a fixture.
cp "$audit" "$repo/public-audit.sh"
cp "$0" "$repo/public-audit-test.sh"
commit 'Public audit implementation and synthetic tests'
run_audit
expect_status 0 'public synthetic references are not private coordination content'
pass 'audit source and regression names do not trigger private heading checks'

new_repo
# Assemble only a generic signature; no real recovery-prompt text is a fixture.
heading_prefix='MASTER CONTINUITY'
printf '# %s & %s\n' "$heading_prefix" 'RECOVERY PROMPT' >"$repo/ordinary-notes.txt"
commit 'Synthetic private content under a neutral filename'
run_audit
expect_status 1 'renamed private heading'
expect_line 'FAIL  no private coordination content tracked'
git -C "$repo" rm -q ordinary-notes.txt
commit 'Remove synthetic heading'
run_audit
expect_status 1 'renamed private heading retained in history'
expect_line 'ok    no private coordination content tracked'
expect_line 'FAIL  no private coordination content in history'
pass 'renaming a private continuity heading cannot bypass history checks'

new_repo
portuguese_prefix='PROMPT'
printf '# %s %s\n' "$portuguese_prefix" 'MESTRE — synthetic placeholder' >"$repo/ordinary-notes.txt"
commit 'Synthetic Portuguese heading under a neutral name'
run_audit
expect_status 1 'Portuguese private heading'
expect_line 'FAIL  no private coordination content tracked'
pass 'Portuguese master headings are rejected under neutral filenames'

new_repo
commit 'Clean source'
python3 - "$scratch/empty.zip" <<'PY'
import sys
import zipfile
with zipfile.ZipFile(sys.argv[1], 'w'):
    pass
PY
run_audit "$scratch/empty.zip"
expect_status 0 'valid empty archive'
pass 'valid empty archives are inspected successfully'
run_audit "$scratch/missing-artifact"
expect_status 1 'missing requested artifact'
expect_line 'FAIL  artifacts can be inspected'
printf 'Damaged synthetic archive.\n' >"$scratch/damaged.zip"
run_audit "$scratch/damaged.zip"
expect_status 1 'damaged archive'
expect_line 'FAIL  artifacts can be inspected'
(cd "$scratch/secret-archive" && zip -q -P synthetic-fixture "$scratch/encrypted.zip" public.txt)
run_audit "$scratch/encrypted.zip"
expect_status 1 'encrypted unreadable archive'
expect_line 'FAIL  artifacts can be inspected'
pass 'missing, damaged or encrypted artifacts cannot silently pass'

echo 'All audit regression cases passed.'
