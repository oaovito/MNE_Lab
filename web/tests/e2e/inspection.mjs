// Preview and cancellation must not write the scientific library.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'inspection');
fs.mkdirSync(out, { recursive: true });
const report = new Report('import-inspection');
const labels = {
  en: { pick: 'Import files', confirm: 'Import reviewed files', cancel: 'Cancel', done: 'Done', detected: 'Detected data', literal: 'Literal source table', readonly: 'Read-only preview', declared: 'Declared value' },
  'pt-BR': { pick: 'Importar arquivos', confirm: 'Importar arquivos revisados', cancel: 'Cancelar', done: 'Concluído', detected: 'Dados detectados', literal: 'Tabela literal da fonte', readonly: 'Prévia somente leitura', declared: 'Valor declarado' },
  es: { pick: 'Importar archivos', confirm: 'Importar archivos revisados', cancel: 'Cancelar', done: 'Listo', detected: 'Datos detectados', literal: 'Tabla literal de origen', readonly: 'Vista previa de solo lectura', declared: 'Valor declarado' },
};
const browser = await launchBrowser();
try {
 for (const [lang, l] of Object.entries(labels)) {
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  await page.goto(await launchURL(dir));
  await page.waitForLoadState('networkidle');
  const state = await api(page, 'GET', '/api/state');
  await api(page, 'PUT', '/api/profile/settings', { language: lang, theme: 'light', onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: state.version });
  await page.reload(); await page.waitForLoadState('networkidle');
  await page.evaluate(() => (history.pushState(null, '', '/ls/files'), dispatchEvent(new PopStateEvent('popstate'))));
  const choose = async (name, buffer) => {
   const [chooser] = await Promise.all([page.waitForEvent('filechooser'), page.getByRole('button', { name: l.pick, exact: true }).first().click()]);
   await chooser.setFiles({ name, mimeType: 'text/plain', buffer });
  };
  const before = await api(page, 'GET', '/api/files');
  const data = Buffer.from(`Sample ID: Synthetic ${lang}\nEffective Diameter (nm): 123.450\nAverage Count Rate (kcps): 73.250\n`);
  let confirmations = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === '/api/import/confirm') confirmations++; });
  await choose(`preview-${lang}.txt`, data);
  await page.getByRole('button', { name: l.confirm, exact: true }).waitFor();
  const previewFiles = await api(page, 'GET', '/api/files');
  report.check(`${lang}: inspection does not save or confirm`, JSON.stringify(before) === JSON.stringify(previewFiles) && confirmations === 0);
  await page.locator('summary').filter({ hasText: l.detected }).click();
  report.check(`${lang}: preview preserves original precision and units`, (await page.locator('.overlay').innerText()).includes('123.450') && (await page.locator('.overlay').innerText()).includes('kcps'));
  const problems = await layoutProblems(page, translationKeys());
  report.check(`${lang}: preview has no untranslated keys or broken layout`, problems.length === 0, problems.join('; '));
  await page.screenshot({ path: path.join(out, `preview-${lang}.png`) });
  await page.getByRole('button', { name: l.cancel, exact: true }).click();
  report.check(`${lang}: cancel leaves library unchanged`, JSON.stringify(before) === JSON.stringify(await api(page, 'GET', '/api/files')) && confirmations === 0);
  await choose(`literal-unknown-${lang}.csv`, Buffer.from('unknown,value\nInvented,1.2300\n'));
  await page.getByRole('button', { name: l.done, exact: true }).waitFor();
  await page.locator('summary').filter({ hasText: l.literal }).click();
  report.check(`${lang}: literal unknown table is visible without units or scientific import`, (await page.locator('.overlay').innerText()).includes('1.2300') && await page.getByRole('button', { name: l.confirm, exact: true }).count() === 0 && JSON.stringify(before) === JSON.stringify(await api(page, 'GET', '/api/files')));
  const literalProblems = await layoutProblems(page, translationKeys());
  report.check(`${lang}: literal table localized layout`, literalProblems.length === 0, literalProblems.join('; '));
  await page.screenshot({ path: path.join(out, `literal-table-${lang}.png`) });
  await page.getByRole('button', { name: l.done, exact: true }).click();
  for (const [theme, viewport] of [['light', { width: 1366, height: 768 }], ['dark', { width: 1024, height: 600 }]]) {
   await api(page, 'PUT', '/api/profile/settings', { language: lang, theme, onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: state.version });
   await page.setViewportSize(viewport); await page.reload(); await page.waitForLoadState('networkidle');
   await choose(`invented-${lang}.ods`, fs.readFileSync('../testdata/ods-preview/invented.ods'));
   await page.getByRole('button', { name: l.done, exact: true }).waitFor();
   await page.locator('summary').filter({ hasText: l.literal }).click();
   const text = await page.locator('.overlay').innerText();
   report.check(`${lang}/${theme}: ODS display and declared precision are separate`, text.includes('1,2300') && text.includes('1.2300') && text.includes(l.declared) && text.includes('0.50') && text.includes('50%'));
   report.check(`${lang}/${theme}: ODS is read-only and cannot confirm scientific data`, text.includes(l.readonly) && await page.getByRole('button', { name: l.confirm, exact: true }).count() === 0 && confirmations === 0 && JSON.stringify(before) === JSON.stringify(await api(page, 'GET', '/api/files')));
   const odsProblems = await layoutProblems(page, translationKeys());
   report.check(`${lang}/${theme}: ODS localized bounded layout`, odsProblems.length === 0, odsProblems.join('; '));
   await page.screenshot({ path: path.join(out, `ods-${lang}-${theme}.png`) });
   await page.getByRole('button', { name: l.done, exact: true }).click();
  }
  await api(page, 'PUT', '/api/profile/settings', { language: lang, theme: 'light', onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: state.version });
  await page.setViewportSize({ width: 1366, height: 768 }); await page.reload(); await page.waitForLoadState('networkidle');
  await choose(`invalid-${lang}.txt`, Buffer.from('Unrecognized synthetic report'));
  await page.getByRole('button', { name: l.done, exact: true }).waitFor();
  report.check(`${lang}: invalid preview cannot be confirmed`, await page.getByRole('button', { name: l.confirm, exact: true }).count() === 0 && JSON.stringify(before) === JSON.stringify(await api(page, 'GET', '/api/files')));
  await page.getByRole('button', { name: l.done, exact: true }).click();
  await choose(`confirmed-${lang}.txt`, data);
  await page.getByRole('button', { name: l.confirm, exact: true }).click();
  await page.getByRole('button', { name: l.done, exact: true }).waitFor();
  const after = await api(page, 'GET', '/api/files');
  const file = after.find(f => f.name === `confirmed-${lang}.txt`);
  report.check(`${lang}: explicit confirmation saves full reviewed data`, after.length === before.length + 1 && confirmations === 1 && file?.items?.[0]?.params?.effective_diameter?.raw === '123.450');
  const original = await page.request.get(new URL(`/api/files/${file.id}/original?download=1`, page.url()).href);
  report.check(`${lang}: confirmed original bytes unchanged`, Buffer.compare(await original.body(), data) === 0);
  await page.close();
 }
} catch(e) { report.check('inspection workflow completes', false, e.message.split('\n')[0]); fs.writeFileSync(path.join(out,'failure.txt'),e.stack || String(e)); }
finally { await browser.close(); }
process.exit(report.finish(out));
