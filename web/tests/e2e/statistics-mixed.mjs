// Actual offline product worker, physical unit assignment, save/graph/export.
// Every observation and expected value is synthetic; no private corpus.
import fs from 'node:fs';
import path from 'node:path';
import {api,launchBrowser,launchURL,layoutProblems,Report,translationKeys} from './lib.mjs';
const [dir,outRoot]=process.argv.slice(2),out=path.join(outRoot,'statistics-mixed');
fs.mkdirSync(out,{recursive:true});
const fixture=JSON.parse(fs.readFileSync(new URL('../fixtures/statistics-mixed-golden.json',import.meta.url))).cases[1];
const report=new Report('statistics-mixed'),browser=await launchBrowser(),messages=[];let page;
try {
 page=await browser.newPage({viewport:{width:1366,height:768}});
 page.on('pageerror',e=>messages.push(e.message));page.on('console',m=>{if(m.type()==='error')messages.push(m.text())});
 await page.goto(await launchURL(dir));await page.waitForLoadState('networkidle');
 const state=await api(page,'GET','/api/state');
 await api(page,'PUT','/api/profile/settings',{language:'en',theme:'light',onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});
 await page.reload();await page.waitForLoadState('networkidle');
 const hint=page.getByRole('button',{name:'Got it',exact:true});if(await hint.count())await hint.click();
 const scope={'X-Account-ID':state.account.id,'X-Profile-ID':state.profile.id,'X-MNE-Lab':'1'};
 const request=async(method,p,data)=>{const res=await page.request.fetch(new URL(p,page.url()).href,{method,headers:scope,data});if(!res.ok())throw new Error(`${method} ${p}: ${res.status()} ${await res.text()}`);return res.json()};
 const units={};for(const u of new Set(fixture.input.unitId))units[u]=(await request('POST','/api/experimental-units',{label:'Invented physical '+u})).id;
 for(let i=0;i<fixture.input.values.length;i++) {
  const bytes=Buffer.from(`Sample ID: Mixed observation ${i+1}\nEffective Diameter (nm): ${fixture.input.values[i].toPrecision(17)}\n`);
  const headers={...scope,'X-File-Name':encodeURIComponent(`synthetic-mixed-${i}.txt`),'Content-Type':'application/octet-stream'};
  const review=await page.request.post(new URL('/api/import/inspect',page.url()).href,{headers,data:bytes}).then(r=>r.json());
  const confirmed=await page.request.post(new URL('/api/import/confirm',page.url()).href,{headers:{...headers,'X-Import-Receipt':review.receipt},data:bytes});
  if(!confirmed.ok())throw new Error('synthetic confirm failed');
 }
 let external=0;
 await page.route('**/*',route=>{if(new URL(route.request().url()).origin===new URL(page.url()).origin)return route.continue();external++;return route.abort()});
 await page.evaluate(()=>{history.pushState(null,'','/ls/statistics/new');dispatchEvent(new PopStateEvent('popstate'))});
 await page.getByRole('textbox',{name:'Analysis name',exact:true}).fill('Synthetic incomplete mixed study');
 await page.getByRole('combobox',{name:'Experimental structure',exact:true}).selectOption('repeated');
 await page.getByRole('combobox',{name:'Method',exact:true}).selectOption('mixed');
 await page.getByRole('checkbox',{name:'I reviewed the experimental units and the independence or repetition of these observations.',exact:true}).check();
 await page.getByRole('button',{name:'Choose measurements',exact:true}).click();
 for(let i=0;i<fixture.input.values.length;i++)await page.getByRole('checkbox',{name:`Mixed observation ${i+1}`,exact:true}).check();
 await page.getByRole('button',{name:'Apply',exact:true}).click();
 for(let i=0;i<fixture.input.values.length;i++) {
  const label=`Mixed observation ${i+1}`;
  await page.getByRole('textbox',{name:'Factor A '+label,exact:true}).fill(fixture.input.factorA[i]);
  await page.getByRole('textbox',{name:'Factor B / time '+label,exact:true}).fill(fixture.input.factorB[i]);
  await page.getByRole('combobox',{name:'Experimental unit '+label,exact:true}).selectOption(units[fixture.input.unitId[i]]);
 }
 await page.getByRole('button',{name:'Review design and sources',exact:true}).click();
 const run=page.getByRole('button',{name:'Run analysis locally',exact:true});await run.waitFor();
 report.check('Mixed has no GG/HF or posthoc choices',await page.getByRole('combobox',{name:'Sphericity correction',exact:true}).count()===0 && await page.getByRole('combobox',{name:'Multiple comparisons',exact:true}).locator('option').count()===1);
 await run.click();await page.getByRole('button',{name:'Cancel',exact:true}).click();await run.waitFor({timeout:15000});
 report.check('cancel releases worker and leaves no saved result',await page.getByRole('button',{name:'Save analysis',exact:true}).count()===0);
 await run.click();await page.getByRole('button',{name:'Save analysis',exact:true}).waitFor({timeout:120000});
 await page.getByRole('heading',{name:'Marginal Wald F tests',exact:true}).scrollIntoViewIfNeeded();
 await page.screenshot({path:path.join(out,'calculated.png')});
 await page.getByRole('button',{name:'Save analysis',exact:true}).click();
 await page.getByRole('button',{name:'Create statistical graph',exact:true}).waitFor();
 const saved=(await request('GET','/api/statistics')).find(a=>a.snapshot.definition.title==='Synthetic incomplete mixed study');
 report.check('explicit physical units and incomplete design persist',saved.snapshot.design.n===fixture.input.values.length && !saved.snapshot.design.completeRepeated && saved.snapshot.design.experimentalUnits===12 && saved.snapshot.definition.sphericityCorrection===undefined && saved.snapshot.definition.structure==='repeated');
 for(const expected of fixture.expected.terms){const t=saved.results.terms.find(t=>t.source===expected.source);report.check('independent marginal test '+expected.source,t.df===expected.df && t.denominatorDF===expected.denominatorDF && Math.abs(t.f-expected.f)<1e-5*Math.max(1,Math.abs(expected.f)) && Math.abs(t.p-expected.p)<1e-6)}
 for(const k of ['randomVariance','residualVariance','logLikelihood'])report.check('independent model '+k,Math.abs(saved.results.model[k]-fixture.expected.model[k])<(k==='logLikelihood'?1e-6:1e-5*Math.max(1,Math.abs(fixture.expected.model[k]))));
 report.check('Mixed omits classical effects and comparisons',saved.results.comparisons.length===0 && saved.results.terms.every(t=>t.ss===undefined&&t.etaSquared===undefined) && saved.results.engine.endsWith('; mixed-random-intercept/1'));
 report.check('R and package assets stay local',external===0);
 report.check('model coefficient and residual plots are displayed',await page.getByRole('heading',{name:'Mixed model and sum-contrast coefficients',exact:true}).count()===1 && await page.getByRole('img',{name:'Residual QQ plot',exact:true}).count()===1);
 await page.getByRole('button',{name:'Create statistical graph',exact:true}).click();
 await page.getByText('Mean error bars',{exact:true}).waitFor();
 const graph=(await request('GET','/api/graphs')).find(g=>g.analysisId===saved.id);
 const figure=await request('POST','/api/graphs/render',{definition:graph,width:900,height:600});
 report.check('shared Graph Engine renders original Mixed observations',JSON.stringify(figure).includes('mixed-random-intercept/1') && graph.kind==='statistical_groups');
 for(const [language,theme,width,height]of [['en','light',1366,768],['en','dark',1024,600],['pt-BR','light',1366,768],['pt-BR','dark',1024,600],['es','light',1366,768],['es','dark',1024,600]]) {
  await api(page,'PUT','/api/profile/settings',{language,theme,onboarding:{tour:true,ls:true,turbo:true},whatsNewSeen:state.version});await page.setViewportSize({width,height});
  await page.goto(new URL('/ls/statistics/'+saved.id,page.url()).href);await page.waitForLoadState('networkidle');await page.locator('.statistics-page table').first().waitFor();
  const problems=await layoutProblems(page,translationKeys());report.check(`Mixed saved layout ${language}/${theme}/${width}x${height}`,problems.length===0,problems.join('; '));
  await page.screenshot({path:path.join(out,`saved-${language}-${theme}.png`)});
 }
 report.check('no script or CSP failures',messages.length===0,messages.join('; '));
} catch(e) {report.check('Mixed workflow completes',false,e.message.split('\n')[0]);fs.writeFileSync(path.join(out,'failure.txt'),(e.stack||String(e))+'\n'+messages.join('\n'));if(page)await page.screenshot({path:path.join(out,'failure.png')}).catch(()=>{});} finally {await browser.close()}
process.exit(report.finish(out));
