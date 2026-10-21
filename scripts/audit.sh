#!/usr/bin/env bash
# Pre-publication audit of the current index, every revision and build artifacts.
# Optional unwanted expressions belong in AUDIT_TERMS_FILE outside the checkout;
# AUDIT_REQUIRE_TERMS=1 makes a missing terms file a failure instead of a warning.
# Findings name files or commits only: matched private contents are never printed.
# Usage: scripts/audit.sh [artifact-file-or-dir ...]
set -u
export LC_ALL=C.UTF-8
# Quoted non-ASCII names would evade anchored patterns; names Git still quotes
# (control characters, quotes, backslashes) fail their own check below.
git() { command git -c core.quotePath=false "$@"; }
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

# Confidential categories: credentials, the private corpus and private
# development documents. Tool names are checked through the terms file.
private_re='(^|/)(\.env(\..*)?|[^/]*\.pem|[^/]*\.key|[^/]*\.p12|[^/]*\.pfx|release-signing[^/]*|providers\.json|Zeta\.xlsx)$|(^|/)(C-(RT|SR)|F-(RT|SR)|\.private-fixtures|\.dev-data)(/|$)|(^|/)[^/]*pacote[-_ .]*mestre[^/]*(/|$)|(^|/)[^/]*(prompt[-_ .]*mestre|mestre[-_ .]*prompt)[^/]*$|(^|/)[^/]*(master[-_ .]*(continuity|recovery|prompt)|continuity[-_ .]*(recovery[-_ .]*)?prompt|recovery[-_ .]*prompt)[^/]*$|(^|/)example[-_ .]*results\.dts$|(^|/)[^/]*pacote[-_ .]*implementa(cao|ção)[^/]*(/|$)'
development_marker='MNE_LAB_'
development_marker+='DEVELOPMENT_ONLY'
command -v python3 >/dev/null 2>&1 || exit 1
private_content=$scratch/private-content
cat >"$private_content" <<'PATTERNS'
^#{1,6}[[:space:]]+.*master[[:space:]_-]+continuity[[:space:][:punct:]]+recovery[[:space:]_-]+prompt
^[[:space:]]*master[[:space:]_-]+continuity[[:space:][:punct:]]+recovery[[:space:]_-]+prompt([[:space:]]|$)
^#{1,6}[[:space:]]+.*prompt[[:space:]_-]+mestre([[:space:]]|$)
PATTERNS
# Development-only documents never belong in the repository or a build.
printf '%s\n' "$development_marker" >>"$private_content"
trailer_re='^[[:space:]]*(co-authored-by|([[:alnum:]_]+-)*session(-[[:alnum:]_]+)*)[[:space:]]*:'

helper=$scratch/inspect.py
cat >"$helper" <<'PY'
"""Name folding and blob, archive and key store inspection for the audit.

Results name objects or positions only; contents stay in the scratch folder.
Limits fail the inspection rather than skipping any content.
"""
import bz2
import io
import lzma
import os
import pathlib
import re
import subprocess
import sys
import tarfile
import unicodedata
import zipfile
import zlib

LIMIT = 512 * 1024 * 1024
ZIP_MAGIC = (b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")
ZIP_SUFFIXES = tuple("." + suffix for suffix in (
    "zip xlsx xlsm xltx xltm xlam docx docm dotx dotm pptx pptm potx ppsx "
    "odt ods odp odg odf ott ots otp epub jar war apk aar whl nupkg vsix xpi kmz 3mf appx msix"
).split())
# Compressed streams that are expanded, and formats that cannot be read here
# and therefore fail the inspection instead of passing unread.
STREAMS = (
    (b"\x1f\x8b", lambda: zlib.decompressobj(31)),
    (b"BZh", bz2.BZ2Decompressor),
    (b"\xfd7zXZ\x00", lzma.LZMADecompressor),
)
UNREADABLE = (b"7z\xbc\xaf\x27\x1c", b"Rar!\x1a\x07", b"\x28\xb5\x2f\xfd", b"\x04\x22\x4d\x18", b"MSCF")
# UTF-16 text runs, as strings -e l and -e b find them.
UTF16 = (re.compile(rb"(?:[\t\x20-\x7e]\x00){6,}"), re.compile(rb"(?:\x00[\t\x20-\x7e]){6,}"))
# ContentInfo types of a PKCS#12 PFX: PKCS#7 data or signedData.
PFX_CONTENT = tuple(bytes.fromhex("06092a864886f70d0107" + kind) for kind in ("01", "02"))


def fold(text):
    # Compatibility forms, combining marks and invisible format characters
    # must not change what a name matches.
    for _ in range(2):
        text = "".join(
            c for c in unicodedata.normalize("NFKD", text)
            if unicodedata.category(c)[0] != "M" and unicodedata.category(c) != "Cf"
        ).casefold()
    return text


def fold_file(path):
    lines = pathlib.Path(path).read_bytes().split(b"\n")
    folded = (fold(line.decode("utf-8", "replace")).encode("utf-8") for line in lines)
    pathlib.Path(str(path) + ".folded").write_bytes(b"\n".join(folded))


def element(raw, pos, tag):
    """Offset of the contents of a DER or BER element with this tag."""
    if len(raw) < pos + 2 or raw[pos] != tag:
        return None
    extra = raw[pos + 1] & 0x7F if raw[pos + 1] & 0x80 else 0
    return pos + 2 + extra if extra <= 4 else None


def pkcs12(raw):
    # Structure at the start only: SEQUENCE { INTEGER 3, ContentInfo }, so
    # executables that merely contain these identifiers are not key stores.
    pos = element(raw, 0, 0x30)
    if pos is None or raw[pos:pos + 3] != b"\x02\x01\x03":
        return False
    pos = element(raw, pos + 3, 0x30)
    return pos is not None and raw[pos:pos + 11] in PFX_CONTENT


def tar(content):
    return len(content) >= 262 and content[257:262] == b"ustar"


def kind(names, head, source):
    """How a file is read: zip, stream, tar, unreadable, or None for plain.

    head is its first 512 bytes and source the file or a file object.
    """
    if head.startswith(UNREADABLE):
        return "unreadable"
    if any(head.startswith(magic) for magic, _ in STREAMS):
        return "stream"
    if tar(head):
        return "tar"
    if head.startswith(ZIP_MAGIC) or any(name.strip('"').lower().endswith(ZIP_SUFFIXES) for name in names):
        return "zip"
    # Archives behind a prefix, such as self-extracting files.
    if zipfile.is_zipfile(source):
        return "zip"
    return None


def expand(content, limit):
    """Expand concatenated compressed streams within limit bytes.

    Bytes after the last stream, such as data appended to a compressed file,
    are kept as they are so they are still matched.
    """
    out = bytearray()
    while content:
        start = next((new for magic, new in STREAMS if content.startswith(magic)), None)
        if start is None:
            out += content
            break
        stream = start()
        out += stream.decompress(content, limit + 1 - len(out))
        if len(out) > limit:
            raise ValueError("archive expansion limit")
        if not stream.eof:
            raise ValueError("truncated compressed stream")
        content = stream.unused_data
    if len(out) > limit:
        raise ValueError("archive expansion limit")
    return bytes(out)


def utf16(content):
    """UTF-16 text runs of content, decoded to bytes for matching."""
    runs = [run.replace(b"\x00", b"") for pattern in UTF16 for run in pattern.findall(content)]
    return b"\n".join(runs)


def inspect(raw, names, data, how):
    """Write member names and expanded bytes; return whether a member is a PFX."""
    total = 0
    found = False

    def member(name, content, depth):
        nonlocal total, found
        if any(ord(c) < 32 or ord(c) == 127 for c in name):
            raise ValueError("member name cannot be matched")
        names.write(name.encode("utf-8", "surrogateescape") + b"\n")
        total += len(content)
        if total > LIMIT:
            raise ValueError("archive expansion limit")
        data.write(b"\n" + content + b"\n" + utf16(content) + b"\n")
        found = pkcs12(content) or found
        inner = kind([name], content[:512], io.BytesIO(content))
        if inner:
            walk(content, inner, depth + 1)

    def walk(content, how, depth):
        if depth > 16:
            raise ValueError("archive nesting limit")
        if how == "unreadable":
            raise ValueError("archive format cannot be inspected")
        if how == "stream":
            # A stream has no member names; its content is judged as a
            # member of the file that holds it.
            member("", expand(content, LIMIT - total), depth)
            return
        if how == "tar":
            with tarfile.open(fileobj=io.BytesIO(content), mode="r:") as archive:
                for entry in archive:
                    if entry.isfile():
                        member(entry.name, archive.extractfile(entry).read(), depth)
                    else:
                        names.write(entry.name.encode("utf-8", "surrogateescape") + b"\n")
            return
        with zipfile.ZipFile(io.BytesIO(content)) as archive:
            for entry in archive.infolist():
                if entry.is_dir():
                    names.write(entry.filename.encode("utf-8", "surrogateescape") + b"\n")
                    continue
                if total + entry.file_size > LIMIT:
                    raise ValueError("archive expansion limit")
                member(entry.filename, archive.read(entry), depth)

    walk(raw, how, 0)
    return found


def inspect_file(path, names, base, text=False):
    """Return (PFX found, inspectable) for one file known by these names.

    With text, UTF-16 runs of a plain file are written for matching too.
    """
    with open(path, "rb") as handle:
        head = handle.read(512)
    found = pkcs12(head)
    try:
        how = kind(names, head, path)
    except Exception:
        return found, False
    if os.path.getsize(path) > LIMIT:
        # Plain files of any size are matched where they are.
        return found, not how and not text
    with open(path, "rb") as handle:
        raw = handle.read()
    if not how:
        runs = utf16(raw) if text else b""
        if runs:
            pathlib.Path(base + ".data").write_bytes(runs + b"\n")
        return found, True
    try:
        with open(base + ".names", "ab") as entries, open(base + ".data", "wb") as data:
            if text:
                data.write(utf16(raw) + b"\n")
            return (inspect(raw, entries, data, how) or found), True
    except Exception:
        return found, False


def plain(entries, out):
    """Names Git still quotes or that are not UTF-8 cannot be matched reliably."""
    with open(out, "w") as report:
        for line in entries:
            meta, _, name = line.partition(b"\t")
            try:
                if not name.decode("utf-8").startswith('"'):
                    continue
            except UnicodeDecodeError:
                pass
            where = meta.split(b" ", 1)[0].decode("ascii")
            shown = name.decode("ascii", "backslashreplace")
            report.write((shown if where == "index" else f"{where}:{shown}") + "\n")


def blobs(entries, out):
    """Write each distinct blob once and inspect its archives and key stores."""
    names = {}
    for line in entries:
        meta, _, name = line.partition(b"\t")
        fields = meta.split(b" ")
        if len(fields) == 4 and fields[2] == b"blob":
            names.setdefault(fields[3].decode("ascii"), []).append(name.decode("utf-8", "replace"))
    out = pathlib.Path(out)
    for folder in ("blobs", "zips"):
        (out / folder).mkdir()
    (out / "requests").write_text("".join(oid + "\n" for oid in names))
    found = {"missing": [], "unreadable": [], "pkcs12": []}
    with open(out / "requests", "rb") as requests:
        process = subprocess.Popen(["git", "cat-file", "--batch"], stdin=requests, stdout=subprocess.PIPE)
        for oid in names:
            header = process.stdout.readline().split()
            if header[1:] == [b"missing"]:
                found["missing"].append(oid)
                continue
            if len(header) != 3 or header[0].decode("ascii") != oid:
                raise ValueError("unexpected object listing")
            remaining = int(header[2])
            with open(out / "blobs" / oid, "wb") as target:
                while remaining:
                    chunk = process.stdout.read(min(remaining, 1 << 20))
                    if not chunk:
                        raise EOFError("truncated object")
                    target.write(chunk)
                    remaining -= len(chunk)
            process.stdout.read(1)
            base = str(out / "zips" / oid)
            pfx, readable = inspect_file(out / "blobs" / oid, names[oid], base, text=True)
            if os.path.exists(base + ".names"):
                fold_file(base + ".names")
            if pfx:
                found["pkcs12"].append(oid)
            if not readable:
                found["unreadable"].append(oid)
        if process.wait() != 0:
            raise OSError("object inspection failed")
    for kind, oids in found.items():
        (out / kind).write_text("".join(oid + "\n" for oid in oids))


def artifacts(listing, out):
    """Inspect NUL-separated artifact files; results are named by position."""
    paths = [path for path in pathlib.Path(listing).read_bytes().split(b"\0") if path]
    for number, path in enumerate(paths, 1):
        base = os.path.join(out, str(number))
        pathlib.Path(base + ".names").write_bytes(path + b"\n")
        try:
            pfx, readable = inspect_file(path, [os.fsdecode(path)], base)
        except OSError:
            pfx, readable = False, False
        fold_file(base + ".names")
        if pfx:
            pathlib.Path(base + ".pkcs12").touch()
        if not readable:
            pathlib.Path(base + ".unreadable").touch()


if __name__ == "__main__":
    mode, arguments = sys.argv[1], sys.argv[2:]
    if mode == "git":  # entries, output folder, name lists to fold
        for argument in arguments[2:]:
            fold_file(argument)
        lines = pathlib.Path(arguments[0]).read_bytes().split(b"\n")
        plain(lines, os.path.join(arguments[1], "plain"))
        blobs(lines, arguments[1])
    elif mode == "fold":
        for argument in arguments:
            fold_file(argument)
    else:
        artifacts(*arguments)
PY

git rev-list --all >"$scratch/revisions" || exit 1
if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
	report "complete Git history is available" "shallow checkout: fetch the complete history before publication"
else
	report "complete Git history is available" ""
fi
# Entries are "<index|commit> <mode> <type> <object>\t<path>"; each distinct
# blob is read and inspected once for the index and every revision.
git ls-files -s >"$scratch/index" || exit 1
git ls-files >"$scratch/index-names" || exit 1
LC_ALL=C awk -F'\t' '{ split($1, f, " "); print "index " f[1] " " (f[1] == "160000" ? "commit" : "blob") " " f[2] "\t" $2 }' "$scratch/index" >"$scratch/entries"
mkdir "$scratch/meta"
: >"$scratch/meta-failed"
while IFS= read -r rev; do
	git ls-tree -r "$rev" >"$scratch/tree" || exit 1
	LC_ALL=C sed "s/^/$rev /" "$scratch/tree" >>"$scratch/entries"
	git show -s --format='%an <%ae>%n%cn <%ce>%n%B%n' "$rev" >"$scratch/meta/$rev" 2>/dev/null || printf '%s\n' "$rev" >>"$scratch/meta-failed"
done <"$scratch/revisions"
# Annotated tags carry their own messages and may point at trees or blobs
# that no commit reaches.
git for-each-ref --format='%(objectname)' refs/tags >"$scratch/tags" || exit 1
while IFS= read -r object; do
	while type=$(git cat-file -t "$object" 2>/dev/null) && [ "$type" = tag ]; do
		git cat-file tag "$object" >"$scratch/meta/$object" 2>/dev/null || printf '%s\n' "$object" >>"$scratch/meta-failed"
		object=$(sed -n '1s/^object //p' "$scratch/meta/$object")
	done
	case "$type" in
		commit) ;;
		tree)
			git ls-tree -r "$object" >"$scratch/tree" || exit 1
			LC_ALL=C sed "s/^/$object /" "$scratch/tree" >>"$scratch/entries"
			;;
		blob) printf '%s 100644 blob %s\t%s\n' "$object" "$object" "[tagged blob]" >>"$scratch/entries" ;;
		*) printf '%s\n' "$object" >>"$scratch/meta-failed" ;;
	esac
done <"$scratch/tags"
LC_ALL=C awk -F'\t' '!/^index / { print $2 }' "$scratch/entries" | sort -u >"$scratch/all-names"
objects=$scratch/objects
mkdir "$objects"
for list in plain missing unreadable pkcs12; do : >"$objects/$list"; done
: >"$scratch/content-errors"
inspection_failed() { printf '%s\n' '[file content inspection failed]' >>"$scratch/content-errors"; }
# One pass folds the name lists, finds names Git quotes and reads every blob.
python3 "$helper" git "$scratch/entries" "$objects" "$scratch/index-names" "$scratch/all-names" 2>/dev/null || inspection_failed
report "Git path names are literal UTF-8 in the index and every revision" "$( { sort -u "$objects/plain"; if [ -s "$scratch/content-errors" ]; then echo '[file name inspection failed]'; fi; } )"

git_matches() {
	git grep "$@" 2>"$scratch/grep-errors"
	local result=$?
	if [ "$result" -gt 1 ] || [ -s "$scratch/grep-errors" ]; then printf '%s\n' '[file content inspection failed]'; fi
}
matching_objects() { # matching_objects <grep options...> <directory>: IDs of matching object files
	grep -r -l -a "$@" >"$scratch/matching" 2>/dev/null
	if [ "$?" -gt 1 ]; then inspection_failed; fi
	sed 's|.*/||; s|\..*||' "$scratch/matching"
}
# blob_items <index|history> <object-list>: "<item>\t<object>\t<path>" for each
# listed or unreadable blob, where an item is "path" or "commit:path".
blob_items() {
	LC_ALL=C awk -F'\t' -v scope="$1" '
		FILENAME == ARGV[1] { listed[$1]; next }
		FILENAME == ARGV[2] { missing[$1]; next }
		{
			split($1, f, " ")
			if (f[3] != "blob" || (f[1] == "index") != (scope == "index")) next
			item = (f[1] == "index" ? "" : f[1] ":") $2
			if (f[4] in missing) print item ": [file content inspection failed]\t" f[4] "\t" $2
			else if (f[4] in listed) print item "\t" f[4] "\t" $2
		}' "$2" "$objects/missing" "$scratch/entries"
}
blob_findings() { { blob_items "$@" | cut -f1; cat "$scratch/content-errors"; } | sort -u; }
names_matching() { # names_matching <names> <grep options...>: names whose literal or folded form matches
	local names=$1
	shift
	if [ ! -f "$names.folded" ]; then printf '%s\n' '[file name inspection failed]'; return; fi
	{ grep -a -n "$@" "$names"; grep -a -n "$@" "$names.folded"; } | cut -d: -f1 | sort -un |
		LC_ALL=C awk 'NR == FNR { wanted[$1]; next } FNR in wanted' - "$names"
}
meta_hits() { # meta_hits <grep options...>: commits whose metadata matches or cannot be read
	grep -r -l -a "$@" "$scratch/meta" >"$scratch/meta-matching" 2>/dev/null
	if [ "$?" -gt 1 ]; then printf '%s\n' '[metadata inspection failed]'; fi
	{ sed 's|.*/||' "$scratch/meta-matching"; sed 's/$/: [metadata inspection failed]/' "$scratch/meta-failed"; } | sort -u
}

# Archive members are judged like artifacts: secrets in names or bytes, PFX
# members, private names, private development content and the marker.
{
	matching_objects -i -E -f "$secrets" "$objects/blobs"
	matching_objects --include='*.names' -i -E -f "$secrets" "$objects/zips"
	matching_objects --include='*.data' -i -E -f "$secrets" "$objects/zips"
	cat "$objects/pkcs12"
} >"$scratch/hits-secrets"
{
	matching_objects -i -E -f "$private_content" "$objects/blobs"
	matching_objects --include='*.data' -i -E -f "$private_content" "$objects/zips"
	matching_objects --include='*.data' -F -e "$development_marker" "$objects/zips"
} >"$scratch/hits-private-content"
names_matching "$scratch/all-names" -i -E -e "$private_re" >"$scratch/private-paths"
blob_findings history "$scratch/hits-private-content" | grep -a -v -F '[file content inspection failed]' | sed 's/^[a-f0-9]*://' >"$scratch/private-content-paths"
# Retain historical basenames even after deletion from the current branch.
cat "$scratch/private-paths" "$scratch/private-content-paths" | sed 's|.*/||' | grep -a -v -x -F '' | sort -u >"$scratch/private-names"
if [ -s "$scratch/private-names" ]; then python3 "$helper" fold "$scratch/private-names" 2>/dev/null; fi
{
	matching_objects --include='*.names' -i -E -e "$private_re" "$objects/zips"
	matching_objects --include='*.folded' -i -E -e "$private_re" "$objects/zips"
	if [ -s "$scratch/private-names" ]; then
		matching_objects --include='*.names' -i -F -f "$scratch/private-names" "$objects/zips"
		matching_objects --include='*.folded' -i -F -f "$scratch/private-names.folded" "$objects/zips"
	fi
} >"$scratch/hits-archive-names"

report "tracked and historical archives can be inspected" "$( { blob_findings index "$objects/unreadable"; blob_findings history "$objects/unreadable"; } | sort -u)"
report "no private files tracked" "$( { names_matching "$scratch/index-names" -i -E -e "$private_re"; blob_findings index "$scratch/hits-archive-names"; } | sort -u)"
report "no private file names in history" "$( { cat "$scratch/private-paths"; blob_findings history "$scratch/hits-archive-names"; } | sort -u)"
report "no private development content tracked" "$( { blob_findings index "$scratch/hits-private-content"; git_matches -l -i -E -f "$private_content"; } | sort -u)"
report "no private development content in history" "$(blob_findings history "$scratch/hits-private-content")"
report "secrets: tracked files" "$( { blob_findings index "$scratch/hits-secrets"; git_matches -l -i -E -f "$secrets"; } | sort -u)"
report "secrets: tracked file names" "$(names_matching "$scratch/index-names" -i -E -f "$secrets")"
report "secrets: history" "$(blob_findings history "$scratch/hits-secrets")"
report "secrets: historical file names" "$(names_matching "$scratch/all-names" -i -E -f "$secrets")"
report "secrets: commit metadata and trailers" "$(meta_hits -i -E -f "$secrets")"
report "no co-author or session trailers in commit messages" "$(meta_hits -i -E -e "$trailer_re")"

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
			# A line may start with "brand:" to mark a product or tool name.
			# Every term applies to the whole repository, its history, file,
			# branch and tag names and commit metadata; only the pinned
			# third-party statistics runtime image is spared brand terms in
			# built artifacts (see artifact_terms).
			terms=$scratch/all-terms
			confidential_terms=$scratch/confidential-terms
			sed -E 's/^brand://' "$AUDIT_TERMS_FILE" | grep -a -v -x -F '' >"$terms"
			grep -a -v -E '^brand:' "$AUDIT_TERMS_FILE" | grep -a -v -x -F '' >"$confidential_terms"
			{
				matching_objects -i -E -f "$terms" "$objects/blobs"
				matching_objects --include='*.data' -i -E -f "$terms" "$objects/zips"
			} >"$scratch/hits-terms"
			report "terms: tracked files (1)" "$( { blob_findings index "$scratch/hits-terms"; git_matches -l -i -E -f "$terms"; } | sort -u)"
			report "terms: file names (1)" "$(names_matching "$scratch/index-names" -i -E -f "$terms")"
			report "terms: every revision (2)" "$(blob_findings history "$scratch/hits-terms")"
			report "terms: historical file names (2)" "$(names_matching "$scratch/all-names" -i -E -f "$terms")"
			report "terms: branch and tag names (2)" "$(git for-each-ref --format='%(refname)' | grep -a -i -E -f "$terms")"
			report "terms: commit metadata and trailers (2)" "$(meta_hits -i -E -f "$terms")"
		fi
	fi
elif [ "${AUDIT_REQUIRE_TERMS:-}" = 1 ]; then
	report "unwanted terms file is readable and nonempty" "AUDIT_TERMS_FILE is not set and AUDIT_REQUIRE_TERMS=1"
else
	echo "WARN  unwanted terms were not checked (AUDIT_TERMS_FILE not set)"
fi

text_runs() { strings -a -e S -n 6 "$1" && strings -a -e l -n 6 "$1"; }
private_basename_text() {
	[ -s "$scratch/private-names" ] && grep -a -i -q -F -f "$scratch/private-names" "$1"
}
private_entry_names() { # private_entry_names <names>: literal and folded entry names
	local names
	for names in "$1" "$1.folded"; do
		if grep -a -i -q -E "$private_re" "$names"; then return 0; fi
		if grep -a -i -q -E '(^|/)development/continuity(/|$)' "$names"; then return 0; fi
	done
	private_basename_text "$1" || { [ -s "$scratch/private-names.folded" ] && grep -a -i -q -F -f "$scratch/private-names.folded" "$1.folded"; }
}
known='GetLargePage[M]inimum'
if [ -n "$terms" ]; then
	# Use explicit text boundaries to avoid accidental minified-symbol matches.
	sed -E -e 's/^\\b/(^|[[:space:]"'"'"'>])/' -e 's/\\b$/([[:space:]"'"'"'<.,;:!?)-]|$)/' "$terms" >"$scratch/artifact-terms"
	sed -E -e 's/^\\b/(^|[[:space:]"'"'"'>])/' -e 's/\\b$/([[:space:]"'"'"'<.,;:!?)-]|$)/' "$confidential_terms" >"$scratch/artifact-confidential-terms"
fi
# The bundled statistics runtime is pinned third-party R content whose
# citations and package names can contain a brand word by coincidence (a
# person's name, a package name). Only brand terms are not applied there.
artifact_terms() {
	case "$1" in */statistics-engine/vfs/*) printf '%s\n' "$scratch/artifact-confidential-terms" ;; *) printf '%s\n' "$scratch/artifact-terms" ;; esac
}

private_artifacts="" secret_artifacts="" term_artifacts="" path_artifacts="" unreadable_artifacts=""
if [ "$#" -gt 0 ]; then
	if ! command -v strings >/dev/null 2>&1; then
		report "artifact text inspection is available" "strings is required"
		exit "$fail"
	fi
	build_home=${HOME:-/nonexistent}
	: >"$scratch/artifact-files"
	for p in "$@"; do
		if [ ! -e "$p" ]; then unreadable_artifacts+="$p"$'\n'; continue; fi
		if ! find -L "$p" -type f -print0 >>"$scratch/artifact-files" 2>/dev/null; then unreadable_artifacts+="$p"$'\n'; fi
	done
	# Inspect ZIP entries, decompressed bytes and key stores without extracting files.
	mkdir "$scratch/artifacts"
	if ! python3 "$helper" artifacts "$scratch/artifact-files" "$scratch/artifacts" 2>/dev/null; then unreadable_artifacts+='[artifact inspection failed]'$'\n'; fi
	number=0
	while IFS= read -r -d '' f; do
		number=$((number + 1))
		item=$scratch/artifacts/$number
		if [ ! -r "$f" ]; then unreadable_artifacts+="$f"$'\n'; continue; fi
		if ! text_runs "$f" >"$scratch/artifact-text" 2>/dev/null; then unreadable_artifacts+="$f"$'\n'; continue; fi
		if [ -e "$item.unreadable" ]; then unreadable_artifacts+="$f"$'\n'; fi
		if [ -s "$item.data" ]; then
			if ! text_runs "$item.data" >>"$scratch/artifact-text" 2>/dev/null; then unreadable_artifacts+="$f"$'\n'; fi
			if grep -a -q -F "$build_home/" "$item.data"; then path_artifacts+="$f"$'\n'; fi
		fi
		if private_entry_names "$item.names" || private_basename_text "$scratch/artifact-text" || grep -a -i -q -E -f "$private_content" "$scratch/artifact-text" || grep -a -q -F "$development_marker" "$scratch/artifact-text"; then private_artifacts+="$f"$'\n'; fi
		if [ -e "$item.pkcs12" ] || grep -a -i -q -E -f "$secrets" "$scratch/artifact-text" || grep -a -i -q -E -f "$secrets" "$item.names"; then secret_artifacts+="$f"$'\n'; fi
		if [ -n "$terms" ] && term_file=$(artifact_terms "$f") && [ -s "$term_file" ] && sed -E "s/($known)//g" "$scratch/artifact-text" | grep -a -i -q -E -f "$term_file"; then term_artifacts+="$f"$'\n'; fi
		if grep -a -q -F "$build_home/" "$f"; then path_artifacts+="$f"$'\n'; fi
	done <"$scratch/artifact-files"
	report "artifacts can be inspected" "${unreadable_artifacts%$'\n'}"
	report "no private file names in artifacts or ZIP entries" "${private_artifacts%$'\n'}"
	report "secrets: built artifacts" "${secret_artifacts%$'\n'}"
	if [ -n "$terms" ]; then report "terms: built artifacts (3)" "${term_artifacts%$'\n'}"; fi
	report "no build-machine paths in artifacts" "$(printf '%s' "$path_artifacts" | sort -u)"
elif [ -n "$terms" ]; then
	echo "skip  terms: built artifacts (3) (no artifacts given)"
fi

exit "$fail"
