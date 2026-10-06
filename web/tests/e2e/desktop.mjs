// Desktop browser test: opens every main screen of a seeded MNE Lab, saves a
// screenshot of each for visual review and fails on script errors, failed
// requests, raw translation keys, cut text or a page that scrolls.
//
//   node tests/e2e/desktop.mjs <harness dir> <out dir> <lang> <light|dark> <WxH>
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys, watch } from './lib.mjs';

const [dir, outRoot, lang = 'en', theme = 'light', size = '1366x768'] = process.argv.slice(2);
const [W, H] = size.split('x').map(Number);
const tag = `${lang}-${theme}-${size}`;
const out = path.join(outRoot, tag);
fs.mkdirSync(out, { recursive: true });
const report = new Report(`desktop-${tag}`);
const keys = translationKeys();
const errors = [];

const browser = await launchBrowser();
const ctx = await browser.newContext({ viewport: { width: W, height: H }, colorScheme: theme, locale: lang });
const page = await ctx.newPage();
watch(page, errors, 'desktop');
await page.goto(await launchURL(dir));
await page.waitForLoadState('networkidle');

const seenAll = { tour: true, ls: true, turbo: true, exit: true, locked: true, mobile: true, cycles: true, 'tip.portable': true, 'tip.temporary': true, 'tip.cloud': true };
const st = await api(page, 'GET', '/api/state');
report.check('a profile is open', !!st.profile);
const settings = (onboarding) => api(page, 'PUT', '/api/profile/settings', { language: lang, theme, onboarding, whatsNewSeen: st.version });
await settings(seenAll);
await page.reload();
await page.waitForLoadState('networkidle');
await sleep(500);
report.check('interface follows the chosen language', (await page.evaluate(() => document.documentElement.lang)) === lang);

const graphs = await api(page, 'GET', '/api/graphs');
const cycles = await api(page, 'GET', '/api/cycles');
const go = async (route) => {
  await page.evaluate((r) => (history.pushState(null, '', r), dispatchEvent(new PopStateEvent('popstate'))), route);
  await sleep(500);
};
const click = async (sel) => {
  await page.locator(sel).first().click({ timeout: 4000 });
  await sleep(300);
};
const label = (m) => m[lang] || m.en;
async function screen(name, fn, extra) {
  try {
    await fn();
    await page.waitForLoadState('networkidle');
    await sleep(600);
    await page.screenshot({ path: path.join(out, `${name}.png`) });
    for (const p of await layoutProblems(page, keys)) report.check(name, false, p);
    if (extra) await extra();
  } catch (e) {
    report.check(name, false, e.message.split('\n')[0]);
  }
  await page.keyboard.press('Escape').catch(() => {});
  await sleep(150);
}
const onboarding = async (patch) => {
  await settings({ ...seenAll, ...patch });
  await page.reload();
  await page.waitForLoadState('networkidle');
  await sleep(500);
};
const menu = async (i) => {
  await click('button.avatar-btn');
  await page.locator('button.menu-item').nth(i).click({ timeout: 3000 });
  await sleep(400);
};

await screen('home', () => go('/'), async () => report.check('home shows "made by oaovito"', (await page.locator('text=made by oaovito').count()) > 0));
await screen('files', () => go('/ls/files'));
await screen('file-detail', async () => (await go('/ls/files'), await click('table tbody tr >> nth=2')));
await screen('graphs', () => go('/ls/graphs'));
for (const kind of ['dls_distribution', 'parameter_time', 'dls_by_time']) {
  const g = graphs.find((x) => x.kind === kind);
  if (g) await screen('graph-' + kind, () => go('/ls/graphs/' + g.id), async () => report.check(`graph ${kind} is drawn`, (await page.locator('.figure svg, svg.figure').count()) > 0 || (await page.locator('svg polyline').count()) > 0));
}
await screen('cycles', () => go('/ls/cycles'));
if (cycles[0]) await screen('cycle', () => go('/ls/cycles/' + cycles[0].id));
const exportLabel = label({ en: 'Export', 'pt-BR': 'Exportar', es: 'Exportar' });
if (graphs[0]) await screen('export-graph', async () => (await go('/ls/graphs/' + graphs[0].id), await click(`button:has-text("${exportLabel}")`), await sleep(800)));
if (cycles[0]) await screen('export-cycle', async () => (await go('/ls/cycles/' + cycles[0].id), await click(`button:has-text("${exportLabel}")`), await sleep(800)));
const tabs = ['general', 'profile', 'account', 'storage', 'backups', 'updates', 'mobile', 'privacy', 'about'];
for (const [i, tab] of tabs.entries()) await screen('settings-' + tab, async () => (await go('/'), await menu(1), await page.locator('.modal nav button').nth(i).click({ timeout: 3000 })));
await screen('mystuff', async () => (await go('/'), await menu(0)));
await screen('help', () => page.keyboard.press('F1'));
await screen('whatsnew', async () => (await go('/'), await menu(3)));

// The command palette finds features by everyday words (spec examples).
const finds = { backup: ['f.backups', 'f.recovery'], mobile: ['f.mobile', 'f.viewer', 'f.controller'], dls: ['f.ls', 'p.graphs', 'p.cycles'] };
for (const [q, want] of Object.entries(finds)) {
  await screen('palette-' + q, async () => {
    await page.keyboard.press('Control+k');
    await page.locator('.palette input').fill(q);
    await sleep(300);
    const ids = await page.locator('.palette .item').evaluateAll((els) => els.map((e) => e.getAttribute('data-id')));
    for (const w of want) report.check(`search "${q}" finds ${w}`, ids.includes(w), ids.slice(0, 8).join(', '));
  });
}

const mobileLabel = label({ en: 'Mobile access', 'pt-BR': 'Acesso pelo celular', es: 'Acceso desde el móvil' });
await screen('mobile-qr', async () => (await go('/'), await click(`header button[aria-label="${mobileLabel}"]`), await click('.modal button.primary'), await sleep(1200)), async () => report.check('QR code is shown', (await page.locator('.modal svg').count()) > 0));
await api(page, 'POST', '/api/mobile/stop', {});
await screen('turbo-first', async () => (await onboarding({ turbo: false }), await click('header button[aria-label="Turbo"]')));
await screen('exit-confirm', async () => (await onboarding({ exit: false }), await go('/'), await click('.quick-row button.primary')));
await screen('tour', () => onboarding({ tour: false }));
await screen('ls-intro', async () => (await onboarding({ ls: false }), await go('/ls/files')));
await onboarding({});
if (graphs.length) {
  await screen('present', async () => (await api(page, 'POST', '/api/present', { action: 'start', graphs: graphs.map((g) => g.id) }), await sleep(1200)));
  await api(page, 'POST', '/api/present', { action: 'stop' });
}

for (const e of errors) report.check('no errors', false, e);
await browser.close();
process.exit(report.finish(out));
