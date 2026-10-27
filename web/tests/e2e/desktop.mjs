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

const seenAll = { tutorial: true, ls: true, turbo: true, exit: true, locked: true, mobile: true, cycles: true, 'tip.portable': true, 'tip.temporary': true, 'tip.cloud': true };
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
const menu = async (name) => {
  await click('button.avatar-btn');
  // Match the label itself: an optional localized "New" badge also becomes
  // part of a menu item's accessible name.
  await page.getByRole('menuitem').filter({ has: page.getByText(name, { exact: true }) }).click({ timeout: 3000 });
  await sleep(400);
};

await screen('home', () => go('/'), async () => report.check('home shows "made by oaovito"', (await page.locator('text=made by oaovito').count()) > 0));
await screen('files', () => go('/ls/files'));
await screen('file-detail', async () => (await go('/ls/files'), await click('table tbody tr >> nth=2')));
// Measurements of a file: each value with its unit and source line, and the
// file's own parameters opened.
await screen('file-measurements', async () => {
  await go('/ls/files');
  await click('table tbody tr >> nth=2');
  await click('[role=tablist] >> nth=1 >> [role=tab] >> nth=1');
  await sleep(400);
  await click('.meas details summary');
});
await screen('graphs', () => go('/ls/graphs'));
for (const kind of ['dls_distribution', 'parameter_time', 'dls_by_time']) {
  const g = graphs.find((x) => x.kind === kind);
  if (g) await screen('graph-' + kind, () => go('/ls/graphs/' + g.id), async () => report.check(`graph ${kind} is drawn`, (await page.locator('.figure svg, svg.figure').count()) > 0 || (await page.locator('svg polyline').count()) > 0));
}
await screen('cycles', () => go('/ls/cycles'));
if (cycles[0]) await screen('cycle', () => go('/ls/cycles/' + cycles[0].id));
await screen('designs', () => go('/ls/designs'));
const exportLabel = label({ en: 'Export', 'pt-BR': 'Exportar', es: 'Exportar' });
if (graphs[0]) await screen('export-graph', async () => (await go('/ls/graphs/' + graphs[0].id), await click(`button:has-text("${exportLabel}")`), await sleep(800)));
if (cycles[0]) await screen('export-cycle', async () => (await go('/ls/cycles/' + cycles[0].id), await click(`button:has-text("${exportLabel}")`), await sleep(800)));
const tabs = ['general', 'profile', 'account', 'storage', 'units', 'backups', 'updates', 'mobile', 'privacy', 'about'];
const settingsLabel = label({ en: 'Settings', 'pt-BR': 'Configurações', es: 'Configuración' });
for (const [i, tab] of tabs.entries()) await screen('settings-' + tab, async () => (await go('/'), await menu(settingsLabel), await page.locator('.modal nav button').nth(i).click({ timeout: 3000 })));
await screen('mystuff', async () => (await go('/'), await menu(label({ en: 'My Stuff', 'pt-BR': 'Minhas coisas', es: 'Mis cosas' }))));
await screen('help', () => page.keyboard.press('F1'));
await screen('whatsnew', async () => (await go('/'), await menu(label({ en: "What's New", 'pt-BR': 'Novidades', es: 'Novedades' }))));

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
// The guided tutorial: it opens once on first use, every stop fits the window
// and points at its control on the real screen, the last one closes it for
// good, and it can be skipped, jumped by chapter and reopened from Help.
const coach = () => page.locator('.coach-card');
const settled = async () => {
  await page.locator('.coach-card:not(.waiting)').waitFor({ timeout: 5000 });
  await sleep(450);
};
const tutorialSeen = async () => (await api(page, 'GET', '/api/state')).profile?.settings?.onboarding?.tutorial === true;
try {
  await onboarding({ tutorial: false });
  report.check('the tutorial opens on first use', (await coach().count()) === 1);
  const total = Number(await page.locator('.coach-bar').getAttribute('aria-valuemax'));
  report.check('the tutorial has its stops', total >= 40, String(total));
  for (let k = 0; k < total; k++) {
    if (k) await page.locator('.coach-foot button.primary').click({ timeout: 3000 });
    await settled();
    const n = `tour-${String(k + 1).padStart(2, '0')}`;
    const stop = await coach().getAttribute('data-stop');
    await page.screenshot({ path: path.join(out, `${n}.png`) });
    for (const p of await layoutProblems(page, keys)) report.check(`${n} ${stop}`, false, p);
    const card = await coach().boundingBox();
    report.check(`tutorial ${stop} fits the window`, !!card && card.x >= 0 && card.y >= 0 && card.x + card.width <= W + 0.5 && card.y + card.height <= H + 0.5, JSON.stringify(card));
    const body = await page.locator('.coach-body').evaluate((e) => e.scrollHeight - e.clientHeight);
    report.check(`tutorial ${stop} shows all its text`, body <= 1, String(body));
    if ((await coach().getAttribute('data-expect')) === '1') {
      const hole = await page.locator('.coach-hole').boundingBox();
      report.check(`tutorial ${stop} points at its control`, !!hole && hole.width > 0 && hole.height > 0);
    }
  }
  await page.locator('.coach-foot button.primary').click({ timeout: 3000 });
  await sleep(800);
  report.check('the tutorial closes on the last stop', (await page.locator('.coach').count()) === 0);
  report.check('the tutorial is marked as seen', await tutorialSeen());
  await page.reload();
  await page.waitForLoadState('networkidle');
  await sleep(800);
  report.check('the tutorial is not shown again', (await page.locator('.coach').count()) === 0);

  await onboarding({ tutorial: false });
  await page.locator('.coach-foot button.primary').click({ timeout: 3000 });
  await settled();
  await page.locator('.coach-foot button.ghost').first().click({ timeout: 3000 });
  await sleep(800);
  report.check('the tutorial can be skipped', (await page.locator('.coach').count()) === 0 && (await tutorialSeen()));

  await onboarding({ tutorial: false });
  await settled();
  await page.keyboard.press('ArrowRight');
  await settled();
  report.check('the arrow keys move through the tutorial', (await page.locator('.coach-bar').getAttribute('aria-valuenow')) === '2');
  await page.keyboard.press('Escape');
  await sleep(800);
  report.check('Esc closes the tutorial', (await page.locator('.coach').count()) === 0);

  await page.keyboard.press('F1');
  await sleep(600);
  await page.locator('.modal button:has-text("' + label({ en: 'Open the guided tutorial', 'pt-BR': 'Abrir o tutorial guiado', es: 'Abrir el tutorial guiado' }) + '")').click({ timeout: 3000 });
  await settled();
  report.check('Help reopens the tutorial', (await coach().getAttribute('data-stop')) === 'tut.welcome');
  await page.locator('.coach-card .row.gap3 button').click({ timeout: 3000 });
  await sleep(500);
  await page.screenshot({ path: path.join(out, 'tour-chapters.png') });
  for (const p of await layoutProblems(page, keys)) report.check('tour-chapters', false, p);
  await page.locator('.coach-chapters button').nth(3).click({ timeout: 3000 });
  await settled();
  report.check('a chapter can be opened directly', (await coach().getAttribute('data-stop')) === 'tut.graphs_library', await coach().getAttribute('data-stop'));
  await page.keyboard.press('Escape');
  await sleep(600);
  await page.keyboard.press('F1');
  await sleep(600);
  await page.locator('.help-chapter').nth(4).click({ timeout: 3000 });
  await settled();
  report.check('Help opens a tutorial chapter', (await coach().getAttribute('data-stop')) === 'tut.cycles_library', await coach().getAttribute('data-stop'));
  await page.keyboard.press('Escape');
  await sleep(600);
} catch (e) {
  report.check('tutorial', false, e.message.split('\n')[0]);
}
await screen('ls-intro', async () => (await onboarding({ ls: false }), await go('/ls/files')));
await onboarding({});
if (graphs.length) {
  await screen('present', async () => (await api(page, 'POST', '/api/present', { action: 'start', graphs: graphs.map((g) => g.id) }), await sleep(1200)));
  await api(page, 'POST', '/api/present', { action: 'stop' });
}

for (const e of errors) report.check('no errors', false, e);
await browser.close();
process.exit(report.finish(out));
