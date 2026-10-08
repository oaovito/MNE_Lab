// Synthetic XLSX import and failed Turbo restart through the desktop interface.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, Report, sleep } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'import');
fs.mkdirSync(out, { recursive: true });
const report = new Report('spreadsheet-import');
const browser = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
  await page.goto(await launchURL(dir));
  await page.waitForLoadState('networkidle');
  const before = await api(page, 'GET', '/api/state');
  await api(page, 'PUT', '/api/profile/settings', { language: 'en', theme: 'light', onboarding: { tour: true, ls: true, turbo: true }, whatsNewSeen: before.version });
  await page.reload();
  await page.waitForLoadState('networkidle');
  await page.evaluate(() => (history.pushState(null, '', '/ls/files'), dispatchEvent(new PopStateEvent('popstate'))));
  const [input] = await Promise.all([
    page.waitForEvent('filechooser'),
    page.getByRole('button', { name: 'Import files', exact: true }).first().click(),
  ]);
  report.check('file chooser offers XLSX', (await (await input.element()).getAttribute('accept')).includes('.xlsx'));
  await input.setFiles(path.join(dir, 'synthetic.xlsx'));
  await page.getByRole('button', { name: 'Import reviewed files', exact: true }).click();
  await page.getByRole('button', { name: 'Review', exact: true }).waitFor({ timeout: 10000 });
  const files = await api(page, 'GET', '/api/files');
  const file = files.find((f) => f.name === 'synthetic.xlsx');
  report.check('workbook imports with one measurement', file?.format === 'xlsx' && file?.items?.length === 1);
  report.check('measurement retains source sheet and value', file?.items?.[0]?.sourceSheet === 'Synthetic sheet' && file?.items?.[0]?.params?.effective_diameter?.value === 123.456);
  const original = await page.request.get(new URL('/api/files/' + file.id + '/original?download=1', page.url()).href);
  report.check('downloaded original has identical bytes', Buffer.compare(await original.body(), fs.readFileSync(path.join(dir, 'synthetic.xlsx'))) === 0);
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  await page.locator('header button[aria-label="Turbo"]').click();
  await page.getByText('The restart failed. Your previous Turbo setting and session have been kept.', { exact: true }).waitFor({ timeout: 10000 });
  await sleep(300);
  const after = await api(page, 'GET', '/api/state');
  report.check('failed Turbo preserves preference and profile', after.settings.turbo === before.settings.turbo && after.profile?.id === before.profile?.id);
  report.check('failed Turbo leaves navigation usable', (await page.locator('.overlay').count()) === 0);
  await page.screenshot({ path: path.join(out, 'spreadsheet-and-restart.png') });
} catch (e) {
  report.check('browser workflow completes', false, e.message.split('\n')[0]);
} finally {
  await browser.close();
}
process.exit(report.finish(out));
