// Origin is a human declaration, never inferred from a readable report.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';

const [dir,outRoot] = process.argv.slice(2);
const out = path.join(outRoot,'producer-origin');
fs.mkdirSync(out,{recursive:true});
const report = new Report('producer-origin');
const labels = {
 en:{kind:'How was this file created?',software:'Producing software (if known)',version:'Software version/build (if known)',evidence:'Source/reference note (optional)',save:'Save origin declaration',warning:'This is your declaration.'},
 'pt-BR':{kind:'Como este arquivo foi criado?',software:'Software que criou o arquivo (se conhecido)',version:'Versão/build do software (se conhecida)',evidence:'Nota de fonte/referência (opcional)',save:'Salvar declaração de origem',warning:'Esta é sua declaração.'},
 es:{kind:'¿Cómo se creó este archivo?',software:'Software que creó el archivo (si se conoce)',version:'Versión/build del software (si se conoce)',evidence:'Nota de fuente/referencia (opcional)',save:'Guardar declaración de origen',warning:'Esta es su declaración.'},
};
const browser = await launchBrowser();
try {
 for(const [lang,l] of Object.entries(labels)) {
  for(const theme of ['light','dark']) {
   const page = await browser.newPage({viewport:{width:theme==='light'?1366:1024,height:theme==='light'?768:600}});
   await page.goto(await launchURL(dir));await page.waitForLoadState('networkidle');
   const state = await api(page,'GET','/api/state');
   await api(page,'PUT','/api/profile/settings',{language:lang,theme,onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
   const files = await api(page,'GET','/api/files');
   const file = files.find(f=>f.items?.some(m=>m.params?.effective_diameter));
   if(!file) throw Error('seeded scientific file missing');
   const scoped = async(body)=>page.evaluate(async({id,body,account,profile})=>{
    const r = await fetch(`/api/files/${id}/producer-origin`,{method:'PUT',headers:{'Content-Type':'application/json','X-MNE-Lab':'1','X-Account-ID':account,'X-Profile-ID':profile},body:JSON.stringify(body)});
    if(!r.ok) throw Error(`declaration HTTP ${r.status}`);return r.json();
   },{id:file.id,body,account:state.account.id,profile:state.profile.id});
   await scoped({kind:'unknown'});
   await page.goto(new URL('/ls/files?file='+file.id,page.url()).href);await page.waitForLoadState('networkidle');
   const editor = page.locator('[data-producer-origin]');
   const kind = editor.getByRole('combobox',{name:l.kind,exact:true});
   await kind.waitFor();
   report.check(`${lang}/${theme}: parser never inferred producing vendor`,await kind.inputValue()==='unknown' && await editor.getByRole('textbox').count()===0);
   report.check(`${lang}/${theme}: declaration is visibly unverified`,(await editor.innerText()).includes(l.warning));
   await kind.selectOption('user_created_table');
   await editor.getByRole('textbox',{name:l.software,exact:true}).fill('Invented table editor');
   await editor.getByRole('textbox',{name:l.version,exact:true}).fill('synthetic-v1');
   await editor.getByRole('textbox',{name:l.evidence,exact:true}).fill('Invented reference; no authenticity claim');
   const response = page.waitForResponse(r=>r.request().method()==='PUT' && new URL(r.url()).pathname.endsWith('/producer-origin'));
   await editor.getByRole('button',{name:l.save,exact:true}).click();
   if(!(await response).ok())throw Error('UI declaration rejected');
   const full = await api(page,'GET','/api/files/'+file.id);
   report.check(`${lang}/${theme}: saved declaration retains kind and software version`,full.file.producerOrigin.kind==='user_created_table' && full.file.producerOrigin.declaration==='user_declared' && full.file.producerOrigin.softwareVersion==='synthetic-v1' && full.file.producerOrigin.declaredAt);
   report.check(`${lang}/${theme}: declaration preserves original checksum/parser/transport`,full.file.sha256===file.sha256 && full.file.parser===file.parser && full.file.origin===file.origin && full.file.status===file.status);
   await page.reload();await page.waitForLoadState('networkidle');
   await editor.getByRole('textbox',{name:l.software,exact:true}).waitFor();
   report.check(`${lang}/${theme}: reloaded editor preserves declaration`,await editor.getByRole('textbox',{name:l.software,exact:true}).inputValue()==='Invented table editor');
   const problems = await layoutProblems(page,translationKeys());
   report.check(`${lang}/${theme}: localized origin editor fits viewport`,problems.length===0,problems.join('; '));
   await page.screenshot({path:path.join(out,`${lang}-${theme}.png`)});
   await kind.selectOption('unknown');
   const clearing = page.waitForResponse(r=>r.request().method()==='PUT' && new URL(r.url()).pathname.endsWith('/producer-origin'));
   await editor.getByRole('button',{name:l.save,exact:true}).click();
   if(!(await clearing).ok())throw Error('origin reset rejected');
   const reset = (await api(page,'GET','/api/files/'+file.id)).file.producerOrigin;
   report.check(`${lang}/${theme}: unknown clears producer details`,reset.kind==='unknown' && !reset.software && !reset.softwareVersion && !reset.evidence);
   await page.close();
  }
 }
}catch(e){report.check('origin workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),e.stack || String(e));}
finally{await browser.close();}
process.exit(report.finish(out));
