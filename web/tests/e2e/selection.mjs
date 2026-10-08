// Explicit worksheet/range review, original bytes and reproducible provenance.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir,outRoot]=process.argv.slice(2);
const out=path.join(outRoot,'selection');fs.mkdirSync(out,{recursive:true});
const report=new Report('workbook-selection');
const labels={
 en:{pick:'Import files',confirm:'Import reviewed files',inspect:'Inspect selected data',done:'Done',range:'Range in Synthetic sheet',value:'A2:B2'},
 'pt-BR':{pick:'Importar arquivos',confirm:'Importar arquivos revisados',inspect:'Inspecionar dados selecionados',done:'Concluído',range:'Intervalo em Synthetic sheet',value:'A1:B2'},
 es:{pick:'Importar archivos',confirm:'Importar archivos revisados',inspect:'Inspeccionar datos seleccionados',done:'Listo',range:'Rango en Synthetic sheet',value:'A2:B3'},
};
const browser=await launchBrowser();
try {
 for(const [lang,l] of Object.entries(labels)){
  const page=await browser.newPage({viewport:{width:1366,height:768}});
  await page.goto(await launchURL(dir));await page.waitForLoadState('networkidle');
  const state=await api(page,'GET','/api/state');
  await api(page,'PUT','/api/profile/settings',{language:lang,theme:'light',onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
  await page.reload();await page.waitForLoadState('networkidle');
  await page.evaluate(()=>(history.pushState(null,'','/ls/files'),dispatchEvent(new PopStateEvent('popstate'))));
  const before=await api(page,'GET','/api/files');
  const [chooser]=await Promise.all([page.waitForEvent('filechooser'),page.getByRole('button',{name:l.pick,exact:true}).first().click()]);
  const original=fs.readFileSync(path.join(dir,'synthetic-selection.xlsx'));
  const name=`selection-${lang}.xlsx`;
  await chooser.setFiles({name,mimeType:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',buffer:original});
  await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  report.check(`${lang}: inventory includes compatible and empty sheets`,await page.getByRole('checkbox',{name:/^Synthetic sheet/}).count()===1&&await page.getByRole('checkbox',{name:/^Other sheet/}).count()===1&&await page.getByRole('checkbox',{name:/^Empty sheet/}).count()===1);
  await page.getByRole('checkbox',{name:/^Other sheet/}).uncheck();
  await page.getByRole('checkbox',{name:/^Empty sheet/}).uncheck();
  await page.getByRole('textbox',{name:l.range,exact:true}).fill(l.value);
  report.check(`${lang}: changed selection invalidates confirmation`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0&&JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.getByRole('button',{name:l.inspect,exact:true}).click();
  await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  const problems=await layoutProblems(page,translationKeys());
  report.check(`${lang}: selected preview has no layout/translation errors`,problems.length===0,problems.join('; '));
  await page.screenshot({path:path.join(out,`selection-${lang}.png`)});
  await page.getByRole('button',{name:l.confirm,exact:true}).click();
  await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  const files=await api(page,'GET','/api/files');const file=files.find(f=>f.name===name);
  const full=await api(page,'GET','/api/files/'+file.id);const m=full.measurements[0];
  report.check(`${lang}: confirmed subset preserves source rows and recipe`,file.items.length===1&&m.sourceSheet==='Synthetic sheet'&&m.sourceRange===l.value&&m.params.effective_diameter.line===2&&m.params.effective_diameter.raw==='123.456'&&file.importSelection.sheets.length===1&&file.importSelection.sheets[0].range===l.value);
  const download=await page.request.get(new URL(`/api/files/${file.id}/original?download=1`,page.url()).href);
  report.check(`${lang}: subset import retains entire workbook unchanged`,Buffer.compare(await download.body(),original)===0);
  await page.close();
 }
}catch(e){report.check('selection workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),e.stack||String(e));}
finally{await browser.close();}
process.exit(report.finish(out));
