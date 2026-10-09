// Save/reapply/delete worksheet recipes without implicit scientific imports.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir,outRoot]=process.argv.slice(2);
const out=path.join(outRoot,'profiles');fs.mkdirSync(out,{recursive:true});
const report=new Report('import-profiles');
const labels={
 en:{pick:'Import files',confirm:'Import reviewed files',inspect:'Inspect selected data',done:'Done',cancel:'Cancel',range:'Range in Synthetic sheet',value:'A2:B2',name:'Name for this selection',save:'Save reviewed selection',saved:'Saved selections in this profile',del:'Delete saved selection'},
 'pt-BR':{pick:'Importar arquivos',confirm:'Importar arquivos revisados',inspect:'Inspecionar dados selecionados',done:'Concluído',cancel:'Cancelar',range:'Intervalo em Synthetic sheet',value:'A1:B2',name:'Nome para esta seleção',save:'Salvar seleção revisada',saved:'Seleções salvas neste perfil',del:'Excluir seleção salva'},
 es:{pick:'Importar archivos',confirm:'Importar archivos revisados',inspect:'Inspeccionar datos seleccionados',done:'Listo',cancel:'Cancelar',range:'Rango en Synthetic sheet',value:'A2:B3',name:'Nombre para esta selección',save:'Guardar selección revisada',saved:'Selecciones guardadas en este perfil',del:'Eliminar selección guardada'},
};
const browser=await launchBrowser();
try{
 for(const [lang,l] of Object.entries(labels)){
  const page=await browser.newPage({viewport:{width:1366,height:768}});
  await page.goto(await launchURL(dir));await page.waitForLoadState('networkidle');
  const state=await api(page,'GET','/api/state');
  await api(page,'PUT','/api/profile/settings',{language:lang,theme:'light',onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
  await page.reload();await page.waitForLoadState('networkidle');
  await page.evaluate(()=>(history.pushState(null,'','/ls/files'),dispatchEvent(new PopStateEvent('popstate'))));
  const before=await api(page,'GET','/api/files');
  const original=fs.readFileSync(path.join(dir,'synthetic-preset.xlsx'));
  const choose=async(name)=>{
   const [chooser]=await Promise.all([page.waitForEvent('filechooser'),page.getByRole('button',{name:l.pick,exact:true}).first().click()]);
   await chooser.setFiles({name,mimeType:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',buffer:original});
   await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  };
  const recipes=async()=>{
   const response=await page.request.get(new URL('/api/import/profiles',page.url()).href,{headers:{'X-Account-ID':state.account.id,'X-Profile-ID':state.profile.id}});
   if(!response.ok())throw new Error('scoped recipe list failed');return response.json();
  };
  await choose(`recipe-preview-${lang}.xlsx`);
  for(const sheet of ['Other sheet','Empty sheet','Preset context'])await page.getByRole('checkbox',{name:new RegExp('^'+sheet)}).uncheck();
  await page.getByRole('textbox',{name:l.range,exact:true}).fill(l.value);
  await page.getByRole('button',{name:l.inspect,exact:true}).click();
  await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  const title=`Synthetic recipe ${lang}`;
  await page.getByRole('textbox',{name:l.name,exact:true}).fill(title);
  await page.getByRole('button',{name:l.save,exact:true}).click();
  await page.getByRole('combobox',{name:l.saved,exact:true}).locator('option').filter({hasText:title}).waitFor({state:'attached'});
  const saved=(await recipes()).find(r=>r.name===title);
  report.check(`${lang}: saving a reviewed recipe does not import data`,saved?.selection?.sheets?.[0]?.range===l.value&&JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.getByRole('button',{name:l.cancel,exact:true}).click();
  report.check(`${lang}: recipe persists after closing preview`,(await recipes()).some(r=>r.id===saved.id));
  const filename=`recipe-confirmed-${lang}.xlsx`;
  await choose(filename);
  await page.getByRole('combobox',{name:l.saved,exact:true}).selectOption(saved.id);
  report.check(`${lang}: applying a recipe requires another inspection`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0&&await page.getByRole('textbox',{name:l.range,exact:true}).inputValue()===l.value);
  await page.getByRole('button',{name:l.inspect,exact:true}).click();
  await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  const problems=await layoutProblems(page,translationKeys());
  report.check(`${lang}: recipe controls have no layout/translation errors`,problems.length===0,problems.join('; '));
  await page.screenshot({path:path.join(out,`recipe-${lang}.png`)});
  await page.getByRole('button',{name:l.confirm,exact:true}).click();
  await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  const after=await api(page,'GET','/api/files');const file=after.find(f=>f.name===filename);
  report.check(`${lang}: recipe confirmation retains exact selected source`,after.length===before.length+1&&file?.items?.[0]?.sourceRange===l.value&&file?.items?.[0]?.params?.effective_diameter?.raw==='123.456');
  await page.getByRole('button',{name:l.done,exact:true}).click();
  await choose(`recipe-delete-${lang}.xlsx`);
  await page.getByRole('combobox',{name:l.saved,exact:true}).selectOption(saved.id);
  await page.getByRole('button',{name:l.del,exact:true}).click();
  await page.getByRole('combobox',{name:l.saved,exact:true}).locator('option').filter({hasText:title}).waitFor({state:'detached'});
  report.check(`${lang}: deleting a recipe does not change imported data`,!(await recipes()).some(r=>r.id===saved.id)&&JSON.stringify(after)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.getByRole('button',{name:l.cancel,exact:true}).click();
  await page.close();
 }
}catch(e){report.check('saved recipe workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),e.stack||String(e));}
finally{await browser.close();}
process.exit(report.finish(out));
