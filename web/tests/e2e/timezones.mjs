// Explicitly synthetic wall clocks and instants, under two browser time zones.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, Report } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'timezones');
fs.mkdirSync(out, { recursive: true });
const report = new Report('timestamp-timezones');
const browser = await launchBrowser();
const labels = {
  en: { measurements: /^Measurements/, create: 'Create cycle', preview: 'Check associations' },
  'pt-BR': { measurements: /^Medições/, create: 'Criar ciclo', preview: 'Conferir associações' },
  es: { measurements: /^Mediciones/, create: 'Crear ciclo', preview: 'Revisar asociaciones' },
};
try {
  for (const lang of Object.keys(labels)) for (const timezoneId of ['UTC', 'America/Sao_Paulo']) {
    const ctx = await browser.newContext({ locale: lang, timezoneId, viewport: { width: 1366, height: 768 } });
    const page = await ctx.newPage();
    page.setDefaultTimeout(10000);
    try {
      await page.goto(await launchURL(dir));
      const state = await api(page, 'GET', '/api/state');
      await api(page, 'PUT', '/api/profile/settings', { language: lang, onboarding: { tour: true, ls: true, cycle: true, cycles: true, 'tip.portable': true }, whatsNewSeen: state.version });
      await page.reload();
      await page.waitForLoadState('networkidle');
      for (const known of [false, true]) {
        const tag = `${lang}-${timezoneId.replaceAll('/', '-')}-${known ? 'instant' : 'wall'}`;
        const originalDate = known ? '2026-02-16T09:00:00-03:00' : '2026-02-16 09:00:00';
        const raw = `Sample ID: SYNTHETIC-TIME-${tag}\nDate: ${originalDate}\nEffective Diameter (nm): 83.125\nDiameter (nm)\tIntensity (%)\n10\t20\n20\t50\n30\t30\n`;
        const response = await page.request.post(new URL('/api/files', page.url()).href, { data: raw, headers: { 'X-MNE-Lab': '1', 'X-File-Name': encodeURIComponent(tag+'.txt'), 'Content-Type': 'text/plain' } });
        if (!response.ok()) throw new Error('synthetic timestamp import failed');
        const imported = await response.json();
        const full = await api(page, 'GET', '/api/files/'+imported.fileId);
        const m = full.measurements[0];
        report.check(tag+' source has explicit zone semantics and original text', m.measuredAt.tzKnown === known && m.measuredAt.raw === originalDate);
        await page.evaluate((route) => (history.pushState(null, '', route), dispatchEvent(new PopStateEvent('popstate'))), '/ls/files?file='+imported.fileId);
        await page.getByRole('tab', { name: labels[lang].measurements }).click();
        const expectedClock = await page.evaluate(([ts, lang]) => new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'short', ...(!ts.tzKnown && { timeZone: 'UTC' }) }).format(new Date(ts.time)), [m.measuredAt, lang]);
        await page.getByText(expectedClock, { exact: true }).waitFor();
        report.check(tag+' measurement displays correct clock in browser zone', true);
        await page.getByRole('button', { name: labels[lang].create, exact: true }).click();
        const expectedInput = known && timezoneId === 'UTC' ? '2026-02-16T12:00' : '2026-02-16T09:00';
        const input = page.locator('input[type="datetime-local"]');
        await input.waitFor();
        await page.waitForFunction((want) => document.querySelector('input[type="datetime-local"]')?.value === want, expectedInput);
        report.check(tag+' cycle start retains wall clock or actual local instant', await input.inputValue() === expectedInput);
        const [previewResponse] = await Promise.all([page.waitForResponse((r) => new URL(r.url()).pathname === '/api/cycles/preview' && r.request().method() === 'POST'), page.getByRole('dialog').getByRole('button', { name: labels[lang].preview, exact: true }).click()]);
        const preview = await previewResponse.json();
        report.check(tag+' serialization and association preserve semantics', preview.config.startTzKnown === known && Date.parse(preview.config.start) === Date.parse(m.measuredAt.time) && preview.assignments[0].status === 'auto');
        const [savedResponse] = await Promise.all([page.waitForResponse((r) => new URL(r.url()).pathname === '/api/cycles' && r.request().method() === 'POST'), page.getByRole('dialog').getByRole('button', { name: labels[lang].create, exact: true }).click()]);
        const saved = await savedResponse.json();
        report.check(tag+' saved cycle retains original time and knowledge', saved.config.startTzKnown === known && Date.parse(saved.config.start) === Date.parse(m.measuredAt.time));
        const rendered = await api(page, 'POST', '/api/graphs/render', { definition: { kind: 'dls_distribution', title: 'Synthetic time provenance', measurements: [m.id], visual: { metadata: true } }, width: 800, height: 500 });
        const ts = rendered.provenance.sources[0].measuredTimestamp;
        report.check(tag+' graph carries raw text and explicit zone knowledge', ts.raw === originalDate && ts.tzKnown === known && ts.source === 'file');
        const repeated = { config: saved.config, measurements: [m.id, m.id, m.id] };
        const repeatedPreview = await api(page, 'POST', '/api/cycles/preview', repeated);
        const repeatedSaved = await api(page, 'POST', '/api/cycles', repeated);
        report.check(tag+' repeated IDs do not create extra observations in preview or save', repeatedPreview.assignments.length === 1 && repeatedPreview.items.length === 1 && repeatedPreview.series.effective_diameter[0].summary.n === 1 && repeatedSaved.measurements.length === 1);
        const source = await api(page, 'GET', '/api/files/'+imported.fileId);
        report.check(tag+' cycle creation did not rewrite source', JSON.stringify(source.measurements[0].measuredAt) === JSON.stringify(m.measuredAt));
        await page.screenshot({ path: path.join(out, tag+'.png') });
      }
    } catch (e) {
      report.check(lang+' '+timezoneId+' workflow completes', false, e.message.split('\n')[0]);
      fs.writeFileSync(path.join(out, lang+'-'+timezoneId.replaceAll('/', '-')+'-failure.txt'), e.stack || String(e));
    } finally { await ctx.close(); }
  }
} finally { await browser.close(); }
process.exit(report.finish(out));
