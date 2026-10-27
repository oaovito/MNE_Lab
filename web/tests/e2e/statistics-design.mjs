// Statistical Analysis from a saved experimental design (addendum B §32-38,
// A §9-15, §57-59): 24 synthetic measurements of units A, B and C under
// F-SR and C-SR at days 0, 7, 14 and 21. The editor takes the structure from
// the design, the pseudoreplication guard refuses independence with the
// owner's message, the review card appears before the calculation, and the
// classical paired analysis (base R only, so it runs offline) shows
// Condition, Time, Condition × Time, simple effects and the units' own
// observations in every language. Every file and value here is invented.
import fs from 'node:fs';
import path from 'node:path';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'statistics-design');
fs.mkdirSync(out, { recursive: true });
const report = new Report('statistics-design');
const keys = translationKeys();
const browser = await launchBrowser();
const messages = [];
let page;
const days = [0, 7, 14, 21];
const owner = {
  en: 'These measurements appear to originate from the same experimental unit. Treating them as independent may inflate the effective sample size and produce misleading p-values.',
};
const words = {
  en: { units: 'Experimental units: 3', within: 'Within-unit factor: Condition', n: 'N = 3 independent experimental units (24 measurements)', interaction: 'Condition × Time interaction', simple: 'Simple effects', individual: 'Individual unit observations', review: 'Experimental design review' },
  'pt-BR': { units: 'Unidades experimentais: 3', within: 'Fator intraunidade: Condição', n: 'N = 3 unidades experimentais independentes (24 medições)', interaction: 'Interação Condition × Time', simple: 'Efeitos simples', individual: 'Observações individuais das unidades', review: 'Revisão do desenho experimental' },
  es: { units: 'Unidades experimentales: 3', within: 'Factor intraunidad: Condición', n: 'N = 3 unidades experimentales independientes (24 mediciones)', interaction: 'Interacción Condition × Time', simple: 'Efectos simples', individual: 'Observaciones individuales de las unidades', review: 'Revisión del diseño experimental' },
};
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
  // Invented values: a unit offset, a condition shift, a slow drift and a
  // deterministic jitter so that every error stratum has variation.
  const offset = { A: 0, B: 6.5, C: -4.2 };
  let n = 0;
  for (const unit of ['A', 'B', 'C'])
    for (const cond of ['F-SR', 'C-SR'])
      for (const day of days) {
        n++;
        const value = 150 + offset[unit] + (cond === 'C-SR' ? -3.1 : 0) + day * 0.42 + ((n * n * 7) % 13) / 9;
        await importFile(`SynthStat ${cond} ${unit} day ${day}.txt`, `Sample ID: SynthStat ${cond} ${unit} d${day}\nEffective Diameter (nm): ${value.toFixed(3)}\n`);
      }
  const files = await request('GET', '/api/files');
  const idOf = {};
  for (const f of files) if (f.name.startsWith('SynthStat ')) idOf[f.name] = f.items[0].id;
  const units = {};
  for (const label of ['A', 'B', 'C']) units[label] = (await request('POST', '/api/experimental-units', { label })).id;
  const draft = {
    name: 'Synthetic stability for statistics',
    units: Object.values(units).map((id) => ({ id })),
    conditions: [{ label: 'F-SR' }, { label: 'C-SR', derivedFrom: 'F-SR', process: 'centrifugation' }],
    times: days.map((d) => ({ label: `Day ${d}`, offset: d, unit: 'days' })),
    assignments: [],
    pairs: Object.values(units).map((id) => ({ unitId: id, from: 'F-SR', to: 'C-SR' })),
  };
  for (const unit of ['A', 'B', 'C'])
    for (const cond of ['F-SR', 'C-SR'])
      for (const day of days) draft.assignments.push({ measurementId: idOf[`SynthStat ${cond} ${unit} day ${day}.txt`], unitId: units[unit], condition: cond, time: `Day ${day}` });
  const view = await request('POST', '/api/designs', { design: draft });
  const designId = view.design.id;
  report.check('design detected as paired longitudinal', view.structure.detected === 'paired_longitudinal' && view.structure.experimentalUnits === 3);
  const initial = (await request('GET', '/api/statistics')).length;

  // Design screen → Analyze opens Statistics with the design.
  let external = 0;
  await page.route('**/*', (route) => {
    if (new URL(route.request().url()).origin === new URL(page.url()).origin) return route.continue();
    external++;
    return route.abort();
  });
  await page.goto(new URL('/ls/designs/' + designId, page.url()).href);
  await page.waitForLoadState('networkidle');
  await page.getByRole('button', { name: 'Analyze', exact: true }).click();
  await page.waitForURL(/\/ls\/statistics\/new\?design=/);
  await page.getByText('Units, conditions and times come from “Synthetic stability for statistics”, revision 1: 3 experimental units, 24 measurements.', { exact: false }).waitFor();
  report.check('structure prefilled from the design, not rebuilt', (await page.getByRole('combobox', { name: 'Design layout', exact: true }).inputValue()) === 'within_within' && (await page.getByRole('button', { name: 'Choose measurements', exact: true }).isDisabled()));
  const unitSelects = page.locator('table select');
  report.check('per-row units are read-only', (await unitSelects.count()) === 24 && (await unitSelects.first().isDisabled()));
  const reviewed = page.getByRole('checkbox', { name: 'I reviewed the experimental units and the independence or repetition of these observations.', exact: true });
  const prepare = page.getByRole('button', { name: 'Review design and sources', exact: true });

  // Pseudoreplication guard: independence over the design's units is refused.
  await page.getByRole('combobox', { name: 'Experimental structure', exact: true }).selectOption('independent');
  await reviewed.check();
  await prepare.click();
  await page.getByText(owner.en, { exact: false }).waitFor();
  report.check('independence refused with the owner message', (await page.locator('.notice').filter({ hasText: owner.en }).count()) >= 1);
  await page.locator('.notice').filter({ hasText: owner.en }).first().scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, 'pseudoreplication-refused.png') });

  await page.getByRole('combobox', { name: 'Experimental structure', exact: true }).selectOption('repeated');
  await page.getByRole('combobox', { name: 'Design layout', exact: true }).selectOption('within_within');
  report.check('simple effects with Holm proposed', (await page.getByRole('combobox', { name: 'Multiple comparisons', exact: true }).inputValue()) === 'simple_effects' && (await page.getByRole('combobox', { name: 'Multiplicity correction', exact: true }).inputValue()) === 'holm');
  await reviewed.check();
  await prepare.click();
  const card = page.locator('.stat-review');
  await card.waitFor();
  const cardText = await card.innerText();
  report.check('review card before Calculate', (await page.getByRole('button', { name: 'Run analysis locally', exact: true }).count()) === 1 && (await page.getByRole('button', { name: 'Save analysis', exact: true }).count()) === 0);
  for (const want of [words.en.units, 'A · B · C', words.en.within, 'F-SR / C-SR', 'Within-unit factor: Time', 'Day 0 / Day 7 / Day 14 / Day 21', words.en.n, 'One reading per unit and cell'])
    report.check('review card states ' + want, cardText.includes(want), cardText.slice(0, 200));
  report.check('review card offers correction in the design', (await card.getByRole('button', { name: 'Correct in the design', exact: true }).count()) === 1);
  report.check('review card cells count readings per unit', (await card.locator('tbody tr').count()) === 3 && (await card.locator('thead th').count()) === 9);
  report.check('review does not save results', (await request('GET', '/api/statistics')).length === initial);
  await card.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, 'review-card-en.png') });

  await page.getByRole('button', { name: 'Run analysis locally', exact: true }).click();
  await page.getByRole('button', { name: 'Save analysis', exact: true }).waitFor({ timeout: 150000 });
  for (const term of ['A', 'B', 'A:B']) report.check('separate section for ' + term, (await page.locator(`.stat-effect[data-term="${term}"]`).count()) === 1);
  const body = await page.locator('body').innerText();
  for (const want of ['Condition effect (F-SR / C-SR)', 'Time effect (Day 0 / Day 7 / Day 14 / Day 21)', words.en.interaction, words.en.simple, 'Condition at each Time', 'Time within F-SR', 'Holm correction within each family', 'Unadjusted 95% CI', words.en.individual, 'Every p-value here rests on 3 units', 'generalized η²', 'Greenhouse–Geisser', 'Lower bound'])
    report.check('results show ' + want, body.includes(want));
  report.check('sphericity note when not testable', body.includes('Sphericity could not be tested') && body.includes('mixed'));
  await page.locator('.stat-effect[data-term="A"]').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, 'results-en.png') });
  await page.locator('.stat-simple').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, 'results-simple-effects-en.png') });
  await page.locator('.stat-units').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(out, 'results-units-en.png') });
  await page.getByRole('button', { name: 'Save analysis', exact: true }).click();
  await page.waitForURL(/\/ls\/statistics\/[^/n][^/]*$/);
  const savedId = new URL(page.url()).pathname.split('/').pop();
  const saved = await request('GET', '/api/statistics/' + savedId);
  const a = saved.results.terms.find((x) => x.source === 'A');
  const b = saved.results.terms.find((x) => x.source === 'B');
  report.check('design revision recorded in the snapshot', saved.snapshot.definition.designId === designId && saved.snapshot.experimentalDesign?.revision === 1 && saved.snapshot.unitLabels?.[units.B] === 'B');
  report.check('df as returned: 3 units, never 24 subjects', saved.snapshot.design.experimentalUnits === 3 && a.denominatorDF === 2 && b.denominatorDF === 6 && saved.results.groups.every((g) => g.n === 3));
  report.check('simple effects named with their correction', saved.results.comparisons.length === 4 + 2 * 6 && saved.results.comparisons.every((c) => c.correction.startsWith('Holm')) && saved.results.simpleEffects?.length === 2, JSON.stringify({ n: saved.results.comparisons.length, se: saved.results.simpleEffects?.length, c: saved.results.comparisons[0]?.correction }));
  report.check('calculation does not request an external service', external === 0);
  await page.unroute('**/*');

  for (const [language, theme, width, height] of [
    ['en', 'light', 1366, 768],
    ['pt-BR', 'light', 1366, 768],
    ['pt-BR', 'dark', 1024, 600],
    ['es', 'light', 1366, 768],
    ['es', 'dark', 1024, 600],
  ]) {
    await settings(language, theme);
    await page.setViewportSize({ width, height });
    const w = words[language];
    await page.goto(new URL('/ls/statistics/' + savedId, page.url()).href);
    await page.waitForLoadState('networkidle');
    await page.locator('.stat-paired').waitFor();
    await sleep(300);
    const text = await page.locator('body').innerText();
    report.check(`saved results sections ${language}`, [w.interaction, w.simple, w.individual].every((x) => text.includes(x)), [w.interaction, w.simple, w.individual].filter((x) => !text.includes(x)).join(' | '));
    let problems = await layoutProblems(page, keys);
    report.check(`saved results layout ${language}/${theme}/${width}x${height}`, problems.length === 0, problems.join('; '));
    await page.screenshot({ path: path.join(out, `results-${language}-${theme}-${width}x${height}.png`) });
    // The review card in this language, before any calculation.
    await page.goto(new URL('/ls/statistics/new?design=' + designId, page.url()).href);
    await page.waitForLoadState('networkidle');
    await page.locator('.stat-design-pick').waitFor();
    await sleep(500);
    await page.locator('fieldset input[type=checkbox]').first().check();
    await page.locator('fieldset > button.primary').last().click();
    await page.locator('.stat-review').waitFor();
    const reviewText = await page.locator('.stat-review').innerText();
    report.check(`review card ${language}`, [w.review, w.units, w.within, w.n].every((x) => reviewText.includes(x)), reviewText.slice(0, 160));
    problems = await layoutProblems(page, keys);
    report.check(`review card layout ${language}/${theme}/${width}x${height}`, problems.length === 0, problems.join('; '));
    await page.locator('.stat-review').scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(out, `review-${language}-${theme}-${width}x${height}.png`) });
  }
  await settings('en', 'light');
  report.check('no script errors', messages.length === 0, messages.slice(0, 4).join('; '));
} catch (e) {
  report.check('statistics from a design completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), (e.stack || String(e)) + '\n' + messages.join('\n'));
  if (page) await page.screenshot({ path: path.join(out, 'failure.png') }).catch(() => {});
} finally {
  await browser.close();
}
process.exit(report.finish(out));
