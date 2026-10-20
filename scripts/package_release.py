"""Package built executables as portable release archives; no publishing.

  python3 scripts/package_release.py --version 0.1.0-dev.1 --out DIR TARGET...

TARGET is goos/goarch as built by scripts/build.sh into out/. Each archive
holds the launcher, the portable marker, the application under
app/<version>/, the runtime licenses, the scope documents, a manifest and
SHA-256 sums. Windows archives are ZIP; others are tar.gz, which keeps the
executable bits.
"""
from pathlib import Path
import argparse, hashlib, io, json, struct, subprocess, sys, tarfile, zipfile

NAMES = {
    'windows/amd64': 'Windows-x64', 'windows/386': 'Windows-x86', 'windows/arm64': 'Windows-ARM64',
    'linux/amd64': 'Linux-x64', 'linux/arm64': 'Linux-ARM64',
    'darwin/arm64': 'macOS-AppleSilicon', 'darwin/amd64': 'macOS-Intel',
}
PE_MACHINE = {'amd64': 0x8664, '386': 0x14c, 'arm64': 0xaa64}
ELF_MACHINE = {'amd64': 62, 'arm64': 183}
MACHO_CPU = {'amd64': 0x01000007, 'arm64': 0x0100000c}
DOCS = ('READINESS.md', 'STATISTICAL_ANALYSIS.md', 'STATISTICAL_RUNTIME_LICENSES.md',
        'STATISTICAL_PACKAGES_LICENSES.md', 'PACKAGE_INTEGRITY.md', 'CMS_SIGNATURES.md',
        'CMS_DEPENDENCY_LICENSE.txt')


def check_binary(raw, os_, arch):
    if os_ == 'windows':
        assert raw[:2] == b'MZ'
        off = struct.unpack_from('<I', raw, 0x3c)[0]
        assert raw[off:off + 4] == b'PE\0\0'
        assert struct.unpack_from('<H', raw, off + 4)[0] == PE_MACHINE[arch]
        magic = struct.unpack_from('<H', raw, off + 24)[0]
        # Subsystem 2 is GUI: no console window.
        assert struct.unpack_from('<H', raw, off + 24 + 68)[0] == 2, magic
    elif os_ == 'linux':
        assert raw[:4] == b'\x7fELF'
        assert struct.unpack_from('<H', raw, 18)[0] == ELF_MACHINE[arch]
    else:
        assert raw[:4] == b'\xcf\xfa\xed\xfe'
        assert struct.unpack_from('<I', raw, 4)[0] == MACHO_CPU[arch]


def readme(version, head, os_, name):
    if os_ == 'windows':
        open_step = "Abra 'MNE Lab.exe' na pasta extraida."
        system = 'Windows 10/11 com Edge, Chrome, Brave ou Chromium instalado.'
        unsigned = ("O Windows pode mostrar 'O Windows protegeu o computador': clique em\n"
                    "   'Mais informacoes' e 'Executar assim mesmo' (build sem assinatura Authenticode).")
    elif os_ == 'linux':
        open_step = "Abra o arquivo 'MNE Lab' na pasta extraida (ou ./'MNE Lab' no terminal)."
        system = 'Linux de 64 bits com Chrome, Chromium, Brave ou Edge instalado.'
        unsigned = 'Se o arquivo nao abrir, marque-o como executavel (chmod +x).'
    else:
        open_step = "Abra o arquivo 'MNE Lab' na pasta extraida."
        system = 'macOS 12 ou mais recente com Chrome, Chromium, Brave ou Edge instalado.'
        unsigned = ("Build sem assinatura Apple. Na primeira vez, no Terminal, rode\n"
                    "   xattr -dr com.apple.quarantine \"/caminho/da/pasta/MNE Lab\"\n"
                    "   ou clique com o botao direito em 'MNE Lab' e escolha Abrir.")
    return f"""MNE LAB {version} - PRE-RELEASE DE DESENVOLVIMENTO - {name}

Codigo-fonte: {head}

1. Extraia TODO o arquivo para uma pasta gravavel; nao execute de dentro dele.
2. {open_step}
   Este e o modo portatil: seus dados ficam na subpasta data. Pode usar uma
   pasta local ou um pendrive; mova sempre a pasta completa.
3. {system}
   Nao precisa instalar Go, Node, R, Python ou Microsoft Office.
4. {unsigned}
5. Na primeira abertura, crie uma conta e um perfil. Mantenha copias dos seus
   originais; esta versao e de desenvolvimento.

Escopo: Statistical Analysis validado apenas no escopo de
docs/STATISTICAL_ANALYSIS.md. DLS parcial e provisorio; ZETA e NTA sem
validacao cientifica. Nuvem real depende do registro oficial dos provedores.
Atualizacao automatica nao e oferecida para esta versao.

Licencas do runtime estatistico em licenses/. O codigo-fonte do aplicativo e
o codigo-fonte correspondente do runtime estatistico (webR, R e pacotes) estao
anexados na mesma pagina da release.
"""


def package(repo, out, version, target, head):
    os_, arch = target.split('/')
    name = NAMES[target]
    ext = '.exe' if os_ == 'windows' else ''
    app = (repo / f'out/mnelab-{os_}-{arch}{ext}').read_bytes()
    launcher = (repo / f'out/mnelab-launcher-{os_}-{arch}{ext}').read_bytes()
    assert version.encode() in app, 'The application must carry this exact VERSION.'
    check_binary(app, os_, arch)
    check_binary(launcher, os_, arch)
    files = {
        f'MNE Lab{ext}': (launcher, 0o755),
        'MNE Lab.portable': (b'', 0o644),
        f'app/{version}/mnelab{ext}': (app, 0o755),
        'LEIA-ME.txt': (readme(version, head, os_, name).encode(), 0o644),
    }
    for doc in DOCS:
        if (repo / 'docs' / doc).is_file():
            files['docs/' + doc] = ((repo / 'docs' / doc).read_bytes(), 0o644)
    runtime = repo / 'web/dist/statistics-engine'
    for f in runtime.rglob('*'):
        if f.is_file() and ('license' in f.name.lower() or 'notice' in f.name.lower()):
            files['licenses/' + str(f.relative_to(runtime))] = (f.read_bytes(), 0o644)
    assert any(n.startswith('licenses/') for n in files)
    manifest = {'version': version, 'channel': 'dev', 'source_head': head, 'target': target,
                'code_signed': False, 'update_manifest': False,
                'files': [{'path': n, 'size': len(r), 'sha256': hashlib.sha256(r).hexdigest()}
                          for n, (r, _) in sorted(files.items())]}
    files['BUILD-MANIFEST.json'] = ((json.dumps(manifest, indent=2) + '\n').encode(), 0o644)
    files['SHA256SUMS.txt'] = (''.join(hashlib.sha256(r).hexdigest() + '  ' + n + '\n'
                                       for n, (r, _) in sorted(files.items())).encode(), 0o644)
    root = 'MNE Lab'
    if os_ == 'windows':
        archive = out / f'MNE_Lab-{version}-{name}.zip'
        with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as z:
            for n, (raw, _) in sorted(files.items()):
                z.writestr(root + '/' + n, raw)
        with zipfile.ZipFile(archive) as z:
            assert z.testzip() is None and len(z.namelist()) == len(files)
            for n, (raw, _) in files.items():
                assert z.read(root + '/' + n) == raw
    else:
        archive = out / f'MNE_Lab-{version}-{name}.tar.gz'
        with tarfile.open(archive, 'w:gz', format=tarfile.PAX_FORMAT) as t:
            for n, (raw, mode) in sorted(files.items()):
                info = tarfile.TarInfo(root + '/' + n)
                info.size, info.mode, info.mtime = len(raw), mode, 0
                info.uname = info.gname = ''
                t.addfile(info, io.BytesIO(raw))
        with tarfile.open(archive) as t:
            members = t.getmembers()
            assert len(members) == len(files)
            for m in members:
                raw, mode = files[m.name[len(root) + 1:]]
                assert t.extractfile(m).read() == raw and m.mode == mode
    data = archive.read_bytes()
    if os_ == 'windows':
        subprocess.run([sys.executable, str(Path(__file__).resolve().with_name('check_development_boundary.py')),
                        '--archive-stdin'], input=data, check=True, capture_output=True)
    return {'archive': archive.name, 'target': target, 'size': len(data),
            'sha256': hashlib.sha256(data).hexdigest(), 'files': len(files)}


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument('--repo', type=Path, default=Path.cwd())
    p.add_argument('--out', type=Path, required=True)
    p.add_argument('--version', required=True)
    p.add_argument('targets', nargs='+', choices=sorted(NAMES))
    a = p.parse_args()
    repo, out = a.repo.resolve(), a.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
    assert not subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=no'],
                                       cwd=repo, text=True).strip()
    receipts = [package(repo, out, a.version, t, head) for t in a.targets]
    print(json.dumps(receipts, indent=2))


if __name__ == '__main__':
    main()
