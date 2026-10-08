// Synthetic NanoBrook alternatives: import, inspect, confirm and preserve graphs.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'methods');
fs.mkdirSync(out, { recursive: true });
const report = new Report('distribution-methods');
const browser = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  await page.goto(await launchURL(dir));
  await page.waitForLoadState('networkidle');
  const state = await api(page, 'GET', '/api/state');
  await api(page, 'PUT', '/api/profile/settings', { language: 'en', theme: 'light', onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: state.version });
  await page.reload();
  await page.waitForLoadState('networkidle');
  const navigate = (route) => page.evaluate((route) => (history.pushState(null, '', route), dispatchEvent(new PopStateEvent('popstate'))), route);
  await navigate('/ls/files');
  const [chooser] = await Promise.all([page.waitForEvent('filechooser'), page.getByRole('button', { name: 'Import files', exact: true }).first().click()]);
  await chooser.setFiles(path.join(dir, 'synthetic-methods.txt'));
  await page.getByRole('button', { name: 'Import reviewed files', exact: true }).click();
  await page.getByRole('button', { name: 'Review', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  const files = await api(page, 'GET', '/api/files');
  const file = files.find((f) => f.name === 'synthetic-methods.txt');
  const full = await api(page, 'GET', '/api/files/' + file.id);
  const m = full.measurements[0];
  report.check('four methods/layouts preserved without default curve', m.distributions.length === 4 && !m.distribution);
  await navigate('/ls/files?file=' + file.id);
  await page.getByRole('tab', { name: /^Measurements/ }).click();
  const select = page.getByRole('combobox', { name: 'Choose a distribution', exact: true });
  await select.waitFor();
  report.check('review offers every original representation', await select.locator('option').count() === 5);
  const first = m.distributions.find((d) => d.method === 'lognormal' && d.format === 'spreadsheet');
  await select.selectOption(first.id);
  await page.getByText('0.0213', { exact: true }).waitFor();
  report.check('preview preserves original precision and unknown percentage unit', await page.getByText('0.0213', { exact: true }).count() > 0 && await page.locator('p').filter({ hasText: 'G(d): relative intensity; percentage units are not stated.' }).count() > 0);
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await sleep(300);
  const chosen = await api(page, 'GET', '/api/files/' + file.id);
  report.check('confirmation records candidate and time', chosen.measurements[0].distributionId === first.id && !!chosen.measurements[0].distributionSelectedAt);
  const def = { title: 'Synthetic pinned method', kind: 'dls_distribution', measurements: [m.id], weighting: 'intensity', xScale: 'auto', visual: { legend: true, metadata: true, grid: true, points: false, lineWidth: 1.75, fontScale: 1 } };
  const saved = await api(page, 'POST', '/api/graphs', def);
  const second = m.distributions.find((d) => d.method === 'multimodal' && d.format === 'spreadsheet');
  await select.selectOption(second.id);
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const rendered = await api(page, 'POST', '/api/graphs/render', { definition: saved, width: 800, height: 500 });
  report.check('saved graph keeps precise original method after library choice changes', rendered.provenance?.sources?.[0]?.distributionId === first.id && rendered.series?.[0]?.y?.[0] === 0.0213 && rendered.y?.unit === '');
  const problems = await layoutProblems(page, translationKeys());
  report.check('review has no broken layout or untranslated keys', problems.length === 0, problems.join('; '));
  await page.screenshot({ path: path.join(out, 'method-review.png') });
} catch (e) {
  report.check('browser workflow completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), e.stack || String(e));
} finally {
  await browser.close();
}
process.exit(report.finish(out));
