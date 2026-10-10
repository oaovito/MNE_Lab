// Explicit DLS assignments are reviewed declarations, never instrument validation.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir,outRoot]=process.argv.slice(2);
const out=path.join(outRoot,'workbook-mapping');fs.mkdirSync(out,{recursive:true});
const report=new Report('manual-workbook-mapping');
const labels={
 en:{pick:'Import files',confirm:'Import reviewed files',inspect:'Inspect selected data',done:'Done',cancel:'Cancel',enable:'Configure DLS mapping',title:'Map DLS columns manually',column:'Column number',unit:'User-assigned unit',sample:'Sample column (optional)',name:'Name for this selection',save:'Save reviewed selection',saved:'Saved selections in this profile'},
 'pt-BR':{pick:'Importar arquivos',confirm:'Importar arquivos revisados',inspect:'Inspecionar dados selecionados',done:'Concluído',cancel:'Cancelar',enable:'Configurar mapeamento DLS',title:'Mapear colunas DLS manualmente',column:'Número da coluna',unit:'Unidade atribuída pelo usuário',sample:'Coluna da amostra (opcional)',name:'Nome para esta seleção',save:'Salvar seleção revisada',saved:'Seleções salvas neste perfil'},
 es:{pick:'Importar archivos',confirm:'Importar archivos revisados',inspect:'Inspeccionar datos seleccionados',done:'Listo',cancel:'Cancelar',enable:'Configurar asignación DLS',title:'Asignar columnas DLS manualmente',column:'Número de columna',unit:'Unidad asignada por el usuario',sample:'Columna de muestra (opcional)',name:'Nombre para esta selección',save:'Guardar selección revisada',saved:'Selecciones guardadas en este perfil'},
};
const browser=await launchBrowser();
try {
 for(const [index,[lang,l]] of Object.entries(labels).entries()) {
  const page=await browser.newPage({viewport:{width:1366,height:768}});
  await page.goto(await launchURL(dir));await page.waitForLoadState('networkidle');
  const state=await api(page,'GET','/api/state');
  await api(page,'PUT','/api/profile/settings',{language:lang,theme:'light',onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
  await page.reload();await page.waitForLoadState('networkidle');
  await page.evaluate(()=>(history.pushState(null,'','/ls/files'),dispatchEvent(new PopStateEvent('popstate'))));
  const before=await api(page,'GET','/api/files');
  const data=fs.readFileSync(new URL('../../../testdata/workbook-mapping/invented.xlsx',import.meta.url));
  const filename=`workbook-mapped-${lang}.xlsx`;
  const lastRow=26+index;
  const choose=async(name)=>{const [chooser]=await Promise.all([page.waitForEvent('filechooser'),page.getByRole('button',{name:l.pick,exact:true}).first().click()]);await chooser.setFiles({name,mimeType:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',buffer:data});};
  await choose(filename);await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  report.check(`${lang}: unknown table cannot be confirmed automatically`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0);
  await page.locator('summary').filter({hasText:l.title}).click();await page.getByRole('button',{name:l.enable,exact:true}).click();
  // Accessible names include translated parameter names, then the common suffix.
  const columnInput=page.getByRole('spinbutton',{name:new RegExp(l.column+'$')});
  const unitInput=page.getByRole('textbox',{name:new RegExp(l.unit+'$')});
  await columnInput.nth(0).fill('2');await unitInput.nth(0).fill('nm');
  const spin=page.getByRole('spinbutton');
  await spin.nth(0).fill('2');await spin.nth(1).fill('3');await spin.nth(2).fill(String(lastRow));
  await page.getByRole('spinbutton',{name:l.sample,exact:true}).fill('1');
  report.check(`${lang}: assigning fields stays read-only until inspection`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0&&JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.getByRole('button',{name:l.inspect,exact:true}).click();await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  report.check(`${lang}: reviewed manual data explicitly remains unvalidated`,(await page.locator('.overlay').innerText()).includes('UNVALIDATED')&&JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  await unitInput.nth(0).fill('um');
  report.check(`${lang}: editing unit invalidates confirmation`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0);
  await unitInput.nth(0).fill('nm');await page.getByRole('button',{name:l.inspect,exact:true}).click();await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  await spin.nth(2).fill('100000');await page.getByRole('button',{name:l.inspect,exact:true}).click();await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  report.check(`${lang}: failed extent cannot confirm and retains sheet choices`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0&&await page.locator('.overlay select').filter({has:page.locator('option[value="Raw Data"]')}).locator('option').count()===2);
  await spin.nth(2).fill(String(lastRow));await page.getByRole('button',{name:l.inspect,exact:true}).click();await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  report.check(`${lang}: correcting extent requires new successful review`,JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  const name=`Invented workbook mapping ${lang}`;
  await page.getByRole('textbox',{name:l.name,exact:true}).fill(name);await page.getByRole('button',{name:l.save,exact:true}).click();
  await page.getByRole('combobox',{name:l.saved,exact:true}).locator('option').filter({hasText:name}).waitFor({state:'attached'});
  const headers={'X-Account-ID':state.account.id,'X-Profile-ID':state.profile.id};
  const recipes=await page.request.get(new URL('/api/import/profiles',page.url()).href,{headers});const saved=(await recipes.json()).find(p=>p.name===name);
  report.check(`${lang}: saved mapping contains recipe without source/receipt`,saved?.schema===3&&saved?.selection?.mapping?.schema===2&&saved?.selection?.mapping?.sheet==='Raw Data'&&saved?.selection?.mapping?.lastRow===lastRow&&saved?.selection?.mapping?.columns?.[0]?.unit==='nm'&&!saved.receipt&&!saved.sha256&&JSON.stringify(before)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  report.check(`${lang}: saved recipe stays selected after normalization`,await page.getByRole('combobox',{name:l.saved,exact:true}).inputValue()===saved.id);
  const problems=await layoutProblems(page,translationKeys());report.check(`${lang}: mapping controls localized layout`,problems.length===0,problems.join('; '));
  await page.screenshot({path:path.join(out,`workbook-mapping-${lang}-light.png`)});
  await page.getByRole('button',{name:l.cancel,exact:true}).click();
  // Fresh file inspection is required even when reusing an authenticated saved recipe.
  await choose(filename);await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  await page.getByRole('combobox',{name:l.saved,exact:true}).selectOption(saved.id);
  report.check(`${lang}: applying saved mapping requires fresh inspection`,await page.getByRole('button',{name:l.confirm,exact:true}).count()===0);
  await page.getByRole('button',{name:l.inspect,exact:true}).click();await page.getByRole('button',{name:l.confirm,exact:true}).waitFor();
  await page.getByRole('button',{name:l.confirm,exact:true}).click();await page.getByRole('button',{name:l.done,exact:true}).waitFor();
  const after=await api(page,'GET','/api/files');const file=after.find(f=>f.name===filename);const q=file?.items?.[0]?.params?.effective_diameter;
  report.check(`${lang}: confirmation preserves precision, header and assigned unit origin`,after.length===before.length+1&&file.status==='partial'&&file.sourceInfo.scientificValidation==='UNVALIDATED'&&q?.value===1.23&&q?.raw==='1.2300'&&q?.label==='unknown size (um)'&&q?.unit==='nm'&&q?.unitOrigin==='user_mapping'&&q?.sourceColumn===2&&q?.line===3&&file.items.length===lastRow-2&&file.items[0].sourceSheet==='Raw Data'&&file.items[0].sourceRange==='A3:B3');
  report.check(`${lang}: missing and zero stay distinct without inventing units or independence`,Object.keys(file.items[1].params).length===0&&file.items[2].params.effective_diameter.raw==='0.0000'&&!file.items[0].experimentalUnitId);
  const original=await page.request.get(new URL(`/api/files/${file.id}/original?download=1`,page.url()).href);report.check(`${lang}: original bytes unchanged`,Buffer.compare(await original.body(),data)===0);
  await page.getByRole('button',{name:l.done,exact:true}).click();
  await api(page,'PUT','/api/profile/settings',{language:lang,theme:'dark',onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});await page.setViewportSize({width:1024,height:600});await page.reload();await page.waitForLoadState('networkidle');
  await choose(`workbook-dark-${lang}.xlsx`);await page.getByRole('button',{name:l.done,exact:true}).waitFor();await page.getByRole('combobox',{name:l.saved,exact:true}).selectOption(saved.id);
  const darkProblems=await layoutProblems(page,translationKeys());report.check(`${lang}: mapping dark small-window layout`,darkProblems.length===0,darkProblems.join('; '));await page.screenshot({path:path.join(out,`workbook-mapping-${lang}-dark.png`)});
  await page.getByRole('button',{name:l.cancel,exact:true}).click();report.check(`${lang}: dark preview cancellation keeps library unchanged`,JSON.stringify(after)===JSON.stringify(await api(page,'GET','/api/files')));
  await page.close();
 }
}catch(e){report.check('manual workbook mapping workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),e.stack||String(e));}
finally{await browser.close();}
process.exit(report.finish(out));
