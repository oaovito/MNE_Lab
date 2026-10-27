// Experimental design: 24 synthetic measurements (units A, B, C; F-SR and
// C-SR; days 0, 7, 14, 21) plus one repeated read, placed through the
// name-suggestion preview, corrected by hand, saved as revisions, reopened,
// and checked for layout in every language, both themes and both sizes.
// Every file and value here is invented; no private corpus.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'design');
fs.mkdirSync(out, { recursive: true });
const report = new Report('design');
const keys = translationKeys();
const browser = await launchBrowser();
const messages = [];
let page;
const days = [0, 7, 14, 21];
try {
  page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  page.on('pageerror', (e) => messages.push(e.message));
  page.on('console', (m) => m.type() === 'error' && !m.text().startsWith('Failed to load resource') && messages.push(m.text()));
  await page.goto(await launchURL(dir));
  await page.waitForLoadState('networkidle');
  const state = await api(page, 'GET', '/api/state');
  const settings = (language, theme) => api(page, 'PUT', '/api/profile/settings', { language, theme, onboarding: { tutorial: true, ls: true, turbo: true, exit: true, locked: true, mobile: true, cycles: true, 'tip.portable': true, 'tip.temporary': true, 'tip.cloud': true }, whatsNewSeen: state.version });
  await settings('en', 'light');
  await page.reload();
  await page.waitForLoadState('networkidle');
  const hint = page.getByRole('button', { name: 'Got it', exact: true });
  if (await hint.count()) await hint.click();
  const scope = { 'X-Account-ID': state.account.id, 'X-Profile-ID': state.profile.id, 'X-MNE-Lab': '1' };
  const request = async (method, p, data) => {
    const res = await page.request.fetch(new URL(p, page.url()).href, { method, headers: scope, data });
    if (!res.ok()) throw new Error(`${method} ${p}: ${res.status()} ${await res.text()}`);
    return res.json();
  };
  const importFile = async (name, text) => {
    const bytes = Buffer.from(text);
    const headers = { ...scope, 'X-File-Name': encodeURIComponent(name), 'Content-Type': 'application/octet-stream' };
    const review = await page.request.post(new URL('/api/import/inspect', page.url()).href, { headers, data: bytes }).then((r) => r.json());
    const ok = await page.request.post(new URL('/api/import/confirm', page.url()).href, { headers: { ...headers, 'X-Import-Receipt': review.receipt }, data: bytes });
    if (!ok.ok()) throw new Error('synthetic confirm failed: ' + name);
  };
  let n = 0;
  for (const unit of ['A', 'B', 'C'])
    for (const cond of ['F-SR', 'C-SR'])
      for (const day of days) {
        n++;
        await importFile(`${cond} ${unit} day ${day}.txt`, `Sample ID: SynthDesign ${cond} ${unit} d${day}\nMeasurement Date: 2026-09-${String(1 + day).padStart(2, '0')} 10:${String(n).padStart(2, '0')}:00\nEffective Diameter (nm): ${(150 + n * 1.7).toFixed(2)}\nPolydispersity: 0.${100 + n}\n`);
      }
  await importFile('F-SR A day 0 repeat.txt', 'Sample ID: SynthDesign F-SR A d0 repeat\nMeasurement Date: 2026-09-01 11:00:00\nEffective Diameter (nm): 152.10\nPolydispersity: 0.101\n');
  const before = JSON.stringify(await request('GET', '/api/files'));

  const go = async (route) => {
    await page.evaluate((r) => (history.pushState(null, '', r), dispatchEvent(new PopStateEvent('popstate'))), route);
    await page.waitForLoadState('networkidle');
    await sleep(400);
  };
  await go('/ls/designs');
  await page.getByRole('button', { name: 'New design', exact: true }).first().click();
  await page.getByRole('textbox', { name: 'Design name', exact: true }).fill('Synthetic F-SR / C-SR stability');
  for (const c of ['F-SR', 'C-SR']) {
    await page.getByRole('textbox', { name: 'New condition', exact: true }).fill(c);
    await page.getByRole('button', { name: 'Add condition', exact: true }).click();
  }
  await page.getByRole('combobox', { name: 'C-SR comes from', exact: true }).selectOption({ label: 'Derived from F-SR' });
  await page.getByRole('textbox', { name: 'How C-SR is obtained', exact: true }).fill('centrifugation');
  for (const d of days) {
    await page.getByRole('textbox', { name: 'New time point (for example Day 7)', exact: true }).fill(`Day ${d}`);
    await page.getByRole('textbox', { name: 'Offset', exact: true }).fill(String(d));
    await page.getByRole('button', { name: 'Add time point', exact: true }).click();
  }
  await page.getByRole('button', { name: 'Add measurements', exact: true }).click();
  await page.locator('.modal input').first().fill('SynthDesign');
  await sleep(300);
  await page.getByRole('checkbox', { name: 'Select all', exact: true }).check();
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await page.locator('.design-table tbody tr').nth(24).waitFor();
  report.check('25 measurements added', (await page.locator('.design-table tbody tr').count()) === 25);

  // The suggestion is a preview; nothing is stored until it is confirmed.
  await page.getByRole('button', { name: 'Suggest from names', exact: true }).click();
  await page.locator('.modal table').waitFor();
  await sleep(700);
  await page.screenshot({ path: path.join(out, 'proposals.png') });
  report.check('preview stores nothing', (await request('GET', '/api/designs')).length === 0);
  const proposalRows = await page.locator('.modal tbody tr').count();
  report.check('every measurement has a proposal row', proposalRows === 25, String(proposalRows));
  await page.locator('.modal .modal-foot button.primary').first().click();
  await page.locator('.design-matrix tbody tr').nth(2).waitFor();
  await sleep(600);
  report.check('three units created from the names', (await page.locator('.design-matrix tbody tr').count()) === 3);
  report.check('paired rows marked', (await page.locator('.design-matrix .design-paired').count()) === 3);

  // The repeated read had no unit in its name: the person sets it, first
  // (wrongly) as a second experimental replicate, which the review flags.
  const repeat = 'SynthDesign F-SR A d0 repeat';
  const unitA = await page.getByRole('combobox', { name: 'Experimental unit of SynthDesign F-SR A d7', exact: true }).inputValue();
  await page.getByRole('combobox', { name: `Experimental unit of ${repeat}`, exact: true }).selectOption(unitA);
  await sleep(600);
  report.check('two experimental replicates in one cell are flagged', (await page.locator('.design-cell.warn').count()) === 1);
  await page.getByRole('button', { name: 'Save revision', exact: true }).click();
  await page.waitForURL(/\/ls\/designs\/[^/n][^/]*$/);
  await page.waitForLoadState('networkidle');
  await sleep(500);
  await page.screenshot({ path: path.join(out, 'revision-1.png') });

  // Correction: the repeated read is a technical repeat, kept as revision 2.
  await page.getByRole('combobox', { name: `Replicate type of ${repeat}`, exact: true }).selectOption('technical');
  await page.getByRole('textbox', { name: 'Reason for this change (optional)', exact: true }).fill('Second read of the same cuvette');
  await sleep(500);
  report.check('technical repeat shown apart from N', (await page.locator('.design-cell.warn').count()) === 0 && (await page.locator('.design-cell', { hasText: '1+1' }).count()) === 1);
  await page.getByRole('button', { name: 'Save revision', exact: true }).click();
  await page.getByText('Revision 2', { exact: true }).first().waitFor();
  const id = new URL(page.url()).pathname.split('/').pop();
  const view = await request('GET', '/api/designs/' + id);
  const s = view.structure;
  report.check('revision 2 is the head and keeps revision 1', view.design.revision === 2 && view.head && view.history.length === 2);
  report.check('change history names the corrected field', view.design.changes.some((c) => c.field === 'replicate_kind' && c.from === 'experimental' && c.to === 'technical'));
  report.check('N counts preparations, not readings', s.experimentalUnits === 3 && s.measurements === 25 && s.technicalReadings === 1);
  report.check('pairing F-SR to C-SR per unit', view.design.pairs.length === 3 && s.detected === 'paired_longitudinal' && s.conditionWithin && s.timeWithin);
  report.check('measurements and files unchanged', JSON.stringify(await request('GET', '/api/files')) === before);
  const catalog = await request('GET', '/api/series?design=' + id);
  const ef = catalog.series.filter((x) => x.kind === 'scalar' && x.experimentalUnitId === unitA && x.condition === 'F-SR' && x.time?.label === 'Day 0');
  report.check('series catalog resolves the design', ef.length > 0 && ef.some((x) => x.replicateKind === 'technical') && ef.some((x) => x.replicateKind === 'experimental'), JSON.stringify(ef.map((x) => x.replicateKind)));

  // Close and reopen: the pairs and the history come back.
  await page.reload();
  await page.waitForLoadState('networkidle');
  await page.locator('.design-matrix').waitFor();
  await sleep(600);
  report.check('pairing survives reopening', (await page.locator('.design-review input[type=checkbox]:checked').count()) === 3 && (await page.locator('.design-matrix .design-paired').count()) === 3);
  report.check('history lists both revisions', (await page.locator('.design-rev').count()) === 2);

  // Search palette finds the feature and the saved design.
  await page.keyboard.press('Control+k');
  await page.locator('.palette input').fill('pairing');
  await sleep(300);
  let ids = await page.locator('.palette .item').evaluateAll((els) => els.map((e) => e.getAttribute('data-id')));
  report.check('palette finds the design feature', ids.includes('f.experimental_design'), ids.slice(0, 8).join(', '));
  await page.locator('.palette input').fill('stability');
  await sleep(300);
  ids = await page.locator('.palette .item').evaluateAll((els) => els.map((e) => e.getAttribute('data-id')));
  report.check('palette finds the saved design', ids.includes('design.' + id), ids.slice(0, 8).join(', '));
  await page.keyboard.press('Escape');

  for (const language of ['en', 'pt-BR', 'es'])
    for (const [theme, width, height] of [
      ['light', 1366, 768],
      ['dark', 1024, 600],
    ]) {
      await settings(language, theme);
      await page.setViewportSize({ width, height });
      for (const [name, route, ready] of [
        ['library', '/ls/designs', '.design-card'],
        ['editor', '/ls/designs/' + id, '.design-matrix'],
      ]) {
        await page.goto(new URL(route, page.url()).href);
        await page.waitForLoadState('networkidle');
        await page.locator(ready).first().waitFor();
        await sleep(500);
        const problems = await layoutProblems(page, keys);
        report.check(`${name} layout ${language}/${theme}/${width}x${height}`, problems.length === 0, problems.join('; '));
        await page.screenshot({ path: path.join(out, `${name}-${language}-${theme}-${width}x${height}.png`) });
      }
    }
  await settings('en', 'light');
  report.check('no script errors', messages.length === 0, messages.join('; '));
} catch (e) {
  report.check('design workflow completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), (e.stack || String(e)) + '\n' + messages.join('\n'));
  if (page) await page.screenshot({ path: path.join(out, 'failure.png') }).catch(() => {});
} finally {
  await browser.close();
}
process.exit(report.finish(out));
