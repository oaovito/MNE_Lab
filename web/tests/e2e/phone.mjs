// Phone browser test: pairs a phone with the QR link, uses the Viewer and the
// presentation controller, ends the session and checks that the link cannot
// be used again.
//
//   node tests/e2e/phone.mjs <harness dir> <out dir> <lang>
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys, watch } from './lib.mjs';

const [dir, outRoot, lang = 'en'] = process.argv.slice(2);
const out = path.join(outRoot, `phone-${lang}`);
fs.mkdirSync(out, { recursive: true });
const report = new Report(`phone-${lang}`);
const keys = translationKeys();
const errors = [];

const browser = await launchBrowser();
const desk = await (await browser.newContext({ viewport: { width: 1366, height: 768 }, locale: lang })).newPage();
watch(desk, errors, 'desktop');
await desk.goto(await launchURL(dir));
await desk.waitForLoadState('networkidle');
const st0 = await api(desk, 'GET', '/api/state');
await api(desk, 'PUT', '/api/profile/settings', { language: lang, theme: 'dark', onboarding: { tour: true, ls: true, mobile: true }, whatsNewSeen: st0.version });

const info = await api(desk, 'POST', '/api/mobile/start');
report.check('mobile access starts with a QR code', info.active && String(info.qr || '').startsWith('<svg'));

const phone = await (await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, locale: lang })).newPage();
watch(phone, errors, 'phone');
const snap = async (name) => {
  await sleep(700);
  await phone.screenshot({ path: path.join(out, `${name}.png`) });
  for (const p of await layoutProblems(phone, keys)) report.check(name, false, p);
};

await phone.goto(info.url);
await phone.waitForSelector('.m-app', { timeout: 10000 }).catch(() => {});
report.check('phone pairs and opens', (await phone.locator('.m-app').count()) > 0);
await snap('home');
report.check('desktop lists the paired phone', ((await api(desk, 'GET', '/api/mobile')).devices || []).length === 1);

await phone.locator('.m-nav button').nth(1).click();
await snap('graphs');
await phone.locator('.m-item-main').first().click();
await sleep(1200);
await snap('graph');
report.check('graph is drawn on the phone', (await phone.locator('svg polyline').count()) > 0);
await phone.locator('.m-nav button').nth(2).click();
await snap('cycles');

const graphs = await api(desk, 'GET', '/api/graphs');
await api(desk, 'POST', '/api/present', { action: 'start', graphs: graphs.map((g) => g.id) });
await sleep(800);
await phone.locator('.m-nav button').nth(3).click();
await snap('controller');
await phone.locator('.m-big.primary').click();
await sleep(800);
report.check('phone moves the presentation', (await api(desk, 'GET', '/api/state')).present?.index === 1);
await phone.locator('.m-switch').first().click();
await sleep(600);
report.check('phone hides the legend', (await api(desk, 'GET', '/api/state')).present?.noLegend === true);
await phone.locator('.m-btn.danger').click();
await sleep(800);
report.check('phone ends the presentation', !(await api(desk, 'GET', '/api/state')).present?.active);

await api(desk, 'POST', '/api/mobile/stop');
await sleep(2500);
await snap('ended');
report.check('phone says the session ended', (await phone.locator('.m-center').count()) > 0);

const again = await (await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true })).newPage();
await again.goto(info.url).catch(() => {});
await sleep(1500);
report.check('a used link cannot pair again', (await again.locator('.m-app').count()) === 0);

// Refusing the used link is the expected answer, not an error.
for (const e of errors.filter((e) => !/\b(401|403|410)\b/.test(e))) report.check('no errors', false, e);
await browser.close();
process.exit(report.finish(out));
