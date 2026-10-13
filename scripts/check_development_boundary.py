#!/usr/bin/env python3
"""Check the Git-only continuity boundary without building the application.

Usage, from the repository root:
  python3 scripts/check_development_boundary.py --repo .
  python3 scripts/check_development_boundary.py --repo . --artifacts out web/dist
  git archive --format=zip HEAD | python3 scripts/check_development_boundary.py --archive-stdin

Only the fixed continuity documents may be authorized in Git. They must be
regular UTF-8 files, carry the development marker, and have export-ignore in
the corresponding index/tree. Every reachable revision is checked. This check
does not exempt credentials or experimental corpus from the privacy audit.
Artifacts, including nested ZIPs and renamed documents, must exclude context.
"""

from __future__ import annotations

import argparse
import io
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import zipfile


# Assemble the marker so the checker itself can accompany a source archive.
MARKER = "MNE_LAB_" + "DEVELOPMENT_ONLY"
MD_MARKER = f"<!-- {MARKER} -->"
PREFIX = "development/continuity"
ALLOWED = frozenset({
    "AGENTS.md", "CLAUDE.md",
    *(f"{PREFIX}/{name}" for name in (
        "README.md", "MASTER_SPECIFICATION.md", "USER_DIRECTIVES.md",
        "CONTEXT.md", "CHECKPOINT.json", "TASKS.json", "MATERIALS.md",
        "PROMPT.md", "REQUIREMENTS_REVIEW.md", "VALIDATION.json",
    )),
})
ARCHIVE_LIMIT = 512 * 1024 * 1024
DOCUMENT_LIMIT = 4 * 1024 * 1024


class BoundaryError(Exception):
    pass


def git(repo: Path, *arguments: str) -> bytes:
    environment = dict(os.environ)
    environment["GIT_ATTR_NOSYSTEM"] = "1"
    result = subprocess.run(
        ["git", *arguments], cwd=repo, env=environment, capture_output=True, check=False,
    )
    if result.returncode != 0:
        # Never echo Git output or unknown identifiers: they can contain secrets.
        raise BoundaryError("Git boundary inspection failed")
    return result.stdout


def protected(path: str) -> bool:
    return path in {"AGENTS.md", "CLAUDE.md"} or path == PREFIX or path.startswith(PREFIX + "/")


def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    value: dict[str, object] = {}
    for key, item in pairs:
        if key in value:
            raise BoundaryError("Duplicate JSON key in continuity document")
        value[key] = item
    return value


def validate_document(path: str, raw: bytes, mode: str) -> None:
    if path not in ALLOWED:
        raise BoundaryError("Unapproved entry under development/continuity")
    if mode not in {"100644", "100755"}:
        raise BoundaryError(f"{path}: continuity document must be a regular file")
    if len(raw) > DOCUMENT_LIMIT or b"\0" in raw:
        raise BoundaryError(f"{path}: invalid continuity document size or NUL byte")
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise BoundaryError(f"{path}: continuity document must be UTF-8") from exc
    if path.endswith(".md"):
        if not text.splitlines() or text.splitlines()[0] != MD_MARKER:
            raise BoundaryError(f"{path}: missing first-line development marker")
    else:
        try:
            value = json.loads(text, object_pairs_hook=unique_object)
        except (ValueError, BoundaryError) as exc:
            raise BoundaryError(f"{path}: invalid continuity JSON") from exc
        if not isinstance(value, dict) or value.get("development_only") != MARKER:
            raise BoundaryError(f"{path}: missing JSON development marker")


def attributes(repo: Path, paths: list[str], source: str | None) -> None:
    if not paths:
        return
    flags = ["--cached"] if source == "index" else ([f"--source={source}"] if source else [])
    raw = git(repo, "-c", f"core.attributesFile={os.devnull}", "check-attr", "-z", *flags, "export-ignore", "--", *paths)
    fields = raw.split(b"\0")[:-1]
    if len(fields) != 3 * len(paths):
        raise BoundaryError("Incomplete export-ignore attribute inspection")
    values = {fields[index].decode("utf-8"): fields[index + 2] for index in range(0, len(fields), 3)}
    for path in paths:
        if values.get(path) != b"set":
            raise BoundaryError(f"{path}: export-ignore is required in the corresponding Git tree/index")


def source_check(repo: Path) -> dict[str, int]:
    local_attributes = Path(git(repo, "rev-parse", "--git-path", "info/attributes").decode("utf-8").strip())
    if not local_attributes.is_absolute():
        local_attributes = repo / local_attributes
    if local_attributes.exists():
        if any(line.strip() and not line.lstrip().startswith("#") for line in local_attributes.read_text(encoding="utf-8").splitlines()):
            raise BoundaryError("Local info/attributes overrides cannot certify committed continuity exclusions")
    checked = 0
    revisions = git(repo, "rev-list", "--all").decode("ascii").splitlines()
    entries = git(repo, "ls-files", "--stage", "-z").split(b"\0")
    current: list[str] = []
    for entry in entries:
        if not entry:
            continue
        metadata, name = entry.split(b"\t", 1)
        path = name.decode("utf-8", errors="surrogateescape")
        if not protected(path):
            continue
        mode, blob, stage = metadata.decode("ascii").split()
        if stage != "0":
            raise BoundaryError("Unmerged continuity document in the index")
        validate_document(path, git(repo, "cat-file", "blob", blob), mode)
        target = repo / path
        if target.is_symlink() or not target.is_file():
            raise BoundaryError(f"{path}: working document must be a regular file")
        validate_document(path, target.read_bytes(), "100644")
        current.append(path)
        checked += 2
    attributes(repo, current, "index")
    attributes(repo, current, None)
    for revision in revisions:
        paths: list[str] = []
        for entry in git(repo, "ls-tree", "-r", "-z", "--full-tree", revision).split(b"\0"):
            if not entry:
                continue
            metadata, name = entry.split(b"\t", 1)
            path = name.decode("utf-8", errors="surrogateescape")
            if not protected(path):
                continue
            mode, kind, blob = metadata.decode("ascii").split()
            if kind != "blob":
                raise BoundaryError("Continuity document must be a blob")
            validate_document(path, git(repo, "cat-file", "blob", blob), mode)
            paths.append(path)
            checked += 1
        attributes(repo, paths, revision)
    return {"reachable_revisions": len(revisions), "document_versions_checked": checked}


def artifact_name(path: str) -> bool:
    normalized = path.replace("\\", "/")
    parts = tuple(part.lower() for part in PurePosixPath(normalized).parts)
    return (
        any(part in {"agents.md", "claude.md"} for part in parts)
        or any(parts[index:index + 2] == ("development", "continuity") for index in range(len(parts) - 1))
    )


def validate_payload(name: str, raw: bytes) -> None:
    if artifact_name(name):
        raise BoundaryError("Development-only path is present in a product artifact")
    if any(MARKER.encode(encoding) in raw for encoding in ("utf-8", "utf-16le", "utf-16be")):
        raise BoundaryError("Development-only content is present in a product artifact")


def archive_check(raw: bytes) -> int:
    total = 0
    members = 0

    def inspect(content: bytes, depth: int) -> None:
        nonlocal total, members
        if depth > 16 or len(content) > ARCHIVE_LIMIT:
            raise BoundaryError("Product archive inspection limit exceeded")
        try:
            with zipfile.ZipFile(io.BytesIO(content)) as archive:
                for entry in archive.infolist():
                    members += 1
                    if artifact_name(entry.filename):
                        raise BoundaryError("Development-only path is present in a product archive")
                    if entry.is_dir():
                        continue
                    total += entry.file_size
                    if total > ARCHIVE_LIMIT:
                        raise BoundaryError("Product archive expansion limit exceeded")
                    data = archive.read(entry)
                    validate_payload(entry.filename, data)
                    if data.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")) or entry.filename.lower().endswith((".zip", ".xlsx")):
                        inspect(data, depth + 1)
        except (OSError, ValueError, RuntimeError, NotImplementedError, zipfile.BadZipFile) as exc:
            raise BoundaryError("Product archive could not be inspected") from exc

    inspect(raw, 0)
    return members


def artifacts_check(paths: list[Path]) -> dict[str, int]:
    files = 0
    members = 0
    for path in paths:
        if not path.exists():
            raise BoundaryError("Requested product artifact is missing")
        if path.is_symlink():
            raise BoundaryError("Product artifact inspection requires regular files/directories")
        targets = [path] if path.is_file() else sorted(path.rglob("*"))
        for target in targets:
            if target.is_symlink():
                raise BoundaryError("Product artifact inspection cannot skip symlink entries")
            if not target.is_file():
                continue
            if target.stat().st_size > ARCHIVE_LIMIT:
                raise BoundaryError("Product artifact inspection limit exceeded")
            raw = target.read_bytes()
            validate_payload(str(target), raw)
            if raw.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")) or target.suffix.lower() in {".zip", ".xlsx"}:
                members += archive_check(raw)
            files += 1
    return {"artifact_files_checked": files, "archive_members_checked": members}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path.cwd())
    parser.add_argument("--artifacts", type=Path, nargs="*", default=[])
    parser.add_argument("--artifacts-only", action="store_true", help="Check products without inspecting source/history")
    parser.add_argument("--archive-stdin", action="store_true", help="Check ZIP bytes from stdin without Git inspection")
    arguments = parser.parse_args()
    try:
        if arguments.archive_stdin:
            raw = sys.stdin.buffer.read(ARCHIVE_LIMIT + 1)
            result = {"archive_members_checked": archive_check(raw)}
        elif arguments.artifacts_only:
            result = artifacts_check(arguments.artifacts)
        else:
            result = source_check(arguments.repo.resolve())
            result.update(artifacts_check(arguments.artifacts))
    except BoundaryError as exc:
        print(f"Development boundary rejected: {exc}", file=sys.stderr)
        return 1
    except (OSError, ValueError):
        print("Development boundary rejected: inspection failed", file=sys.stderr)
        return 1
    print(json.dumps({"development_boundary_passed": True, **result}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
