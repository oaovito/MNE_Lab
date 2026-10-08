// Independent, explicitly synthetic current/average count rate review.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'count-rates');
fs.mkdirSync(out, { recursive: true });
const report = new Report('count-rate-review');
const browser = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  await page.goto(await launchURL(dir));
  await page.waitForLoadState('networkidle');
  const state = await api(page, 'GET', '/api/state');
  await api(page, 'PUT', '/api/profile/settings', { language: 'en', theme: 'light', onboarding: { tour: true, ls: true, turbo: true, cycles: true, 'tip.portable': true }, whatsNewSeen: state.version });
  await page.reload();
  await page.waitForLoadState('networkidle');
  const go = (route) => page.evaluate((route) => (history.pushState(null, '', route), dispatchEvent(new PopStateEvent('popstate'))), route);
  await go('/ls/files');
  const [chooser] = await Promise.all([page.waitForEvent('filechooser'), page.getByRole('button', { name: 'Import files', exact: true }).first().click()]);
  await chooser.setFiles(path.join(dir, 'synthetic-count-rates.txt'));
  await page.getByRole('button', { name: 'Import reviewed files', exact: true }).click();
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  const files = await api(page, 'GET', '/api/files');
  const file = files.find((f) => f.name === 'synthetic-count-rates.txt');
  const summary = file.items[0];
  report.check('library exposes independently labeled current and average rates', summary.params.count_rate.value === 73.25 && summary.params.average_count_rate.value === 0.1845);
  report.check('summary retains explicit units and source precision', summary.params.count_rate.raw === '73.250' && summary.params.count_rate.unit === 'kcps' && summary.params.average_count_rate.raw === '0.18450' && summary.params.average_count_rate.unit === 'Mcps');
  await go('/ls/files?file=' + file.id);
  await page.getByRole('tab', { name: /^Measurements/ }).click();
  await page.getByText('Average Count Rate', { exact: true }).waitFor();
  report.check('measurement review distinguishes both quantities', await page.getByText('Current Count Rate', { exact: true }).count() > 0 && await page.getByText('Average Count Rate', { exact: true }).count() > 0);
  const rendered = await api(page, 'POST', '/api/graphs/render', { definition: { title: 'Synthetic count rate metadata', kind: 'dls_distribution', measurements: [summary.id], visual: { metadata: true, legend: true } }, width: 800, height: 500 });
  report.check('graph metadata retains independent source values', rendered.series[0].meta.count_rate === '73.250' && rendered.series[0].meta.average_count_rate === '0.18450');
  const full = await api(page, 'GET', '/api/files/' + file.id);
  report.check('recognized fields retain their own original source lines', full.measurements[0].params.count_rate.line === 4 && full.measurements[0].params.average_count_rate.line === 5);
  const problems = await layoutProblems(page, translationKeys());
  report.check('review has no broken layout or untranslated keys', problems.length === 0, problems.join('; '));
  await page.screenshot({ path: path.join(out, 'count-rate-review.png') });
} catch (e) {
  report.check('browser workflow completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), e.stack || String(e));
} finally {
  await browser.close();
}
process.exit(report.finish(out));
