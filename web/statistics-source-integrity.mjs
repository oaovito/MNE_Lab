// Build-only integrity gate shared by build.mjs and supply-chain tests.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
export function verifyStatisticsSources(root){
 const manifest=JSON.parse(fs.readFileSync(path.join(root,'manifest.json'),'utf8'));
 const allowed=['ci.pvaf.R','conf.limits.ncf.R','ci.pvaf.Rd','conf.limits.ncf.Rd','DESCRIPTION'];
 if(manifest.version!=='5.0.1'||manifest.files.length!==allowed.length||new Set(manifest.files.map(f=>f.path)).size!==allowed.length)throw new Error('Invalid MBESS source manifest');
 for(const file of manifest.files){
  if(!allowed.includes(file.path))throw new Error('Invalid MBESS source manifest path');
  const bytes=fs.readFileSync(path.join(root,file.path));
  if(bytes.length!==file.size||createHash('sha256').update(bytes).digest('hex')!==file.sha256)throw new Error('MBESS pinned source checksum mismatch');
 }
}
