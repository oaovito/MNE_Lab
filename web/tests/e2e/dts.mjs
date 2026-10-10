// Entirely invented compound container: no private vendor fixture is shipped.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';
const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'dts');
fs.mkdirSync(out, { recursive: true });
const report = new Report('dts-partial-interoperability');
const labels = {
  en: { pick: 'Import files', confirm: 'Import reviewed files', done: 'Done', detected: 'Detected data', unknown: 'UNKNOWN MALVERN DTS' },
  'pt-BR': { pick: 'Importar arquivos', confirm: 'Importar arquivos revisados', done: 'Concluído', detected: 'Dados detectados', unknown: 'MALVERN DTS DESCONHECIDO' },
  es: { pick: 'Importar archivos', confirm: 'Importar archivos revisados', done: 'Listo', detected: 'Datos detectados', unknown: 'MALVERN DTS DESCONOCIDO' },
};
const browser = await launchBrowser();
try {
  for (const [index, [lang, l]] of Object.entries(labels).entries()) {
    const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
    await page.goto(await launchURL(dir)); await page.waitForLoadState('networkidle');
    const state = await api(page, 'GET', '/api/state');
    await api(page, 'PUT', '/api/profile/settings', { language: lang, theme: 'light', onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: state.version });
    await page.reload(); await page.waitForLoadState('networkidle');
    await page.evaluate(() => (history.pushState(null, '', '/ls/files'), dispatchEvent(new PopStateEvent('popstate'))));
    const data = Buffer.from(fs.readFileSync(path.join(dir, 'synthetic-compound.dts')));
    data[5120 + 4000] = index + 1; // Invented unused record bytes vary each fixture.
    const before = await api(page, 'GET', '/api/files');
    const choose = async (name, buffer) => {
      const [chooser] = await Promise.all([page.waitForEvent('filechooser'), page.getByRole('button', { name: l.pick, exact: true }).first().click()]);
      await chooser.setFiles({ name, mimeType: 'application/octet-stream', buffer });
    };
    await choose(`synthetic-container-${lang}.dts`, data);
    await page.getByRole('button', { name: l.confirm, exact: true }).waitFor();
    report.check(`${lang}: review is read-only`, JSON.stringify(before) === JSON.stringify(await api(page, 'GET', '/api/files')));
    const summary = page.locator('summary').filter({ hasText: l.detected });
    report.check(`${lang}: content classification remains unknown`, (await summary.innerText()).includes(l.unknown) && !(await summary.innerText()).includes('LIGHTSCATTERING'));
    report.check(`${lang}: partial / unvalidated warning is visible`, (await page.locator('.overlay').innerText()).includes('PARTIAL / UNVALIDATED'));
    const problems = await layoutProblems(page, translationKeys());
    report.check(`${lang}: localized preview layout`, problems.length === 0, problems.join('; '));
    await page.screenshot({ path: path.join(out, `preview-${lang}.png`) });
    await page.getByRole('button', { name: l.confirm, exact: true }).click();
    await page.getByRole('button', { name: l.done, exact: true }).waitFor();
    await page.getByRole('button', { name: l.done, exact: true }).click();
    const after = await api(page, 'GET', '/api/files');
    const file = after.find(f => f.name === `synthetic-container-${lang}.dts`);
    report.check(`${lang}: zero scientific measurements`, file?.module === 'unknown_malvern_dts' && !file.items?.length && file.sourceInfo?.scientificValidation === 'UNVALIDATED');
    const original = await page.request.get(new URL(`/api/files/${file.id}/original?download=1`, page.url()).href);
    report.check(`${lang}: byte-exact original download`, original.ok() && Buffer.compare(await original.body(), data) === 0);
    const opts = await api(page, 'GET', `/api/export/options?kind=file&file=${file.id}`);
    report.check(`${lang}: export options reflect actual coverage`, opts.metadataOnly === true && opts.options.sections.every(s => s.formats.every(f => ['original', 'json'].includes(f))));
    await page.evaluate(id => (history.pushState(null, '', `/ls/files?file=${id}`), dispatchEvent(new PopStateEvent('popstate'))), file.id);
    await page.getByText('PARTIAL / UNVALIDATED', { exact: true }).waitFor();
    report.check(`${lang}: library retains coverage metadata`, (await page.locator('.panel').last().innerText()).includes('Microsoft Compound File'));
    await page.close();
  }
} catch (e) {
  report.check('partial DTS workflow completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), e.stack || String(e));
} finally { await browser.close(); }
process.exit(report.finish(out));
