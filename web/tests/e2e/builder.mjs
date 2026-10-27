// Series Explorer and Graph Builder (addendum B §58) on the twenty invented
// series the harness seeds from testdata/lightscattering/synthetic-twenty-series.txt
// (S01-S10 under F-SR, S11-S20 under C-SR): search, filter, select three,
// draw, edit the selection, save, reopen, duplicate with lineage, and stop
// superseded previews. The seventeen series left out and every record of
// the file stay as they were. Runs in every language, light at 1366x768 and
// dark at 1024x600, and checks the page itself never scrolls.
//
//   node tests/e2e/builder.mjs <harness dir> <out dir>
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'builder');
fs.mkdirSync(out, { recursive: true });
const report = new Report('builder');
const keys = translationKeys();
const browser = await launchBrowser();
const TWENTY = 'synthetic-twenty-series.txt';
const name = (n) => 'Synthetic S' + String(n).padStart(2, '0');

for (const [language, theme, size] of [
  ['en', 'light', '1366x768'],
  ['pt-BR', 'light', '1366x768'],
  ['es', 'light', '1366x768'],
  ['en', 'dark', '1024x600'],
  ['pt-BR', 'dark', '1024x600'],
  ['es', 'dark', '1024x600'],
]) {
  const tag = `${language}-${theme}-${size}`;
  const [W, H] = size.split('x').map(Number);
  const errors = [];
  const page = await browser.newPage({ viewport: { width: W, height: H } });
  page.on('pageerror', (e) => errors.push('script error: ' + e.message));
  page.on('console', (m) => m.type() === 'error' && !m.text().startsWith('Failed to load resource') && errors.push('console: ' + m.text()));
  // The last preview the core answered, to compare with what was asked.
  let rendered = null;
  page.on('response', async (r) => {
    if (r.url().endsWith('/api/graphs/render') && r.status() === 200) rendered = await r.json().catch(() => rendered);
  });
  const aborted = [];
  page.on('requestfailed', (r) => r.url().endsWith('/api/graphs/render') && aborted.push(r.failure()?.errorText || ''));
  const shot = async (n) => {
    await page.screenshot({ path: path.join(out, `${n}-${tag}.png`) });
    for (const p of await layoutProblems(page, keys)) report.check(`${tag} ${n} layout`, false, p);
  };
  const go = async (route) => {
    await page.evaluate((r) => (history.pushState(null, '', r), dispatchEvent(new PopStateEvent('popstate'))), route);
    await page.waitForLoadState('networkidle');
    await sleep(400);
  };
  const settle = async () => {
    await sleep(250);
    await page.waitForLoadState('networkidle');
    await page.locator('.figure-wrap .fig-progress').waitFor({ state: 'detached', timeout: 15000 }).catch(() => {});
    await sleep(250);
  };
  const row = (n) => page.locator(`.sx-row:has-text("${name(n)}")`).first();
  const search = async (q) => {
    await page.locator('.sx-head input').first().fill(q);
    await sleep(250);
  };
  // The list is windowed: search for a series so its row is drawn, then click it.
  const pick = async (n, opts) => {
    await search(name(n));
    await row(n).click(opts);
  };
  const selectedIds = () => page.locator('.sx-row[aria-selected="true"]').evaluateAll((els) => els.map((e) => e.getAttribute('data-series')));
  try {
    await page.goto(await launchURL(dir));
    await page.waitForLoadState('networkidle');
    const state = await api(page, 'GET', '/api/state');
    await api(page, 'PUT', '/api/profile/settings', { language, theme, onboarding: { tutorial: true, ls: true, turbo: true, exit: true, locked: true, mobile: true, cycles: true, 'tip.portable': true, 'tip.temporary': true, 'tip.cloud': true }, whatsNewSeen: state.version });
    await page.reload();
    await page.waitForLoadState('networkidle');
    const scope = { 'X-Account-ID': state.account.id, 'X-Profile-ID': state.profile.id, 'X-MNE-Lab': '1' };
    const catalog = async () => (await (await page.request.get(new URL('/api/series', page.url()).href, { headers: scope })).json()).series;

    const file = (await api(page, 'GET', '/api/files')).find((f) => f.name === TWENTY);
    report.check(`${tag} the twenty synthetic series are seeded`, file?.items?.length === 20, String(file?.items?.length));
    const record = async () => JSON.stringify(await api(page, 'GET', '/api/files/' + file.id));
    const recordBefore = await record();
    const curves = async () => (await catalog()).filter((s) => s.kind === 'curve' && s.fileId === file.id && s.y.key === 'intensity');
    const curvesBefore = JSON.stringify(await curves());

    // A new graph: the explorer lists every curve, windowed.
    await go('/ls/graphs/new');
    await page.locator('.sx-list').waitFor({ timeout: 10000 });
    await shot('builder-empty');
    await search('twenty');
    const listH = await page.locator('.sx-list > div').first().evaluate((e) => e.getBoundingClientRect().height);
    const drawnRows = await page.locator('.sx-row').count();
    report.check(`${tag} the explorer lists all twenty series`, Math.round(listH) === 20 * 46, String(listH));
    report.check(`${tag} the explorer only draws the rows in view`, drawnRows < 20, String(drawnRows));

    // Filter by condition, then select three.
    await page.locator('.sx-filter-btn').click();
    await page.locator('.sx-filters select').nth(1).selectOption('F-SR');
    await page.locator('.sx-filter-btn').click();
    await search('twenty');
    report.check(`${tag} filtering by condition keeps the F-SR ten`, (await page.locator('.sx-row').count()) === 10, String(await page.locator('.sx-row').count()));
    report.check(`${tag} the filter does not list C-SR series`, (await page.locator(`.sx-row:has-text("${name(15)}")`).count()) === 0);
    // Shift selects a range; Clear empties the selection.
    await row(1).click();
    await row(3).click({ modifiers: ['Shift'] });
    report.check(`${tag} shift selects a range`, (await selectedIds()).length === 3, JSON.stringify(await selectedIds()));
    await page.locator('.sx-clear').click();
    report.check(`${tag} clear empties the selection`, (await selectedIds()).length === 0);
    for (const n of [1, 2, 3]) await pick(n);
    await search('twenty');
    await settle();
    const three = await selectedIds();
    report.check(`${tag} three series selected`, three.length === 3, JSON.stringify(three));
    report.check(`${tag} the preview draws the three`, (await page.locator('.figure svg').count()) > 0 && rendered?.series?.length === 3, String(rendered?.series?.length));
    await shot('builder-three');

    // Edit the selection: drop S02, add S12 from the other condition.
    await pick(2);
    await page.locator('.sx-filter-btn').click();
    await page.locator('.sx-filters button').last().click(); // clear the filters
    await page.locator('.sx-filter-btn').click();
    await pick(12);
    const s12 = await row(12).getAttribute('data-series');
    await search('');
    await settle();
    const want = [three[0], three[2], s12];
    await page.locator('.sx-only').click();
    await sleep(300);
    report.check(`${tag} the edited preview draws S01, S03 and S12`, rendered?.series?.length === 3 && want.every((id) => rendered.series.some((s) => s.seriesId === id)), JSON.stringify(rendered?.series?.map((s) => s.seriesId)));
    // Focus mutes the others without removing them.
    await page.locator(`.sx-row[data-series="${want[0]}"] .sx-focus`).click();
    await settle();
    report.check(`${tag} focus mutes the other selected series`, rendered?.series?.filter((s) => s.muted).length === 2, JSON.stringify(rendered?.series?.map((s) => !!s.muted)));
    await page.locator(`.sx-row[data-series="${want[0]}"] .sx-focus`).click();
    await page.locator('.sx-only').click();
    await settle();
    // Settings drawer on narrow windows.
    if (W < 1180) {
      await page.locator('.page-head .narrow-only').click();
      await sleep(400);
      report.check(`${tag} the settings drawer opens`, await page.locator('.builder-right.open .gb-settings').isVisible());
      await shot('builder-drawer');
      await page.locator('.page-head .narrow-only').click();
      await sleep(300);
    }

    // Save and reopen.
    await page.locator('.page-head input').first().fill(`Builder ${tag}`);
    await page.locator('.builder-bar .gb-save').click();
    await page.waitForURL(/\/ls\/graphs\/(?!new)[^/?]+$/, { timeout: 10000 });
    const id = new URL(page.url()).pathname.split('/').pop();
    const saved = await api(page, 'GET', '/api/graphs/' + id);
    report.check(`${tag} saved by SeriesID`, saved.schema === 2 && JSON.stringify([...saved.seriesIds].sort()) === JSON.stringify([...want].sort()), JSON.stringify(saved.seriesIds));
    report.check(`${tag} saved with parser and calculation versions`, saved.parserVersions?.length > 0 && !!saved.calculationVersion && saved.sourceFiles?.includes(file.id));
    await go('/ls/graphs');
    await go('/ls/graphs/' + id);
    await page.locator('.sx-list').waitFor({ timeout: 10000 });
    await page.locator('.sx-only').click();
    await sleep(300);
    const reopened = await selectedIds();
    report.check(`${tag} reopening shows the same three`, reopened.length === 3 && want.every((x) => reopened.includes(x)), JSON.stringify(reopened));
    await page.locator('.sx-only').click();
    await settle();
    await shot('builder-reopened');
    report.check(`${tag} the seventeen others are still listed`, JSON.stringify(await curves()) === curvesBefore);
    report.check(`${tag} the file and its measurements are unchanged`, (await record()) === recordBefore);

    // Duplicate: a new graph that records where it came from.
    const originalBefore = JSON.stringify(await api(page, 'GET', '/api/graphs/' + id));
    await page.locator('.builder-bar .gb-duplicate').click();
    await page.waitForURL((u) => /\/ls\/graphs\/[^/]+$/.test(u.pathname) && !u.pathname.endsWith('/' + id) && !u.pathname.endsWith('/new'), { timeout: 10000 });
    const copyId = new URL(page.url()).pathname.split('/').pop();
    const copy = await api(page, 'GET', '/api/graphs/' + copyId);
    report.check(`${tag} the copy records its origin`, copy.derivedFrom?.[0]?.graphId === id && copy.derivedFrom[0].relation === 'duplicate' && !!copy.derivedFrom[0].definitionHash, JSON.stringify(copy.derivedFrom));
    await page.locator('.sx-list').waitFor({ timeout: 10000 });
    await settle();
    await pick(12);
    await settle();
    await page.locator('.builder-bar .gb-save').click();
    await sleep(800);
    report.check(`${tag} the edited copy keeps two series`, (await api(page, 'GET', '/api/graphs/' + copyId)).seriesIds?.length === 2);
    report.check(`${tag} the original is byte-identical`, JSON.stringify(await api(page, 'GET', '/api/graphs/' + id)) === originalBefore);
    await shot('builder-copy');

    // Superseded previews are stopped; a slow one shows progress and can be stopped.
    await page.route('**/api/graphs/render', async (r) => {
      await sleep(1500);
      await r.continue().catch(() => {});
    });
    aborted.length = 0;
    for (const n of [4, 5, 6]) await pick(n);
    await page.locator('.fig-progress').waitFor({ timeout: 5000 });
    report.check(`${tag} a slow preview shows its progress`, await page.locator('.fig-progress').isVisible());
    await page.screenshot({ path: path.join(out, `builder-progress-${tag}.png`) });
    await page.locator('.fig-progress button').click();
    await sleep(300);
    report.check(`${tag} Stop ends the preview`, (await page.locator('.fig-progress').count()) === 0);
    report.check(`${tag} superseded previews are aborted`, aborted.length >= 2, JSON.stringify(aborted));
    await pick(6);
    await page.unroute('**/api/graphs/render');
    await settle();
    report.check(`${tag} the last preview matches the selection`, rendered?.series?.length === 4, String(rendered?.series?.length));

    // Single values with the Y unit shown in µm (exact factor, display only).
    await go('/ls/graphs/new?kind=series_values');
    await page.locator('.sx-list').waitFor({ timeout: 10000 });
    for (const n of [1, 2]) await pick(n);
    await settle();
    if (W < 1180) await page.locator('.page-head .narrow-only').click();
    await page.locator('.gb-sec-axes select').first().selectOption('µm');
    await settle();
    const c = rendered?.y?.conversion;
    const s0 = rendered?.series?.[0];
    report.check(`${tag} the Y unit is shown in µm with the exact factor`, c?.from === 'nm' && c?.to === 'µm' && c?.factor === '1/1000' && Math.abs(s0.y[0] - s0.yOriginal[0] / 1000) < 1e-12, JSON.stringify(c));
    await shot('builder-values');

    // The library duplicates selected graphs too.
    await go('/ls/graphs');
    const graphsBefore = await api(page, 'GET', '/api/graphs');
    await page.locator('.gcard .sel input').first().check();
    await page.locator('.graphs-duplicate').click();
    await sleep(1200);
    const graphsAfter = await api(page, 'GET', '/api/graphs');
    const fresh = graphsAfter.filter((g) => !graphsBefore.some((b) => b.id === g.id));
    report.check(`${tag} the library duplicates with lineage`, fresh.length === 1 && graphsBefore.some((b) => b.id === fresh[0].derivedFrom?.[0]?.graphId), JSON.stringify(fresh.map((g) => g.derivedFrom)));
    await shot('graphs-library');
    report.check(`${tag} the file is still unchanged`, (await record()) === recordBefore);
    report.check(`${tag} no script errors`, errors.length === 0, errors.join('; '));
  } catch (e) {
    report.check(`${tag} builder workflow completes`, false, e.message.split('\n')[0]);
    fs.writeFileSync(path.join(out, `failure-${tag}.txt`), e.stack || String(e));
    await page.screenshot({ path: path.join(out, `failure-${tag}.png`) }).catch(() => {});
  } finally {
    await page.close();
  }
}
await browser.close();
process.exit(report.finish(out));
