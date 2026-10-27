// Settings → Manage Units in every language: system versus custom units, a
// custom unit used through the column-mapping selector (a diameter field
// never offers count-rate units), protected delete with Archive / Replace
// Usage Then Delete / Cancel, archive and restore, replacement with
// provenance, and deletion of an unused unit. Values are invented.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { transformSync } from 'esbuild';
import { api, launchBrowser, launchURL, layoutProblems, Report, sleep, translationKeys, watch } from './lib.mjs';

const [dir, outRoot] = process.argv.slice(2);
const out = path.join(outRoot, 'units');
fs.mkdirSync(out, { recursive: true });
const report = new Report('measurement-units');
const keys = translationKeys();

// The screen's own texts come from its catalog, so the test follows them.
const src = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', '..', 'src', 'i18n', 'units.ts');
const tmp = path.join(out, 'units-catalog.mjs');
fs.writeFileSync(tmp, transformSync(fs.readFileSync(src, 'utf8'), { loader: 'ts', format: 'esm' }).code);
const { units: catalog } = await import(pathToFileURL(tmp).href);
fs.rmSync(tmp);
const LANGS = ['en', 'pt-BR', 'es'];
const tr = (lang, key, vars = {}) => {
  let s = catalog[key][LANGS.indexOf(lang)];
  s = s.replace(/\{(\w+):([^{}/]*)\/([^{}]*)\}/g, (_, v, one, other) => (Number(vars[v]) === 1 ? one : other));
  for (const k in vars) s = s.split('{' + k + '}').join(String(vars[k]));
  return s;
};
const fixed = {
  en: { settings: 'Settings', cancel: 'Cancel', pick: 'Import files', confirm: 'Import reviewed files', inspect: 'Inspect selected data', done: 'Done', enable: 'Configure DLS mapping', title: 'Map DLS columns manually', column: 'Column number', unit: 'User-assigned unit' },
  'pt-BR': { settings: 'Configurações', cancel: 'Cancelar', pick: 'Importar arquivos', confirm: 'Importar arquivos revisados', inspect: 'Inspecionar dados selecionados', done: 'Concluído', enable: 'Configurar mapeamento DLS', title: 'Mapear colunas DLS manualmente', column: 'Número da coluna', unit: 'Unidade atribuída pelo usuário' },
  es: { settings: 'Configuración', cancel: 'Cancelar', pick: 'Importar archivos', confirm: 'Importar archivos revisados', inspect: 'Inspeccionar datos seleccionados', done: 'Listo', enable: 'Configurar asignación DLS', title: 'Asignar columnas DLS manualmente', column: 'Número de columna', unit: 'Unidad asignada por el usuario' },
};

const browser = await launchBrowser();
try {
  for (const lang of LANGS) {
    const l = fixed[lang];
    const errors = [];
    const page = await browser.newPage({ viewport: { width: 1366, height: 768 } });
    watch(page, errors, lang);
    await page.goto(await launchURL(dir));
    await page.waitForLoadState('networkidle');
    const state = await api(page, 'GET', '/api/state');
    const headers = { 'X-Account-ID': state.account.id, 'X-Profile-ID': state.profile.id };
    const scoped = async (method, p, body) => {
      const r = await page.request.fetch(new URL(p, page.url()).href, { method, headers: { ...headers, 'X-MNE-Lab': '1', 'Content-Type': 'application/json' }, data: body ? JSON.stringify(body) : undefined });
      const text = await r.text();
      return text ? JSON.parse(text) : null;
    };
    const setPrefs = (theme) => api(page, 'PUT', '/api/profile/settings', { language: lang, theme, onboarding: { tutorial: true, ls: true, turbo: true }, whatsNewSeen: state.version });
    await setPrefs('light');
    await page.reload();
    await page.waitForLoadState('networkidle');

    // A custom unit for this run, created through the API, then used by the
    // column-mapping selector.
    const symbol = `Å-${lang}`;
    const custom = await scoped('POST', '/api/measurement-units', { symbol, name: 'Invented length unit', dimension: 'length' });
    report.check(`${lang}: create custom unit`, custom?.symbol === symbol && custom.system === false);

    await page.evaluate(() => (history.pushState(null, '', '/ls/files'), dispatchEvent(new PopStateEvent('popstate'))));
    const data = Buffer.from(`sample,size,counts\nInvented ${lang},1.2300,73.250\nInvented ${lang},2.3400,80.125\n`);
    const filename = `units-${lang}.csv`;
    const [chooser] = await Promise.all([page.waitForEvent('filechooser'), page.getByRole('button', { name: l.pick, exact: true }).first().click()]);
    await chooser.setFiles({ name: filename, mimeType: 'text/csv', buffer: data });
    await page.getByRole('button', { name: l.done, exact: true }).waitFor();
    await page.locator('summary').filter({ hasText: l.title }).click();
    await page.getByRole('button', { name: l.enable, exact: true }).click();
    const columns = page.getByRole('spinbutton', { name: new RegExp(l.column + '$') });
    const unitBoxes = page.getByRole('combobox', { name: new RegExp(l.unit + '$') });
    await columns.nth(0).fill('2');
    await columns.nth(2).fill('3');
    await unitBoxes.nth(0).click();
    const options = async () => (await page.getByRole('option').allInnerTexts()).join('\n');
    const diameterOptions = await options();
    await page.screenshot({ path: path.join(out, `selector-${lang}-light.png`) });
    report.check(`${lang}: diameter selector offers compatible units only`, diameterOptions.includes('nm') && diameterOptions.includes(symbol) && !/cps/.test(diameterOptions), diameterOptions.replace(/\n/g, ' | '));
    await unitBoxes.nth(0).fill('Å-');
    await page.getByRole('option').filter({ hasText: symbol }).first().click();
    await unitBoxes.nth(2).click();
    const countOptions = await options();
    report.check(`${lang}: count-rate selector offers count-rate units`, countOptions.includes('kcps') && !countOptions.includes(symbol) && !/\bnm\b/.test(countOptions));
    await unitBoxes.nth(2).fill('kcps');
    await page.keyboard.press('Tab');
    report.check(`${lang}: exact symbol typed selects the unit`, (await unitBoxes.nth(2).inputValue()) === 'kcps' && (await unitBoxes.nth(0).inputValue()) === symbol);
    const mappingProblems = await layoutProblems(page, keys);
    report.check(`${lang}: mapping selector layout`, mappingProblems.length === 0, mappingProblems.join('; '));
    await page.getByRole('button', { name: l.inspect, exact: true }).click();
    await page.getByRole('button', { name: l.confirm, exact: true }).waitFor();
    await page.getByRole('button', { name: l.confirm, exact: true }).click();
    await page.getByRole('button', { name: l.done, exact: true }).waitFor();
    await page.getByRole('button', { name: l.done, exact: true }).click();
    const file = (await api(page, 'GET', '/api/files')).find((f) => f.name === filename);
    const q = file?.items?.[0]?.params?.effective_diameter;
    report.check(`${lang}: use custom unit in a mapping`, q?.unit === symbol && q?.unitOrigin === 'user_mapping' && q?.raw === '1.2300');

    // Settings → Units of measure.
    const openUnits = async () => {
      await page.evaluate(() => (history.pushState(null, '', '/'), dispatchEvent(new PopStateEvent('popstate'))));
      await page.locator('button.avatar-btn').first().click();
      await page.getByRole('menuitem').filter({ has: page.getByText(l.settings, { exact: true }) }).click();
      await page.locator('.modal nav button').filter({ hasText: tr(lang, 'settings.tab.units') }).click();
      await page.locator('[data-unit="nm"]').waitFor();
    };
    await openUnits();
    const row = (s) => page.locator(`[data-unit="${s}"]`);
    report.check(`${lang}: system and custom units are distinguished`, (await page.getByRole('heading', { name: tr(lang, 'units.system') }).count()) === 1 && (await page.getByRole('heading', { name: tr(lang, 'units.custom') }).count()) === 1 && (await row(symbol).innerText()).includes(tr(lang, 'units.badge.custom')) && (await row('nm').innerText()).includes(tr(lang, 'units.badge.system')));
    report.check(`${lang}: spelling variants stay separate rows`, (await row('µm').count()) === 1 && (await row('μm').count()) === 1 && (await row('um').count()) === 1);
    report.check(`${lang}: system units have no delete`, (await row('nm').getByRole('button', { name: tr(lang, 'units.delete') }).count()) === 0);
    report.check(`${lang}: usage count shown`, (await row(symbol).innerText()).includes(tr(lang, 'units.uses', { n: 2 })));
    await page.waitForTimeout(300);
    let problems = await layoutProblems(page, keys);
    report.check(`${lang}: manage units layout (light)`, problems.length === 0, problems.join('; '));
    await page.screenshot({ path: path.join(out, `units-${lang}-light.png`) });

    // Prevent destructive delete: three options, Cancel keeps everything.
    const inUseTitle = tr(lang, 'units.inuse.q', { symbol });
    await row(symbol).getByRole('button', { name: tr(lang, 'units.delete') }).click();
    const dialog = page.getByRole('dialog', { name: inUseTitle });
    await dialog.waitFor();
    report.check(`${lang}: used unit offers Archive / Replace Usage Then Delete / Cancel`, (await dialog.getByRole('button', { name: tr(lang, 'units.archive'), exact: true }).count()) === 1 && (await dialog.getByRole('button', { name: tr(lang, 'units.opt.replace'), exact: true }).count()) === 1 && (await dialog.getByRole('button', { name: l.cancel, exact: true }).count()) === 1);
    problems = await layoutProblems(page, keys);
    report.check(`${lang}: in-use dialog layout`, problems.length === 0, problems.join('; '));
    await page.screenshot({ path: path.join(out, `units-in-use-${lang}-light.png`) });
    await dialog.getByRole('button', { name: l.cancel, exact: true }).click();
    report.check(`${lang}: cancel keeps the unit`, (await row(symbol).count()) === 1);

    // Archive: hidden from the list and selectors, still on the data.
    await row(symbol).getByRole('button', { name: tr(lang, 'units.delete') }).click();
    await dialog.waitFor();
    await dialog.getByRole('button', { name: tr(lang, 'units.archive'), exact: true }).click();
    await dialog.waitFor({ state: 'detached' });
    await sleep(400);
    const offered = await scoped('GET', '/api/measurement-units?field=effective_diameter');
    const stillOnData = (await api(page, 'GET', '/api/files')).find((f) => f.name === filename)?.items?.[0]?.params?.effective_diameter?.unit;
    report.check(`${lang}: archive hides the unit and keeps historical data`, (await row(symbol).count()) === 0 && !offered.some((u) => u.symbol === symbol) && stillOnData === symbol);
    await page.locator('label.switch').filter({ hasText: tr(lang, 'units.show_archived') }).click();
    await row(symbol).waitFor();
    report.check(`${lang}: archived unit is listed on request`, (await row(symbol).innerText()).includes(tr(lang, 'units.badge.archived')));
    await row(symbol).getByRole('button', { name: tr(lang, 'units.restore') }).click();
    await sleep(500);
    report.check(`${lang}: restore`, !(await row(symbol).innerText()).includes(tr(lang, 'units.badge.archived')));
    await page.locator('label.switch').filter({ hasText: tr(lang, 'units.show_archived') }).click();

    // Replace usage then delete, with provenance.
    await row(symbol).getByRole('button', { name: tr(lang, 'units.delete') }).click();
    await dialog.waitFor();
    const nm = (await scoped('GET', '/api/measurement-units?q=nm')).find((u) => u.symbol === 'nm');
    await dialog.getByRole('combobox', { name: tr(lang, 'units.replace.with') }).selectOption(nm.id);
    await dialog.getByRole('button', { name: tr(lang, 'units.opt.replace'), exact: true }).click();
    await dialog.waitFor({ state: 'detached' });
    await sleep(400);
    const replaced = (await api(page, 'GET', '/api/files')).find((f) => f.name === filename)?.items?.[0]?.params?.effective_diameter;
    const provenance = await scoped('GET', '/api/measurement-units/replacements');
    report.check(`${lang}: replacement rewrites the assigned unit only, then deletes`, (await row(symbol).count()) === 0 && replaced?.unit === 'nm' && replaced?.raw === '1.2300' && replaced?.unitOrigin === 'user_mapping' && provenance.some((p) => p.unit.symbol === symbol && p.replacement.symbol === 'nm' && p.changes.length === 2));
    const original = await page.request.get(new URL(`/api/files/${file.id}/original?download=1`, page.url()).href);
    report.check(`${lang}: original bytes unchanged`, Buffer.compare(await original.body(), data) === 0);

    // Delete an unused custom unit.
    const unused = `unused-${lang}`;
    await scoped('POST', '/api/measurement-units', { symbol: unused, dimension: 'time' });
    await page.keyboard.press('Escape');
    await openUnits();
    await row(unused).getByRole('button', { name: tr(lang, 'units.delete') }).click();
    await page.getByRole('dialog', { name: tr(lang, 'units.delete.q', { symbol: unused }) }).getByRole('button', { name: tr(lang, 'units.delete'), exact: true }).click();
    await sleep(500);
    report.check(`${lang}: delete unused`, (await row(unused).count()) === 0 && !(await scoped('GET', '/api/measurement-units?archived=1')).some((u) => u.symbol === unused));

    // Dark theme, small window.
    await page.keyboard.press('Escape');
    await setPrefs('dark');
    await page.setViewportSize({ width: 1024, height: 600 });
    await page.reload();
    await page.waitForLoadState('networkidle');
    await openUnits();
    await sleep(300);
    problems = await layoutProblems(page, keys);
    report.check(`${lang}: manage units layout (dark, small window)`, problems.length === 0, problems.join('; '));
    await page.screenshot({ path: path.join(out, `units-${lang}-dark.png`) });
    await page.keyboard.press('Escape');
    await setPrefs('light');
    report.check(`${lang}: no script errors or failed requests`, errors.length === 0, errors.join('; '));
    await page.close();
  }
} catch (e) {
  report.check('units workflow completes', false, e.message.split('\n')[0]);
  fs.writeFileSync(path.join(out, 'failure.txt'), e.stack || String(e));
} finally {
  await browser.close();
}
process.exit(report.finish(out));
