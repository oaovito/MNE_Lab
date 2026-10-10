// Build supply-chain regression: corrupted cached bytes must never reach an
// application asset. This invokes the actual assembler with a modified TGZ.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
const web=fileURLToPath(new URL('..',import.meta.url));
const root=path.resolve(web,'..');
const pins=JSON.parse(fs.readFileSync(path.join(root,'scripts/statistics-packages.json')));
test('corrupted native package cache is rejected before image creation',()=>{
  const temp=fs.mkdtempSync(path.join(os.tmpdir(),'mnelab-package-integrity-'));
  try {
    const cache=process.env.MNELAB_R_PACKAGE_CACHE || path.join(web,'.statistics-package-cache');
    const p=pins.packages[0];const bytes=fs.readFileSync(path.resolve(web,cache,p.file));
    bytes[bytes.length-1]^=1;fs.writeFileSync(path.join(temp,p.file),bytes);
    const out=path.join(temp,'image');
    const result=spawnSync('go',['run','scripts/statistics_packages.go','-manifest','scripts/statistics-packages.json','-cache',temp,'-out',out],{cwd:root,encoding:'utf8',timeout:60000});
    assert.equal(result.status,1,result.error?.message);
    assert.match(result.stderr,/package checksum mismatch: MASS/);
    assert.equal(fs.existsSync(out),false,'corrupted package produced an image');
  } finally {fs.rmSync(temp,{recursive:true,force:true});}
});
