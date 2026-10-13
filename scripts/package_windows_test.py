from pathlib import Path
import hashlib,json,struct,subprocess,sys,zipfile
import argparse
parser=argparse.ArgumentParser(description='Package a private owner-test Windows x64 build; no publishing.')
parser.add_argument('--repo',type=Path,default=Path.cwd());parser.add_argument('--out',type=Path,required=True)
args=parser.parse_args();repo=args.repo.resolve();out=args.out.resolve();out.mkdir(parents=True,exist_ok=True)
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
assert not subprocess.check_output(['git','status','--porcelain'],cwd=repo,text=True).strip()
version='0.1.0-dev.'+head[:7]; root='MNE Lab Windows x64 - '+head[:7]
app=(repo/'out/mnelab-windows-amd64.exe').read_bytes(); launcher=(repo/'out/mnelab-launcher-windows-amd64.exe').read_bytes()
assert version.encode() in app, 'Application VERSION must identify this exact source checkpoint.'
for raw in (app,launcher):
 assert raw[:2]==b'MZ'
 offset=struct.unpack_from('<I',raw,0x3c)[0]
 assert raw[offset:offset+4]==b'PE\0\0'
 assert struct.unpack_from('<H',raw,offset+4)[0]==0x8664
 assert struct.unpack_from('<H',raw,offset+24)[0]==0x20b
 optional=offset+24
 assert struct.unpack_from('<H',raw,optional+68)[0]==2
# Snapshot contains only the clean current tree, not historical refs/corpus.
source=subprocess.check_output(['git','archive','--format=zip','--prefix=MNE_Lab-source/','HEAD'],cwd=repo)
# Attributes must exclude the Git-only handoff from the actual distributed
# snapshot, not merely from a directory listing or a future working-tree rule.
subprocess.run([sys.executable,str(Path(__file__).resolve().with_name('check_development_boundary.py')),'--archive-stdin'],input=source,check=True,capture_output=True)
readme=f"""MNE LAB - TESTE PRIVADO DE DESENVOLVIMENTO - WINDOWS x64

Versao: {version}; canal dev; source HEAD: {head}

1. Extraia TODO o ZIP para uma pasta gravavel; nao execute de dentro do ZIP.
2. Abra 'MNE Lab.exe' na pasta extraida. Este e o inicializador do modo
   portatil; seus dados de teste ficam na subpasta data. Pode usar uma pasta
   local ou pendrive. Preserve a pasta completa ao mover para outra unidade.
3. Windows 10/11 x64 com Edge, Chrome, Brave ou Chromium instalado.
   Nao precisa instalar Go, Node, R, Python ou Microsoft Office.
4. Crie uma conta e perfil DE TESTE, importe uma copia de arquivo, confira
   File Library/Graph/Cycle/Statistics e exporte. Feche e reabra pelo mesmo
   inicializador para testar persistencia. Mantenha seus originais separados.
5. Se falhar, informe Windows/arquitetura, etapa e mensagem apresentada;
   nao envie senha, chaves ou dados pessoais. Os logs locais ficam data/logs.

Binarios de desenvolvimento SEM assinatura Authenticode oficial. Este
pacote nao e Stable nem uma publicacao de release/update automatico.
A validacao de GUI/USB em Windows limpo ainda depende desse teste real.
Statistical Analysis: validado apenas no escopo de STATISTICAL_ANALYSIS.md.
DLS parcial/provisorio; ZETA e NTA cientificamente NAO validados. Consulte
READINESS para o escopo de assinatura e as demais funcionalidades pendentes.
Cloud real depende de configuracao oficial. Nao use este teste como unica
copia dos seus dados. Manual de readiness e escopos acompanha em docs.

Source snapshot do aplicativo incluso: MNE_Lab-source-{head[:7]}.zip.
Licencas upstream e inventario estao em licenses e docs. Reconstrucao completa
Corresponding Source do runtime estatistico continua uma etapa pendente;
este arquivo de teste nao deve ser republicado como uma distribuicao publica.
"""
files={'MNE Lab.exe':launcher,'MNE Lab.portable':b'',f'app/{version}/mnelab.exe':app,'LEIA-ME-TESTE.txt':readme.encode(),f'MNE_Lab-source-{head[:7]}.zip':source}
for name in ('READINESS.md','STATISTICAL_ANALYSIS.md','STATISTICAL_RUNTIME_LICENSES.md','STATISTICAL_PACKAGES_LICENSES.md','PACKAGE_INTEGRITY.md'):
 files['docs/'+name]=(repo/'docs'/name).read_bytes()
for f in (repo/'web/dist/statistics-engine').rglob('*'):
 if f.is_file() and ('license' in f.name.lower() or 'notice' in f.name.lower()): files['licenses/'+str(f.relative_to(repo/'web/dist/statistics-engine'))]=f.read_bytes()
assert any(name.startswith('licenses/') for name in files)
for name in ('CMS_SIGNATURES.md','CMS_DEPENDENCY_LICENSE.txt'):
 if (repo/'docs'/name).is_file():files['docs/'+name]=(repo/'docs'/name).read_bytes()
manifest={'source_head':head,'version':version,'channel':'dev','target':'windows/amd64','authenticode_signed':False,'public_release':False,'native_windows_GUI_tested':False,'scientific_scope_upgraded':False,'files':[{'path':n,'size':len(raw),'sha256':hashlib.sha256(raw).hexdigest()} for n,raw in sorted(files.items())]}
files['BUILD-MANIFEST.json']=(json.dumps(manifest,indent=2)+'\n').encode()
files['SHA256SUMS.txt']=''.join(hashlib.sha256(raw).hexdigest()+'  '+name+'\n' for name,raw in sorted(files.items())).encode()
archive=out/f'MNE_Lab-Windows-x64-teste-{head[:7]}.zip'
with zipfile.ZipFile(archive,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as z:
 for name,raw in sorted(files.items()):z.writestr(root+'/'+name,raw)
with zipfile.ZipFile(archive) as z:
 assert z.testzip() is None
 assert len(z.namelist())==len(files)
 for name,raw in files.items(): assert z.read(root+'/'+name)==raw
subprocess.run([sys.executable,str(Path(__file__).resolve().with_name('check_development_boundary.py')),'--archive-stdin'],input=archive.read_bytes(),check=True,capture_output=True)
receipt={'path':str(archive),'size':archive.stat().st_size,'sha256':hashlib.sha256(archive.read_bytes()).hexdigest(),'source_head':head,'files':len(files),'zip_CRC_and_all_member_bytes_verified':True,'PE_machine_amd64_gui_verified':True,'publicly_uploaded':False,'native_windows_GUI_tested':False}
(out/(archive.name+'.sha256')).write_text(receipt['sha256']+'  '+archive.name+'\n')
(out/'windows-test-package.json').write_text(json.dumps(receipt,indent=2)+'\n')
print(json.dumps(receipt))
