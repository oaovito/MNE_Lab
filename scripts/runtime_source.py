"""Collect the statistical runtime's source for a binary release.

  python3 scripts/runtime_source.py --version 0.1.0-dev.1 --out DIR

Downloads the R source, the source of every pinned R package and the webR
checkout (with its build-script submodule) at the commits the embedded
runtime was built from, checks every recorded SHA-256 and writes one tar
archive with a SOURCES.json inventory. Nothing is rebuilt.
"""
from pathlib import Path
import argparse, hashlib, io, json, subprocess, tarfile, tempfile, urllib.request

R_SOURCE = ('R-4.6.0.tar.gz', 'https://cloud.r-project.org/src/base/R-4/R-4.6.0.tar.gz',
            'b8dc9b4543660c7b596b87938df532394350360976527d344228ee0ed12e45ec')
WEBR = ('https://github.com/r-wasm/webr.git', 'f116a5e60e220182d09ac015837c88c454c663c5')
WEBR_SUBMODULE = '25541509519240557c6c7bc695f3c9615366c219'
NOTES = """Source of the statistical runtime shipped with MNE Lab {version}

The application embeds the unmodified webR 0.6.0 distribution (R 4.6.0 built
for WebAssembly) and the R packages listed in SOURCES.json, as published by
the webR project. This archive holds:

- webr/: the webR source at commit {webr}, including the build scripts and
  patches it applies to R and its components, and its submodule(s) at the
  recorded commits;
- R-4.6.0.tar.gz: the R source the webR recipes build;
- packages/: the source of every R package in the runtime image.

webR's recipes name Emscripten 5.0.7 and a flang-wasm base image; see
webr/README.md for the build instructions. The application's own R code is
web/src/lib/statistics-engine.R in the application source archive.
"""


def fetch(url):
    req = urllib.request.Request(url, headers={'User-Agent': 'mnelab-release'})
    with urllib.request.urlopen(req, timeout=120) as r:
        return r.read()


def git(*args, cwd=None):
    return subprocess.run(['git', *args], cwd=cwd, check=True, capture_output=True).stdout


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument('--repo', type=Path, default=Path.cwd())
    p.add_argument('--out', type=Path, required=True)
    p.add_argument('--version', required=True)
    a = p.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    pins = json.loads((a.repo / 'scripts/statistics-packages.json').read_text())
    members, modes, inventory = {}, {}, {'version': a.version, 'runtime': pins['runtime'], 'files': []}

    def add(path, raw, origin, expected=None, note=None):
        digest = hashlib.sha256(raw).hexdigest()
        if expected is not None and digest != expected:
            raise SystemExit(f'{path}: SHA-256 {digest} does not match the recorded {expected}')
        members[path] = raw
        entry = {'path': path, 'size': len(raw), 'sha256': digest, 'origin': origin,
                 'matches_recorded_sha256': expected is not None}
        if note:
            entry['note'] = note
        inventory['files'].append(entry)

    name, url, digest = R_SOURCE
    add(name, fetch(url), url, digest)

    for pkg in pins['packages']:
        src = pkg['source']
        path = 'packages/' + src['file']
        if 'commit' in src:
            # Archived CRAN versions are tried first; the recorded commit of
            # the CRAN mirror is the fallback, with its own hash.
            archive = f"https://cloud.r-project.org/src/contrib/Archive/{src['package']}/{src['file']}"
            try:
                raw = fetch(archive)
            except OSError:
                raw = None
            if raw is not None and hashlib.sha256(raw).hexdigest() == src['sha256']:
                add(path, raw, archive, src['sha256'])
            else:
                github = f"https://github.com/cran/{src['package']}/archive/{src['commit']}.tar.gz"
                add(path, fetch(github), github, None,
                    'GitHub archive of the recorded CRAN mirror commit; the recorded tarball hash was not reproduced from the CRAN archive')
        else:
            add(path, fetch(src['url']), src['url'], src['sha256'])

    with tempfile.TemporaryDirectory() as tmp:
        repo, commit = WEBR
        checkout = Path(tmp) / 'webr'
        git('clone', '--quiet', '--filter=blob:none', '--no-checkout', repo, str(checkout))
        git('checkout', '--quiet', commit, cwd=checkout)
        git('submodule', 'update', '--init', '--recursive', '--quiet', cwd=checkout)
        subs = git('submodule', 'status', '--recursive', cwd=checkout).decode().split('\n')
        subs = [s.split() for s in subs if s.strip()]
        assert any(s[0].lstrip('+-U') == WEBR_SUBMODULE for s in subs), subs
        inventory['webr'] = {'repository': repo, 'commit': commit,
                             'submodules': [{'path': s[1], 'commit': s[0].lstrip('+-U')} for s in subs]}
        archives = [('webr/', git('archive', '--format=tar', 'HEAD', cwd=checkout))]
        for s in subs:
            archives.append((f'webr/{s[1]}/', git('archive', '--format=tar', 'HEAD', cwd=checkout / s[1])))
        for prefix, data in archives:
            with tarfile.open(fileobj=io.BytesIO(data)) as t:
                for m in t.getmembers():
                    if m.isfile():
                        modes[prefix + m.name] = m.mode
                        members[prefix + m.name] = t.extractfile(m).read()

    members['README-SOURCE.txt'] = NOTES.format(version=a.version, webr=WEBR[1]).encode()
    members['SOURCES.json'] = (json.dumps(inventory, indent=2) + '\n').encode()
    target = a.out / f'MNE_Lab-{a.version}-statistical-runtime-source.tar'
    root = f'MNE_Lab-{a.version}-statistical-runtime-source/'
    with tarfile.open(target, 'w', format=tarfile.PAX_FORMAT) as t:
        for path, raw in sorted(members.items()):
            info = tarfile.TarInfo(root + path)
            info.size, info.mode, info.mtime = len(raw), modes.get(path, 0o644), 0
            t.addfile(info, io.BytesIO(raw))
    print(json.dumps({'archive': target.name, 'size': target.stat().st_size,
                      'sha256': hashlib.sha256(target.read_bytes()).hexdigest(),
                      'members': len(members)}))


if __name__ == '__main__':
    main()
