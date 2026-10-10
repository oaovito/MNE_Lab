"""Independent invented unsigned ZIP/JSON fixtures; Python standard library only."""
from pathlib import Path
import hashlib, json, zipfile
p=Path(__file__).resolve().parent
for version in (1,2):
 files={'original-files/invented.txt':b'invented source 1.2300\n'}
 readme=b'Invented unsigned package fixture.\n'
 if version==2:files['README.txt']=readme
 entries=[{'path':name,'size':len(data),'sha256':hashlib.sha256(data).hexdigest(),'role':'readme' if name=='README.txt' else 'original'} for name,data in sorted(files.items())]
 m={'format':f'mnelab-package/{version}','kind':'dataset','title':'Invented package','software':'MNE Lab synthetic fixture','schema':1,'created':'2026-10-10T00:00:00Z','files':entries}
 content=dict(files)
 if version==1:content['README.txt']=readme
 content['manifest/manifest.json']=(json.dumps(m,indent=2)+'\n').encode()
 content['manifest/checksums.sha256']=''.join(e['sha256']+'  '+e['path']+'\n' for e in entries).encode()
 with zipfile.ZipFile(p/f'invented-v{version}.zip','w')as z:
  for name,data in sorted(content.items()):
   info=zipfile.ZipInfo('InventedPackage/'+name,(2026,10,10,0,0,0));info.create_system=3;info.external_attr=0o100644<<16;z.writestr(info,data)
 print(f'invented-v{version}.zip',hashlib.sha256((p/f'invented-v{version}.zip').read_bytes()).hexdigest())
