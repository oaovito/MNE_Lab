// Paired condition × time graphs end to end (addendum B §59-61): import 24
// synthetic measurements of units A, B and C under F-SR and C-SR at days
// 0, 7, 14 and 21 → save the design → Graph 1 (F-SR) and Graph 2 (C-SR)
// from the design → Combine them into Graph 3 (a new definition from data;
// the parents stay byte-identical and lineage links both ways; a graph of
// another variable is refused) → Analyze (classical paired analysis, base R
// only, offline) → annotate Graph 3 from the saved analysis → export the
// research package → reopen with the pairing intact. Runs in every
// language, light at 1366x768 and dark at 1024x600. Every value here is
// invented.
//
//   node tests/e2e/paired.mjs <harness dir> <out dir>
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'paired');
fs.mkdirSync(out, { recursive: true });
const report = new Report('paired');
const keys = translationKeys();
const browser = await launchBrowser();
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..');
const goText = Object.fromEntries(['en', 'pt-BR', 'es'].map((l) => [l, JSON.parse(fs.readFileSync(path.join(root, 'internal/i18n/locales', l + '.json'), 'utf8'))]));
const days = [0, 7, 14, 21];
const DESIGN = 'Synthetic paired graphs';
let designId = '';
const ids = {}; // measurement IDs by "cond unit day"

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
  const T = goText[language];
  const errors = [];
  const page = await browser.newPage({ viewport: { width: W, height: H } });
  page.on('pageerror', (e) => errors.push('script error: ' + e.message));
  page.on('console', (m) => m.type() === 'error' && !m.text().startsWith('Failed to load resource') && errors.push('console: ' + m.text()));
  let rendered = null;
  page.on('response', async (r) => {
    if (r.url().endsWith('/api/graphs/render') && r.status() === 200) rendered = await r.json().catch(() => rendered);
  });
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
    await sleep(300);
    await page.waitForLoadState('networkidle');
    await page.locator('.figure-wrap .fig-progress').waitFor({ state: 'detached', timeout: 15000 }).catch(() => {});
    await sleep(300);
  };
  const narrow = W < 1180;
  // The inspector is a drawer on narrow windows.
  const inspector = async (open) => {
    if (!narrow) return;
    const on = await page.locator('.builder-right.open').count();
    if (!!on !== open) await page.locator('.page-head .narrow-only').click();
    await sleep(250);
  };
  const rightTab = async (n) => {
    await inspector(true);
    await page.locator(`.builder-right .tabs .tab:nth-child(${n})`).click();
    await sleep(250);
  };
  const figureText = () => page.locator('.figure-wrap svg text').allTextContents().then((x) => x.join(' | '));
  try {
    await page.goto(await launchURL(dir));
    await page.waitForLoadState('networkidle');
    const state = await api(page, 'GET', '/api/state');
    await api(page, 'PUT', '/api/profile/settings', { language, theme, onboarding: { tutorial: true, ls: true, turbo: true, exit: true, locked: true, mobile: true, cycles: true, 'tip.portable': true, 'tip.temporary': true, 'tip.cloud': true }, whatsNewSeen: state.version });
    await page.reload();
    await page.waitForLoadState('networkidle');
    const scope = { 'X-Account-ID': state.account.id, 'X-Profile-ID': state.profile.id, 'X-MNE-Lab': '1' };
    const request = async (method, p, data) => {
      const res = await page.request.fetch(new URL(p, page.url()).href, { method, headers: scope, data });
      if (!res.ok()) throw new Error(`${method} ${p}: ${res.status()} ${await res.text()}`);
      return res.json();
    };
    const raw = async (p) => (await page.request.get(new URL(p, page.url()).href, { headers: scope })).text();

    // ---- import and design (once) ----
    if (!designId) {
      const offset = { A: 0, B: 5.5, C: -3.8 };
      let n = 0;
      for (const unit of ['A', 'B', 'C'])
        for (const cond of ['F-SR', 'C-SR'])
          for (const day of days) {
            n++;
            const value = 140 + offset[unit] + (cond === 'C-SR' ? -6.4 : 0) + day * 0.55 + ((n * n * 5) % 11) / 8;
            const name = `SynthPair ${cond} ${unit} day ${day}.txt`;
            const bytes = Buffer.from(`Sample ID: SynthPair ${cond} ${unit} d${day}\nEffective Diameter (nm): ${value.toFixed(3)}\nPolydispersity: ${(0.11 + n / 400).toFixed(4)}\n`);
            const headers = { ...scope, 'X-File-Name': encodeURIComponent(name), 'Content-Type': 'application/octet-stream' };
            const review = await page.request.post(new URL('/api/import/inspect', page.url()).href, { headers, data: bytes }).then((r) => r.json());
            const ok = await page.request.post(new URL('/api/import/confirm', page.url()).href, { headers: { ...headers, 'X-Import-Receipt': review.receipt }, data: bytes });
            if (!ok.ok()) throw new Error('synthetic import failed: ' + name);
          }
      for (const f of await request('GET', '/api/files')) {
        const m = f.name.match(/^SynthPair (\S+) (\S) day (\d+)\.txt$/);
        if (m) ids[`${m[1]} ${m[2]} ${m[3]}`] = f.items[0].id;
      }
      report.check('24 synthetic measurements imported', Object.keys(ids).length === 24, String(Object.keys(ids).length));
      const units = {};
      for (const label of ['A', 'B', 'C']) units[label] = (await request('POST', '/api/experimental-units', { label })).id;
      const draft = {
        name: DESIGN,
        units: Object.values(units).map((id) => ({ id })),
        conditions: [{ label: 'F-SR' }, { label: 'C-SR', derivedFrom: 'F-SR', process: 'centrifugation' }],
        times: days.map((d) => ({ label: `Day ${d}`, offset: d, unit: 'days' })),
        assignments: [],
        pairs: Object.values(units).map((id) => ({ unitId: id, from: 'F-SR', to: 'C-SR' })),
      };
      for (const unit of ['A', 'B', 'C']) for (const cond of ['F-SR', 'C-SR']) for (const day of days) draft.assignments.push({ measurementId: ids[`${cond} ${unit} ${day}`], unitId: units[unit], condition: cond, time: `Day ${day}` });
      const view = await request('POST', '/api/designs', { design: draft });
      designId = view.design.id;
      report.check('design is paired longitudinal with 3 units', view.structure.detected === 'paired_longitudinal' && view.structure.experimentalUnits === 3);
    }

    // ---- Graph 1 and Graph 2 from the design ----
    const fromDesign = async (keep, title) => {
      await go('/ls/designs/' + designId);
      if (keep === 'F-SR') await shot('design');
      await page.locator('.dsg-graph').click();
      await page.waitForURL(/\/ls\/graphs\/new\?kind=design_time/);
      await settle();
      await inspector(true);
      const drop = keep === 'F-SR' ? 'C-SR' : 'F-SR';
      await page.locator('.dg-conditions label').filter({ hasText: drop }).click();
      await settle();
      await inspector(false);
      await page.locator('.page-head input').first().fill(title);
      await page.locator('.gb-save').click();
      await page.waitForURL(/\/ls\/graphs\/(?!new)[^/?]+$/);
      await settle();
      return new URL(page.url()).pathname.split('/').pop();
    };
    const g1 = await fromDesign('F-SR', `F-SR ${tag}`);
    const def1 = await request('GET', '/api/graphs/' + g1);
    report.check(`${tag} Graph 1 is a design graph of F-SR`, def1.kind === 'design_time' && def1.designId === designId && JSON.stringify(def1.conditions) === '["F-SR"]' && def1.measurements.length === 12, JSON.stringify({ k: def1.kind, c: def1.conditions, m: def1.measurements?.length }));
    const mean1 = rendered?.series?.find((s) => s.kind === 'mean');
    const units1 = rendered?.series?.find((s) => s.kind === 'individual');
    report.check(`${tag} n counts 3 units at each day, SD by default`, rendered?.errorBars === 'sd' && JSON.stringify(mean1?.n) === '[3,3,3,3]' && mean1?.err?.length === 4, JSON.stringify({ e: rendered?.errorBars, n: mean1?.n }));
    report.check(`${tag} each unit's value is drawn`, units1?.y?.length === 12 && units1.pointLabels?.length === 12);
    let text = await figureText();
    report.check(`${tag} error bars and n are labelled`, text.includes(T['series.design_mean_sd']) && text.includes(T['plot.meta.design_n'].split('{n}')[0].trim()), text.slice(0, 300));
    await shot('graph1');
    const g2 = await fromDesign('C-SR', `C-SR ${tag}`);
    const def2 = await request('GET', '/api/graphs/' + g2);
    report.check(`${tag} Graph 2 is a design graph of C-SR`, def2.kind === 'design_time' && JSON.stringify(def2.conditions) === '["C-SR"]');
    // A graph of another variable, to see Combine refuse it.
    const other = await request('POST', '/api/graphs', { title: `PDI ${tag}`, kind: 'design_time', designId, conditions: ['F-SR'], param: 'polydispersity', visual: { legend: true, metadata: true, grid: true, points: false, lineWidth: 1.75, fontScale: 1 } });
    const before = { [g1]: await raw('/api/graphs/' + g1), [g2]: await raw('/api/graphs/' + g2) };

    // ---- Analyze from Graph 1: one condition, the other rows excluded with a reason ----
    await go('/ls/graphs/' + g1);
    await settle();
    await inspector(false);
    await page.locator('.gb-analyze').click();
    await page.waitForURL(/\/ls\/statistics\/new\?design=/);
    await page.locator('.stat-graph-one').waitFor();
    await sleep(600);
    // The exclusion reason is the last field of each measurement row.
    const reasons = await page.locator('table tbody tr').evaluateAll((rows) => rows.map((tr) => [...tr.querySelectorAll('input.input')].pop()?.value).filter((v) => v !== undefined));
    report.check(`${tag} Analyze from Graph 1 keeps the design's 24 rows, 12 excluded as not shown`, reasons.length === 24 && reasons.filter(Boolean).length === 12 && reasons.filter(Boolean).every((v) => v.includes(`F-SR ${tag}`)), JSON.stringify(reasons));
    const one = await page.locator('select').evaluateAll((els) => els.map((e) => e.value));
    report.check(`${tag} one condition proposes Time within units, not Condition × Time`, !one.includes('within_within') && one.includes('repeated'), one.join(','));
    report.check(`${tag} the subset and the route are stated`, (await page.locator('.stat-graph-subset').count()) === 1);
    await shot('analyze-one-condition');
    await page.locator('fieldset input[type=checkbox]').first().check();
    await page.locator('.stat-prepare').click();
    await page.locator('.stat-run').waitFor({ timeout: 15000 }).catch(() => {});
    report.check(`${tag} the core accepts the graph's rows against the saved design`, (await page.locator('.stat-run').count()) === 1, await page.locator('.notice.danger').allInnerTexts().then((x) => x.join(' | ')));

    // ---- Combine from the Graph Library ----
    await go('/ls/graphs');
    await page.locator('.graphs-kind select').selectOption('design_time');
    await sleep(300);
    for (const t of [`F-SR ${tag}`, `PDI ${tag}`]) await page.locator('.gcard').filter({ hasText: t }).locator('.sel').click();
    await page.locator('.graphs-combine').click();
    await page.locator('.combine-dialog').waitFor();
    await page.locator('.gc-verdict').waitFor();
    report.check(`${tag} another variable is refused`, (await page.locator('.gc-verdict[data-compatible="no"] .gc-problem[data-code="graph.combine_variable"]').count()) === 1 && (await page.locator('.gc-create').isDisabled()));
    await shot('combine-refused');
    await page.locator('.gc-row').filter({ hasText: `PDI ${tag}` }).click();
    await page.locator('.gc-row').filter({ hasText: `C-SR ${tag}` }).click();
    await page.locator('.gc-verdict[data-compatible="yes"]').waitFor();
    await page.locator('.combine-dialog input:not([type=checkbox])').last().fill(`Graph 3 ${tag}`);
    await shot('combine-ok');
    await page.locator('.gc-create').click();
    await page.waitForURL(/\/ls\/graphs\/(?!new)[^/?]+$/);
    await settle();
    const g3 = new URL(page.url()).pathname.split('/').pop();
    const def3 = await request('GET', '/api/graphs/' + g3);
    report.check(`${tag} Graph 3 is a new definition derived from both`, ![g1, g2].includes(g3) && def3.derivedFrom?.length === 2 && def3.derivedFrom.every((l) => l.relation === 'combine' && l.definitionHash) && JSON.stringify(def3.derivedFrom.map((l) => l.graphId).sort()) === JSON.stringify([g1, g2].sort()));
    report.check(`${tag} Graph 3 draws both conditions on one time axis`, def3.kind === 'design_time' && JSON.stringify(def3.conditions) === '["F-SR","C-SR"]' && rendered?.series?.filter((s) => s.kind === 'mean').length === 2);
    const means = rendered?.series?.filter((s) => s.kind === 'mean') || [];
    report.check(`${tag} conditions have distinct markers and line styles`, means.length === 2 && means[0].marker !== means[1].marker && means[0].lineStyle !== means[1].lineStyle, JSON.stringify(means.map((s) => [s.marker, s.lineStyle])));
    report.check(`${tag} Graphs 1 and 2 stay byte-identical`, (await raw('/api/graphs/' + g1)) === before[g1] && (await raw('/api/graphs/' + g2)) === before[g2]);
    await shot('graph3');

    // ---- lineage both ways ----
    await rightTab(3);
    await page.locator('.gb-lineage').waitFor();
    report.check(`${tag} Graph 3 lists both parents`, (await page.locator('.gb-lineage .lineage-parent').count()) === 2);
    await shot('graph3-lineage');
    await page.locator('.gb-lineage .lineage-parent').filter({ hasText: `F-SR ${tag}` }).click();
    await page.waitForURL(new RegExp('/ls/graphs/' + g1 + '$'));
    await settle();
    await rightTab(3);
    await page.locator('.gb-lineage .lineage-child').first().waitFor();
    report.check(`${tag} Graph 1 shows Graph 3 as derived`, (await page.locator('.gb-lineage .lineage-child').filter({ hasText: `Graph 3 ${tag}` }).count()) === 1);
    await page.locator('.gb-lineage .lineage-child').filter({ hasText: `Graph 3 ${tag}` }).click();
    await page.waitForURL(new RegExp('/ls/graphs/' + g3 + '$'));
    await settle();
    await inspector(false);

    // ---- Analyze (offline) ----
    let external = 0;
    await page.route('**/*', (route) => {
      if (new URL(route.request().url()).origin === new URL(page.url()).origin) return route.continue();
      external++;
      return route.abort();
    });
    await page.locator('.gb-analyze').click();
    await page.waitForURL(/\/ls\/statistics\/new\?design=/);
    await page.locator('.stat-from-graph').waitFor();
    await sleep(600);
    const layout = await page.locator('select').evaluateAll((els) => els.map((e) => e.value));
    report.check(`${tag} Analyze prefills the paired design`, layout.includes('within_within') && layout.includes('effective_diameter'), layout.join(','));
    report.check(`${tag} Analyze takes the 24 measurements of the graph`, (await page.locator('table select').count()) === 24);
    await shot('analyze-prefilled');
    await page.locator('fieldset input[type=checkbox]').first().check();
    await page.locator('.stat-prepare').click();
    await page.locator('.stat-review').waitFor();
    await page.locator('.stat-run').click();
    await page.locator('.stat-save').waitFor({ timeout: 150000 });
    await page.locator('.stat-save').click();
    await page.waitForURL(/\/ls\/statistics\/(?!new)[^/?]+\?graph=/);
    const analysisId = new URL(page.url()).pathname.split('/').pop();
    const saved = await request('GET', '/api/statistics/' + analysisId);
    report.check(`${tag} the analysis counts 3 units`, saved.snapshot.definition.designId === designId && saved.snapshot.design.experimentalUnits === 3 && saved.results.groups.every((g) => g.n === 3));
    report.check(`${tag} the calculation stays offline`, external === 0);
    await page.unroute('**/*');
    await page.locator('.stat-annotate').waitFor();
    await shot('analysis-saved');

    // ---- annotate from the saved analysis ----
    await page.locator('.stat-annotate').click();
    await page.waitForURL(new RegExp(`/ls/graphs/${g3}\\?analysis=${analysisId}`));
    await settle();
    await rightTab(1);
    await page.locator('.gb-sec-annotations').waitFor();
    await page.locator('.gb-sec-annotations select').first().selectOption('effect');
    const boxes = page.locator('.gb-sec-annotations .an-comparison input');
    report.check(`${tag} every saved comparison is offered`, (await boxes.count()) === saved.results.comparisons.length, `${await boxes.count()} of ${saved.results.comparisons.length}`);
    // One bracket between conditions at day 0 and one at day 21, by their stored values.
    const pick = (day) => saved.results.comparisons.findIndex((c) => c.leftB === day && c.rightB === day && c.leftA !== c.rightA);
    const chosen = [pick('Day 0'), pick('Day 21')];
    report.check(`${tag} the analysis compares conditions at each day`, chosen.every((i) => i >= 0), JSON.stringify(saved.results.comparisons.map((c) => c.contrast + ' ' + (c.context || ''))));
    for (const i of chosen) {
      const c = saved.results.comparisons[i];
      await page.locator('.gb-sec-annotations .an-comparison').filter({ hasText: c.contrast }).filter({ hasText: c.context || '' }).locator('input').check();
    }
    await settle();
    await page.locator('.gb-sec-bars .seg button').last().click(); // CI, from the analysis
    await settle();
    await shot('graph3-annotated');
    await inspector(false);
    await page.locator('.gb-save').click();
    await settle();
    const def3a = await request('GET', '/api/graphs/' + g3);
    const anns = rendered?.annotations || [];
    const want = chosen.map((i) => saved.results.comparisons[i]);
    report.check(`${tag} annotations reference the saved comparisons`, def3a.analysisId === analysisId && JSON.stringify(def3a.annotations) === JSON.stringify(want.map((c) => c.id)) && anns.length === 2 && anns.every((a) => a.analysisId === analysisId), JSON.stringify({ a: def3a.annotations, n: anns.length }));
    report.check(
      `${tag} brackets show the stored adjusted p and effect`,
      anns.every((a, i) => a.comparisonId === want[i].id && a.label.startsWith('Δ ') && Math.abs(parseFloat(a.label.split('p(adj)=')[1]) - want[i].adjustedP) <= Math.abs(want[i].adjustedP) * 1e-5 && a.adjustedP === want[i].adjustedP),
      anns.map((a) => a.label).join(' || '),
    );
    report.check(`${tag} CI bars come from the saved analysis`, rendered?.errorBars === 'ci' && def3a.errorBars === 'ci');

    // ---- export the research package ----
    await page.locator('.graph-export').click();
    await page.locator('.intent[data-preset="package"]').click();
    await sleep(800);
    await shot('export');
    const exported = page.waitForResponse((r) => r.url().endsWith('/api/export') && r.request().method() === 'POST');
    await page.locator('.modal-foot .btn.primary').last().click();
    const ex = await (await exported).json();
    const zip = ex.saved?.[0]?.path;
    report.check(`${tag} the package is saved`, !!zip && fs.existsSync(zip), JSON.stringify(ex).slice(0, 200));
    if (zip && fs.existsSync(zip)) {
      const list = execFileSync('unzip', ['-Z1', zip], { encoding: 'utf8' }).split('\n');
      const inner = list.find((x) => x.endsWith('design/design.json'));
      const defs = list.find((x) => x.endsWith('graphs/definitions.json'));
      report.check(`${tag} the package holds design, lineage and the analysis`, !!inner && !!defs && list.some((x) => x.includes('statistics/analyses/' + analysisId + '/')), list.filter((x) => /design|definitions|analyses/.test(x)).join(', '));
      if (inner && defs) {
        const design = JSON.parse(execFileSync('unzip', ['-p', zip, inner], { encoding: 'utf8' }));
        const graphs = JSON.parse(execFileSync('unzip', ['-p', zip, defs], { encoding: 'utf8' }));
        const text = JSON.stringify(design);
        report.check(`${tag} the exported design keeps its 3 pairs`, (text.match(/"from":"F-SR","to":"C-SR"/g) || []).length >= 3, text.slice(0, 200));
        report.check(`${tag} the exported definitions keep the lineage`, JSON.stringify(graphs).includes(g1) && JSON.stringify(graphs).includes(g2));
      }
    }
    await page.keyboard.press('Escape');
    await sleep(300);

    // ---- reopen with the pairing intact ----
    await go('/ls/graphs');
    await page.reload();
    await page.waitForLoadState('networkidle');
    await sleep(400);
    report.check(`${tag} the library marks Graph 3 as combined`, (await page.locator('.gcard').filter({ hasText: `Graph 3 ${tag}` }).locator('.badge').count()) >= 2);
    await shot('library');
    await go('/ls/graphs/' + g3);
    await settle();
    report.check(`${tag} reopened Graph 3 keeps analysis and brackets`, (rendered?.annotations || []).length === 2 && rendered?.design?.id === designId);
    await inspector(true);
    await page.locator('.gb-settings select').first().selectOption('paired_condition');
    await settle();
    const paired = (rendered?.series || []).filter((s) => s.kind === 'paired');
    // The paired view (last day) keeps only the brackets between conditions at that day.
    const atLast = want.filter((c) => c.leftB === 'Day 21' && c.rightB === 'Day 21' && c.leftA !== c.rightA);
    report.check(`${tag} the paired view keeps the brackets of its day`, (rendered?.annotations || []).length === atLast.length && (rendered?.annotations || []).every((a) => atLast.some((c) => c.id === a.comparisonId)), JSON.stringify(want.map((c) => [c.leftA, c.leftB, c.rightA, c.rightB])));
    report.check(`${tag} the paired view joins each unit through its declared pair`, paired.length === 3 && paired.every((s) => s.meta?.pairing === 'declared' && s.y.length === 2), JSON.stringify(paired.map((s) => s.meta)));
    text = await figureText();
    report.check(`${tag} the paired view says lines join the same unit`, text.includes(T['plot.meta.design_pairs']), text.slice(0, 300));
    await inspector(false);
    await shot('paired-view');

    // ---- groups ----
    const grp = await request('POST', '/api/groups', { name: `F-SR units ${tag}`, measurements: days.flatMap((d) => ['A', 'B', 'C'].map((u) => ids[`F-SR ${u} ${d}`])) });
    report.check(`${tag} a group stores references only`, grp.measurements.length === 12 && !('values' in grp));
    await go('/ls/groups');
    await page.locator('.grp-card').first().waitFor();
    await shot('groups');
    await page.locator('.grp-card').filter({ hasText: grp.name }).locator('.grp-graph').click();
    await page.waitForURL(/\/ls\/graphs\/new\?kind=design_time/);
    await settle();
    const gm = (rendered?.series || []).filter((s) => s.kind === 'mean');
    report.check(`${tag} a group feeds a design graph`, gm.length === 1 && JSON.stringify(gm[0].n) === '[3,3,3,3]', JSON.stringify(gm.map((s) => s.label)));
    await page.locator('.builder-left .tabs .tab:nth-child(3)').click();
    await sleep(300);
    await shot('graph-groups');
    report.check(`${tag} no script errors`, errors.length === 0, errors.slice(0, 4).join('; '));
  } catch (e) {
    report.check(`${tag} paired flow completes`, false, e.message.split('\n')[0]);
    fs.writeFileSync(path.join(out, `failure-${tag}.txt`), (e.stack || String(e)) + '\n' + errors.join('\n'));
    await page.screenshot({ path: path.join(out, `failure-${tag}.png`) }).catch(() => {});
  } finally {
    await page.close();
  }
}
await browser.close();
process.exit(report.finish(out));
