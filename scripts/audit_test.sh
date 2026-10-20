#!/usr/bin/env bash
# Synthetic privacy-audit regression cases. All fixtures stay outside the checkout.
set -euo pipefail
audit=$(cd "$(dirname "$0")" && pwd)/audit.sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
for command_name in git strings zip gzip bzip2 tar python3; do
	command -v "$command_name" >/dev/null || { echo "missing test prerequisite: $command_name"; exit 1; }
done

case_number=0
passed=0
test_terms=""
require_terms=""
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
	(cd "$repo" && AUDIT_TERMS_FILE="$test_terms" AUDIT_REQUIRE_TERMS="$require_terms" bash "$audit" "$@") >"$log" 2>&1 || status=$?
}
expect_status() {
	if [ "$status" -ne "$1" ]; then echo "FAIL: $2 (unexpected audit exit $status)"; exit 1; fi
}
expect_line() {
	if ! grep -q -F "$1" "$log"; then echo "FAIL: missing expected check: $1"; exit 1; fi
}
pass() { passed=$((passed + 1)); echo "ok    $1"; }

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
for name in Zeta.xlsx AGENTS.md ferramenta.md MASTER_CONTINUITY_RECOVERY_PROMPT.txt ferramenta_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts; do
	printf 'Synthetic private-category placeholder.\n' >"$repo/$name"
done
commit 'Synthetic private filename categories'
run_audit
expect_status 1 'tracked private filenames'
expect_line 'FAIL  no private files tracked'
for name in C-RT/sample-record.txt C-SR/sample-record.txt F-RT/sample-record.txt F-SR/sample-record.txt Zeta.xlsx MASTER_CONTINUITY_RECOVERY_PROMPT.txt ferramenta_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts; do expect_line "$name"; done
expect_line 'FAIL  development continuity is marked and excluded in every Git tree/index'
pass 'private categories and unapproved root coordination contracts are rejected'
git -C "$repo" rm -rq -- C-RT C-SR F-RT F-SR Zeta.xlsx AGENTS.md ferramenta.md MASTER_CONTINUITY_RECOVERY_PROMPT.txt ferramenta_transcript.md PROMPT_MESTRE_MNE_LAB_CURRENT.md MNE_Lab_Pacote_Mestre_COMPLETO.zip MNE_Lab_Pacote_Implementacao_ATUALIZADO.zip Example_Results.dts
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

# The explicitly authorized Git-only bridge is the sole coordination exception.
# Assemble the marker: this regression script is itself distributable source.
marker='MNE_LAB_'
marker+='DEVELOPMENT_ONLY'
boundary=$(dirname "$audit")/check_development_boundary.py
bridge() {
	mkdir -p "$repo/development/continuity"
	printf '/development/continuity export-ignore\n/development/continuity/** export-ignore\n/AGENTS.md export-ignore\n/ferramenta.md export-ignore\n' >"$repo/.gitattributes"
	for name in AGENTS.md ferramenta.md development/continuity/README.md development/continuity/MASTER_SPECIFICATION.md development/continuity/USER_DIRECTIVES.md development/continuity/CONTEXT.md development/continuity/MATERIALS.md development/continuity/PROMPT.md development/continuity/REQUIREMENTS_REVIEW.md; do
		printf '<!-- %s -->\n# ferramenta coordination — synthetic example\nferramenta and ferramenta development notes.\n' "$marker" >"$repo/$name"
	done
	python3 - "$repo" "$marker" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1]) / 'development' / 'continuity'
for name in ['CHECKPOINT.json', 'TASKS.json', 'VALIDATION.json']:
    (root/name).write_text(json.dumps({'development_only': sys.argv[2], 'synthetic': True}, indent=2)+'\n')
PY
}

new_repo
bridge
commit 'Authorized Git-only continuity fixture'
run_audit
expect_status 0 'authorized continuity documents'
pass 'only marked export-ignored continuity documents pass source audit'
test_terms=$scratch/tool-terms
printf 'ferramenta|ferramenta\n' >"$test_terms"
run_audit
expect_status 0 'tool names in authorized source continuity'
pass 'optional tool terms permit authorized source coordination'
printf '# ferramenta text outside a structural exclusion rule.\n' >>"$repo/.gitattributes"
git -C "$repo" add .gitattributes
run_audit
expect_status 1 'unwanted term outside exact config declarations'
expect_line 'FAIL  terms: tracked files (1)'
pass 'boundary declarations do not exempt unrelated config text from optional terms'
git -C "$repo" restore --staged --worktree .gitattributes
printf 'ferramenta artifact bytes.\n' >"$scratch/tool-artifact.bin"
run_audit "$scratch/tool-artifact.bin"
expect_status 1 'tool names in a product artifact'
expect_line 'FAIL  terms: built artifacts (3)'
pass 'tool-term exemption does not reach product artifacts'
git -C "$repo" commit --allow-empty -qm 'Synthetic ferramenta metadata'
run_audit
expect_status 1 'tool name in commit metadata'
expect_line 'FAIL  terms: commit metadata and trailers (2)'
pass 'authorized source coordination does not exempt commit metadata'
test_terms=""

git -C "$repo" archive --format=zip HEAD >"$scratch/bridge-source.zip"
python3 "$boundary" --archive-stdin <"$scratch/bridge-source.zip" >"$scratch/bridge-archive-result"
python3 - "$scratch/bridge-source.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as archive:
    assert 'README.md' in archive.namelist()
    assert 'AGENTS.md' not in archive.namelist()
    assert 'ferramenta.md' not in archive.namelist()
    assert not any(name.startswith('development/continuity') for name in archive.namelist())
PY
run_audit "$scratch/bridge-source.zip"
expect_status 0 'actual source archive excludes context'
pass 'committed export-ignore excludes every context entry from git archive'

cp "$repo/development/continuity/README.md" "$scratch/renamed-context.bin"
run_audit "$scratch/renamed-context.bin"
expect_status 1 'renamed development document'
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'development marker rejects renamed artifact content'
cp "$repo/development/continuity/TASKS.json" "$scratch/renamed-context-json.bin"
run_audit "$scratch/renamed-context-json.bin"
expect_status 1 'renamed development JSON'
pass 'JSON marker rejects renamed artifact content'
python3 - "$scratch/renamed-context-utf16.bin" "$marker" <<'PY'
import pathlib, sys
pathlib.Path(sys.argv[1]).write_bytes(sys.argv[2].encode('utf-16be'))
PY
run_audit "$scratch/renamed-context-utf16.bin"
expect_status 1 'UTF-16 renamed development marker'
expect_line 'FAIL  development context is absent from product artifacts'
pass 'development marker is also rejected in renamed UTF-16 content'
python3 - "$scratch/nested-context.zip" "$repo/development/continuity/README.md" <<'PY'
import io, pathlib, sys, zipfile
inner = io.BytesIO()
with zipfile.ZipFile(inner, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
    archive.writestr('renamed/public.txt', pathlib.Path(sys.argv[2]).read_bytes())
with zipfile.ZipFile(sys.argv[1], 'w', compression=zipfile.ZIP_DEFLATED) as archive:
    archive.writestr('nested/ordinary.zip', inner.getvalue())
PY
run_audit "$scratch/nested-context.zip"
expect_status 1 'context in nested archive'
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
if python3 "$boundary" --archive-stdin <"$scratch/nested-context.zip" >"$scratch/nested-context-result" 2>&1; then echo 'FAIL: boundary accepted nested context'; exit 1; fi
pass 'nested archives reject renamed marked continuity documents'

printf '%s\n' "$synthetic_secret" >>"$repo/AGENTS.md"
git -C "$repo" add AGENTS.md
run_audit
expect_status 1 'secret inside authorized coordination'
expect_line 'FAIL  secrets: tracked files'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: context audit exposed secret'; exit 1; fi
pass 'continuity permission never exempts credentials in the index or working tree'
git -C "$repo" restore --staged --worktree AGENTS.md
mkdir "$repo/development/continuity/C-RT"
printf 'Synthetic corpus placeholder.\n' >"$repo/development/continuity/C-RT/sample.txt"
git -C "$repo" add development/continuity/C-RT/sample.txt
run_audit
expect_status 1 'corpus under continuity'
expect_line 'FAIL  no private files tracked'
pass 'corpus categories stay forbidden inside the continuity directory'

new_repo
bridge
printf 'Unmarked synthetic placeholder.\n' >"$repo/AGENTS.md"
commit 'Unmarked root coordination fixture'
run_audit
expect_status 1 'root agent file without development marker'
expect_line 'AGENTS.md: missing first-line development marker'
pass 'root coordination requires its explicit development marker'

new_repo
bridge
printf '# Prompt mestre — synthetic example\n' >>"$repo/development/continuity/MASTER_SPECIFICATION.md"
rm "$repo/.gitattributes"
commit 'Missing distribution exclusion fixture'
run_audit
expect_status 1 'coordination without committed exclusion'
expect_line 'export-ignore is required'
printf '/development/continuity export-ignore\n/development/continuity/** export-ignore\n/AGENTS.md export-ignore\n/ferramenta.md export-ignore\n' >"$repo/.gitattributes"
commit 'Add exclusion after unauthorized revision'
run_audit
expect_status 1 'old unauthorized coordination remains in history'
expect_line 'export-ignore is required'
pass 'current exclusion cannot conceal a historical unauthorized coordination tree'

new_repo
bridge
cp "$repo/.gitattributes" "$repo/.git/info/attributes"
rm "$repo/.gitattributes"
commit 'Local attribute overlay fixture'
run_audit
expect_status 1 'local-only exclusion cannot certify a committed tree'
expect_line 'Local info/attributes overrides cannot certify committed continuity exclusions'
pass 'local attribute overrides cannot replace committed distribution exclusions'

new_repo
bridge
commit 'Valid contract for index isolation'
cp "$repo/.gitattributes" "$scratch/valid-bridge-attributes"
printf '# Missing exclusions in index.\n' >"$repo/.gitattributes"
git -C "$repo" add .gitattributes
cp "$scratch/valid-bridge-attributes" "$repo/.gitattributes"
run_audit
expect_status 1 'index-only exclusion removal'
expect_line 'export-ignore is required'
pass 'export-ignore is verified independently in index and working tree'

new_repo
bridge
printf '<!-- %s -->\nUnknown document.\n' "$marker" >"$repo/development/continuity/EXTRA.md"
commit 'Unapproved context entry fixture'
run_audit
expect_status 1 'unknown path under continuity'
expect_line 'Unapproved entry under development/continuity'
pass 'continuity is a fixed file whitelist, not a directory-wide exception'

new_repo
bridge
printf '<!-- %s -->\nInvalid text:\377\n' "$marker" >"$repo/AGENTS.md"
commit 'Non-UTF8 context fixture'
run_audit
expect_status 1 'non-UTF8 continuity'
expect_line 'continuity document must be UTF-8'
pass 'binary text cannot use an approved continuity filename'

new_repo
bridge
printf '<!-- %s -->\nInvalid NUL:\000\n' "$marker" >"$repo/AGENTS.md"
commit 'NUL context fixture'
run_audit
expect_status 1 'NUL continuity'
expect_line 'NUL byte'
pass 'NUL-bearing continuity documents are rejected'

new_repo
bridge
rm "$repo/AGENTS.md"
ln -s README.md "$repo/AGENTS.md"
commit 'Symlink context fixture'
run_audit
expect_status 1 'symlink continuity'
expect_line 'continuity document must be a regular file'
pass 'continuity documents cannot be symlinks'

new_repo
bridge
printf '{"development_only": false}\n' >"$repo/development/continuity/TASKS.json"
commit 'Missing JSON context marker fixture'
run_audit
expect_status 1 'JSON without explicit development marker'
expect_line 'missing JSON development marker'
pass 'JSON continuity requires its exact development marker'

new_repo
mkdir -p "$repo/docs"
printf '<!-- %s -->\n# ferramenta coordination — synthetic\n' "$marker" >"$repo/docs/ferramenta.md"
printf '/docs/ferramenta.md export-ignore\n' >"$repo/.gitattributes"
commit 'Coordination outside approved paths'
run_audit
expect_status 1 'coordination outside whitelist'
expect_line 'FAIL  no private files tracked'
pass 'marking and export-ignore cannot authorize an unapproved source path'

new_repo
bridge
commit 'Valid bridge for exact branding patterns'
test_terms=$scratch/exact-branding-terms
printf 'ferramenta|ferramenta|ferramenta|ferramenta\n' >"$test_terms"
printf 'ferramenta and ferramenta development references.\n' >>"$repo/AGENTS.md"
git -C "$repo" add AGENTS.md
run_audit
expect_status 0 'only known tool names in a valid bridge'
pass 'branding exemption accepts only literal known tool names or their simple alternation'
printf 'ferramenta|privateword\n' >"$test_terms"
run_audit
expect_status 1 'unknown mixed alternation is not branding-only'
expect_line 'FAIL  terms: tracked files (1)'
pass 'a mixed branding and custom expression retains coverage of continuity content'
test_terms=""

new_repo
bridge
printf 'Synthetic private_identifier.\n' >>"$repo/development/continuity/CONTEXT.md"
commit 'Valid bridge with synthetic custom identifier'
test_terms=$scratch/custom-continuity-terms
printf 'private[-_]identifier\n' >"$test_terms"
run_audit
expect_status 1 'custom confidentiality regex inside a marked bridge'
expect_line 'FAIL  terms: tracked files (1)'
pass 'custom confidentiality expressions inspect marked continuity content'
test_terms=""

new_repo
bridge
commit 'Valid bridge for staged-only custom identifier'
printf 'Synthetic private_identifier.\n' >>"$repo/AGENTS.md"
git -C "$repo" add AGENTS.md
git -C "$repo" show HEAD:AGENTS.md >"$repo/AGENTS.md"
test_terms=$scratch/staged-custom-continuity-terms
printf 'private[-_]identifier\n' >"$test_terms"
run_audit
expect_status 1 'custom identifier exists only in a marked staged document'
expect_line 'FAIL  terms: tracked files (1)'
pass 'staged continuity bytes retain custom confidentiality term checks'
test_terms=""

new_repo
bridge
commit 'Valid bridge for historical custom identifier'
printf 'Synthetic private_identifier.\n' >>"$repo/development/continuity/CONTEXT.md"
commit 'Synthetic custom identifier in prior bridge revision'
git -C "$repo" show HEAD~1:development/continuity/CONTEXT.md >"$repo/development/continuity/CONTEXT.md"
commit 'Remove synthetic identifier from current bridge'
test_terms=$scratch/historical-custom-continuity-terms
printf 'private[-_]identifier\n' >"$test_terms"
run_audit
expect_status 1 'custom identifier exists only in a historical marked document'
expect_line 'ok    terms: tracked files (1)'
expect_line 'FAIL  terms: every revision (2)'
pass 'historical continuity bytes retain custom confidentiality term checks'
test_terms=""

new_repo
bridge
commit 'Valid bridge for custom filename expression'
test_terms=$scratch/filename-custom-continuity-terms
printf 'CONTEXT\.md\n' >"$test_terms"
run_audit
expect_status 1 'custom confidentiality expression matches an approved filename'
expect_line 'FAIL  terms: file names (1)'
expect_line 'FAIL  terms: historical file names (2)'
pass 'approved continuity filenames retain custom confidentiality term checks'
test_terms=""

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

# Git quotes non-ASCII names unless told otherwise; anchored patterns must see them literally.
new_repo
accented=$(printf 'An\303\241lises')
mkdir "$repo/$accented"
printf 'Synthetic placeholder.\n' >"$repo/$accented/Zeta.xlsx"
commit 'Synthetic accented private path'
run_audit
expect_status 1 'precomposed accented private path'
expect_line 'FAIL  no private files tracked'
expect_line "$accented/Zeta.xlsx"
git -C "$repo" rm -rq -- "$accented"
commit 'Remove synthetic accented path'
run_audit
expect_status 1 'accented private path retained in history'
expect_line 'FAIL  no private file names in history'
expect_line ":$accented/Zeta.xlsx"
pass 'precomposed accented private paths are matched and reported literally'

new_repo
decomposed=$(printf 'Pacote_Implementac\314\247a\314\203o')
compatibility=$(printf 'release.\357\275\220\357\275\205\357\275\215')
mkdir "$repo/$decomposed"
printf 'Synthetic placeholder.\n' >"$repo/$decomposed/notes.txt"
printf 'Synthetic placeholder.\n' >"$repo/$compatibility"
commit 'Synthetic decomposed and compatibility private names'
run_audit
expect_status 1 'decomposed and compatibility private names'
expect_line 'FAIL  no private files tracked'
expect_line "$decomposed/notes.txt"
expect_line "$compatibility"
python3 - "$scratch/folded-entry.zip" "$compatibility" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w') as archive:
    archive.writestr('bundle/' + sys.argv[2], 'Synthetic placeholder.\n')
PY
run_audit "$scratch/folded-entry.zip"
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'decomposed and compatibility forms of private names are folded in Git and ZIP entries'

new_repo
control_name=$(printf 'notes\tdraft.txt')
printf 'Synthetic placeholder.\n' >"$repo/$control_name"
commit 'Synthetic control character in a name'
run_audit
expect_status 1 'control character in a tracked name'
expect_line 'FAIL  Git path names are literal UTF-8 in the index and every revision'
expect_line '"notes\tdraft.txt"'
git -C "$repo" rm -q -- "$control_name"
commit 'Remove synthetic control character name'
run_audit
expect_status 1 'control character name retained in history'
expect_line 'FAIL  Git path names are literal UTF-8 in the index and every revision'
pass 'names that Git still quotes fail in the index and in history'

new_repo
{ printf 'BIN\000\001\002'; printf '%s' "$synthetic_secret"; printf '\000\377\376'; } >"$repo/public.bin"
commit 'Synthetic binary bytes'
run_audit
expect_status 1 'secret inside binary content'
expect_line 'FAIL  secrets: tracked files'
expect_line 'FAIL  secrets: history'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a binary secret'; exit 1; fi
git -C "$repo" rm -q public.bin
commit 'Remove synthetic binary bytes'
run_audit
expect_status 1 'binary secret retained in history'
expect_line 'ok    secrets: tracked files'
expect_line 'FAIL  secrets: history'
pass 'binary blobs are scanned for secrets in the index and history'

new_repo
python3 - "$repo/report.xlsx" "$repo/attachment.dat" <<'PY'
import sys, zipfile
for target, member in ((sys.argv[1], 'xl/embeddings/C-RT/sample-record.txt'), (sys.argv[2], 'config/.env')):
    with zipfile.ZipFile(target, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
        archive.writestr('[Content_Types].xml', '<Types/>')
        archive.writestr(member, 'Synthetic placeholder.\n')
PY
commit 'Synthetic workbook and attachment'
run_audit
expect_status 1 'private entry names inside tracked archives'
expect_line 'FAIL  no private files tracked'
expect_line 'report.xlsx'
expect_line 'attachment.dat'
git -C "$repo" rm -q report.xlsx attachment.dat
commit 'Remove synthetic archives'
run_audit
expect_status 1 'private archive entry names retained in history'
expect_line 'ok    no private files tracked'
expect_line 'FAIL  no private file names in history'
expect_line ':report.xlsx'
pass 'archive entry names are checked in the index and history, by extension or magic bytes'

new_repo
python3 - "$repo/data.ods" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', compression=zipfile.ZIP_DEFLATED) as archive:
    archive.writestr('mimetype', 'application/vnd.oasis.opendocument.spreadsheet')
    archive.writestr('content.xml', '<text>synthetic blocked' + 'word cell ' * 40 + '</text>')
PY
if grep -a -q -F blockedword "$repo/data.ods"; then echo 'FAIL: archive fixture stores the term uncompressed'; exit 1; fi
commit 'Synthetic spreadsheet'
git -C "$repo" rm -q data.ods
commit 'Remove synthetic spreadsheet'
test_terms=$scratch/archive-terms
printf 'blockedword\n' >"$test_terms"
run_audit
expect_status 1 'term inside a historical archive'
expect_line 'FAIL  terms: every revision (2)'
expect_line ':data.ods'
test_terms=""
pass 'decompressed archive members in history are checked for terms'

new_repo
python3 - "$repo/bundle.zip" "$marker" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], 'w', compression=zipfile.ZIP_DEFLATED) as archive:
    archive.writestr('notes/ordinary.md', '<!-- ' + sys.argv[2] + ' -->\n' + 'Synthetic padding. ' * 20)
PY
commit 'Synthetic bundle'
run_audit
expect_status 1 'development marker inside a tracked archive'
expect_line 'FAIL  no private coordination content tracked'
expect_line 'bundle.zip'
pass 'tracked archive members are checked for the development marker'

new_repo
mkdir "$scratch/plain-archive"
printf 'Harmless synthetic bytes.\n' >"$scratch/plain-archive/public.txt"
(cd "$scratch/plain-archive" && zip -q -P synthetic-fixture "$repo/locked.zip" public.txt)
printf 'Damaged synthetic workbook.\n' >"$repo/broken.xlsx"
commit 'Synthetic damaged and encrypted archives'
run_audit
expect_status 1 'tracked archives that cannot be inspected'
expect_line 'FAIL  tracked and historical archives can be inspected'
expect_line 'broken.xlsx'
expect_line 'locked.zip'
pass 'damaged or encrypted tracked archives cannot silently pass'

new_repo
mkdir "$repo/certs"
printf 'Synthetic placeholder.\n' >"$repo/certs/device.p12"
commit 'Synthetic key store name'
run_audit
expect_status 1 'PKCS#12 file name'
expect_line 'FAIL  no private files tracked'
expect_line 'certs/device.p12'
mkdir "$scratch/pfx-entry"
printf 'Synthetic placeholder.\n' >"$scratch/pfx-entry/identity.pfx"
(cd "$scratch/pfx-entry" && zip -q "$scratch/pfx-entry.zip" identity.pfx)
run_audit "$scratch/pfx-entry.zip"
expect_line 'FAIL  no private file names in artifacts or ZIP entries'
pass 'PKCS#12 key store names are private in Git and in archive entries'

pfx=$scratch/fixture-store.bin
if command -v openssl >/dev/null 2>&1 &&
	openssl req -x509 -newkey rsa:2048 -nodes -subj /CN=audit-fixture -days 1 -keyout "$scratch/pfx-key.pem" -out "$scratch/pfx-cert.pem" >/dev/null 2>&1 &&
	openssl pkcs12 -export -inkey "$scratch/pfx-key.pem" -in "$scratch/pfx-cert.pem" -passout pass:synthetic-fixture -out "$pfx" >/dev/null 2>&1; then
	pfx_source=openssl
else
	# SEQUENCE { INTEGER 3, ContentInfo { data, [0] OCTET STRING } } without a real key.
	python3 - "$pfx" <<'PY'
import sys
def der(tag, body):
    size = len(body)
    width = (size.bit_length() + 7) // 8
    length = bytes([size]) if size < 128 else bytes([0x80 | width]) + size.to_bytes(width, 'big')
    return bytes([tag]) + length + body
content = der(0x30, der(0x06, bytes.fromhex('2a864886f70d010701')) + der(0xA0, der(0x04, b'synthetic safe contents ' * 8)))
open(sys.argv[1], 'wb').write(der(0x30, der(0x02, b'\x03') + content))
PY
	pfx_source='fixed DER bytes'
fi
new_repo
mkdir "$repo/assets" "$scratch/pfx-member"
cp "$pfx" "$repo/assets/logo.dat"
cp "$pfx" "$scratch/pfx-member/payload.bin"
(cd "$scratch/pfx-member" && zip -q "$repo/assets/bundle.zip" payload.bin)
commit 'Synthetic opaque assets'
run_audit
expect_status 1 'PFX content in tracked blobs and archive members'
expect_line 'FAIL  secrets: tracked files'
expect_line 'assets/logo.dat'
expect_line 'assets/bundle.zip'
git -C "$repo" rm -q assets/logo.dat assets/bundle.zip
commit 'Remove synthetic opaque assets'
run_audit
expect_status 1 'PFX content retained in history'
expect_line 'ok    secrets: tracked files'
expect_line 'FAIL  secrets: history'
(cd "$scratch/pfx-member" && zip -q "$scratch/pfx-member.zip" payload.bin)
run_audit "$scratch/pfx-member.zip"
expect_line 'FAIL  secrets: built artifacts'
cp "$pfx" "$scratch/pfx-artifact.bin"
run_audit "$scratch/pfx-artifact.bin"
expect_line 'FAIL  secrets: built artifacts'
pass "PFX content under innocent names is found in blobs, history, archive members and artifacts ($pfx_source)"

new_repo
python3 - "$repo/tool.exe" <<'PY'
import sys
identifiers = bytes.fromhex('3082010a020103308201000609' '2a864886f70d010701')
open(sys.argv[1], 'wb').write(b'MZ' + b'\0' * 62 + identifiers + b'\0' * 64 + bytes.fromhex('06092a864886f70d010702'))
PY
cp "$repo/tool.exe" "$scratch/tool.exe"
commit 'Synthetic executable with key store identifiers'
run_audit "$scratch/tool.exe"
expect_status 0 'key store identifiers inside an executable'
pass 'PKCS#12 identifiers inside other binaries are not mistaken for key stores'

new_repo
git -C "$repo" add -A
git -C "$repo" commit -qm 'Synthetic fixture' -m 'Co-authored-by: Fixture Person <fixture@example.invalid>'
run_audit
expect_status 1 'co-author trailer without terms'
expect_line 'WARN  unwanted terms were not checked'
expect_line 'FAIL  no co-author or session trailers in commit messages'
new_repo
git -C "$repo" add -A
git -C "$repo" commit -qm 'Synthetic fixture' -m 'Fixture-Session: https://example.invalid/session'
run_audit
expect_status 1 'session trailer without terms'
expect_line 'FAIL  no co-author or session trailers in commit messages'
pass 'co-author and session trailers fail without any terms file'

new_repo
commit 'Clean source'
run_audit
expect_status 0 'optional terms are missing'
expect_line 'WARN  unwanted terms were not checked (AUDIT_TERMS_FILE not set)'
require_terms=1
run_audit
expect_status 1 'required terms are missing'
expect_line 'FAIL  unwanted terms file is readable and nonempty'
test_terms=$scratch/empty-required-terms
: >"$test_terms"
run_audit
expect_status 1 'required terms file is empty'
test_terms=$scratch/required-terms
printf 'blockedword\n' >"$test_terms"
run_audit
expect_status 0 'required terms are present'
require_terms=""
test_terms=""
pass 'AUDIT_REQUIRE_TERMS=1 turns missing terms into a failure; otherwise a warning is shown'

# Branding terms come from the earlier fixture so no new tool name is written here.
branding_word=$(cut -d'|' -f1 "$scratch/tool-terms")
new_repo
mkdir "$repo/scripts"
printf 'Detects %s coordination files.\n' "$branding_word" >"$repo/scripts/audit.sh"
commit 'Synthetic audit tooling'
printf 'Detects %s names in history too.\n' "$branding_word" >>"$repo/scripts/audit.sh"
commit 'Synthetic audit tooling revision'
test_terms=$scratch/branding-word-terms
printf '%s\n' "$branding_word" >"$test_terms"
run_audit
expect_status 0 'branding term inside audit tooling'
cp "$repo/scripts/audit.sh" "$repo/scripts/other.sh"
git -C "$repo" add scripts/other.sh
run_audit
expect_status 1 'branding term outside audit tooling'
expect_line 'FAIL  terms: tracked files (1)'
expect_line 'scripts/other.sh'
git -C "$repo" rm -q --cached scripts/other.sh
rm "$repo/scripts/other.sh"
printf 'Synthetic blockedword reference.\n' >>"$repo/scripts/audit.sh"
git -C "$repo" add scripts/audit.sh
printf '%s\nblockedword\n' "$branding_word" >"$test_terms"
run_audit
expect_status 1 'confidential term inside audit tooling'
expect_line 'FAIL  terms: tracked files (1)'
expect_line 'scripts/audit.sh'
test_terms=""
pass 'audit tooling paths are exempt from branding terms only, never from confidential terms'

new_repo
mkdir "$repo/docs"
printf 'Published notes naming %s and blockedword.\n' "$branding_word" >"$repo/docs/notes.md"
commit 'Synthetic published notes'
accepted_blob=$(git -C "$repo" rev-parse HEAD:docs/notes.md)
printf 'Published notes without those names.\n' >"$repo/docs/notes.md"
commit 'Synthetic cleaned notes'
test_terms=$scratch/accepted-terms
printf '%s\n' "$branding_word" >"$test_terms"
run_audit
expect_status 1 'unlisted historical branding blob'
expect_line 'FAIL  terms: every revision (2)'
# Exercise the fixed list through a copy that also lists this synthetic blob.
mkdir "$scratch/accepting"
sed "/<<'BLOBS'\$/a $accepted_blob" "$audit" >"$scratch/accepting/audit.sh"
cp "$boundary" "$scratch/accepting/"
grep -q -x -F "$accepted_blob" "$scratch/accepting/audit.sh" || { echo 'FAIL: accepted blob fixture was not inserted'; exit 1; }
original_audit=$audit
audit=$scratch/accepting/audit.sh
run_audit
expect_status 0 'accepted historical branding blob'
printf '%s\nblockedword\n' "$branding_word" >"$test_terms"
run_audit
expect_status 1 'accepted blob with a confidential term'
expect_line 'FAIL  terms: every revision (2)'
printf '%s\n' "$branding_word" >"$test_terms"
printf 'New notes naming %s again.\n' "$branding_word" >"$repo/docs/notes.md"
commit 'Synthetic new branding notes'
run_audit
expect_status 1 'new blob with a branding term'
expect_line 'FAIL  terms: tracked files (1)'
expect_line 'FAIL  terms: every revision (2)'
audit=$original_audit
test_terms=""
pass 'only listed published blobs are accepted for branding terms; confidential terms still apply'

new_repo
commit 'Clean source'
git -C "$repo" commit -q --allow-empty -m 'Synthetic fixture' -m 'Fixture-Session-Id: 1234'
run_audit
expect_status 1 'session trailer variant'
expect_line 'FAIL  no co-author or session trailers in commit messages'
new_repo
commit 'Clean source'
git -C "$repo" tag -a v1 -m 'Synthetic release' -m "Co-Authored-By: Fixture <fixture@example.invalid>"
run_audit
expect_status 1 'trailer in an annotated tag'
expect_line 'FAIL  no co-author or session trailers in commit messages'
new_repo
commit 'Clean source'
git -C "$repo" tag -a v1 -m "Synthetic release $synthetic_secret"
run_audit
expect_status 1 'secret in an annotated tag'
expect_line 'FAIL  secrets: commit metadata and trailers'
if grep -q -F "$synthetic_secret" "$log"; then echo 'FAIL: audit printed a tag secret'; exit 1; fi
new_repo
commit 'Clean source'
tagged=$(printf '%s\n' "$synthetic_secret" | git -C "$repo" hash-object -w --stdin)
git -C "$repo" tag -a blob-tag -m 'Synthetic tagged blob' "$tagged"
run_audit
expect_status 1 'secret in a tagged blob'
expect_line 'FAIL  secrets: history'
pass 'annotated tags, their messages and the objects they point at are inspected'

new_repo
mkdir "$scratch/bundle-$case_number"
printf '%s\n' "$synthetic_secret" >"$scratch/bundle-$case_number/ferramenta.md"
tar -C "$scratch/bundle-$case_number" -czf "$repo/bundle.tar.gz" ferramenta.md
commit 'Synthetic compressed archive'
run_audit
expect_status 1 'compressed tar archive'
expect_line 'FAIL  secrets: tracked files'
expect_line 'FAIL  no private files tracked'
new_repo
printf '%s\n' "$synthetic_secret" | gzip -c >"$repo/data.bin"
commit 'Synthetic compressed stream'
run_audit
expect_status 1 'compressed stream'
expect_line 'FAIL  secrets: tracked files'
new_repo
printf '%s\n' "$synthetic_secret" >"$scratch/member-$case_number.txt"
(cd "$scratch" && zip -q "$scratch/prefixed-$case_number.zip" "member-$case_number.txt")
{ printf 'X'; cat "$scratch/prefixed-$case_number.zip"; } >"$repo/prefixed.bin"
commit 'Synthetic archive behind a prefix'
run_audit
expect_status 1 'archive behind a prefix'
expect_line 'FAIL  secrets: tracked files'
new_repo
printf '%s\n' "$synthetic_secret" | python3 -c 'import sys; sys.stdout.buffer.write(sys.stdin.read().encode("utf-16"))' >"$repo/wide.txt"
commit 'Synthetic UTF-16 text'
run_audit
expect_status 1 'UTF-16 text'
expect_line 'FAIL  secrets: tracked files'
new_repo
{ printf 'Synthetic compressed part.\n' | gzip -c; printf '%s\n' "$synthetic_secret"; } >"$repo/appended.gz"
commit 'Synthetic data after a compressed stream'
run_audit
expect_status 1 'data after a compressed stream'
expect_line 'ok    tracked and historical archives can be inspected'
expect_line 'FAIL  secrets: tracked files'
new_repo
{ printf 'First part.\n' | gzip -c; printf 'Second part.\n' | bzip2 -c; } >"$repo/clean.gz"
commit 'Synthetic concatenated streams'
run_audit
expect_status 0 'concatenated compressed streams'
new_repo
printf '7z\274\257\047\034synthetic' >"$repo/opaque.7z"
commit 'Synthetic archive that cannot be read'
run_audit
expect_status 1 'archive that cannot be inspected'
expect_line 'FAIL  tracked and historical archives can be inspected'
printf '%s\n' "$synthetic_secret" | gzip -c >"$scratch/release-$case_number.gz"
new_repo
commit 'Clean source'
run_audit "$scratch/release-$((case_number - 1)).gz"
expect_status 1 'compressed artifact'
expect_line 'FAIL  secrets: built artifacts'
pass 'compressed streams, tar and prefixed archives and UTF-16 text are inspected; unknown archives fail'

echo "All $passed audit regression cases passed."
