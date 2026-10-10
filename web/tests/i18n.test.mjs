// Translation coverage: every key the interface can ask for exists in English,
// Portuguese (Brasil) and Spanish, with the same placeholders, and every
// stable error or warning identifier of the core has a message.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync, mkdtempSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { transformSync } from 'esbuild';

const web = join(dirname(fileURLToPath(import.meta.url)), '..');
const root = join(web, '..');
const src = join(web, 'src');

function walk(dir, ext, out = []) {
  for (const n of readdirSync(dir)) {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) walk(p, ext, out);
    else if (ext.some((e) => n.endsWith(e))) out.push(p);
  }
  return out;
}

async function loadCatalogs() {
  const tmp = mkdtempSync(join(tmpdir(), 'mnelab-i18n-'));
  const all = {};
  for (const name of ['core', 'ls', 'export', 'help', 'errors']) {
    const code = transformSync(readFileSync(join(src, 'i18n', name + '.ts'), 'utf8'), { loader: 'ts', format: 'esm' }).code;
    const f = join(tmp, name + '.mjs');
    writeFileSync(f, code);
    const mod = await import(pathToFileURL(f).href);
    for (const table of Object.values(mod)) {
      for (const [k, v] of Object.entries(table)) {
        assert.ok(!(k in all), `duplicate key ${k}`);
        all[k] = v;
      }
    }
  }
  const go = ['en', 'pt-BR', 'es'].map((l) => JSON.parse(readFileSync(join(root, 'internal/i18n/locales', l + '.json'), 'utf8')));
  for (const k of Object.keys(go[0])) all[k] ??= [go[0][k], go[1][k] || go[0][k], go[2][k] || go[0][k]];
  return all;
}

const catalog = await loadCatalogs();
test('every statistical engine diagnostic has a translated label', () => {
  const engine=readFileSync(join(src,'lib/statistics-engine.R'),'utf8');
  const codes=[...engine.matchAll(/mne_diagnostic\("([a-z_]+)"/g)].map(m=>m[1]);
  assert.ok(codes.includes('dunnett_confidence_integration'));
  for(const code of codes)assert.ok(catalog['stat.diagnostic.'+code],`untranslated engine diagnostic: ${code}`);
});
const uiFiles = walk(src, ['.ts', '.tsx']).filter((f) => !f.includes(join('src', 'i18n')));
const ui = uiFiles.map((f) => readFileSync(f, 'utf8')).join('\n');
const goFiles = walk(join(root, 'internal'), ['.go']).filter((f) => !f.endsWith('_test.go'));
const go = goFiles.map((f) => readFileSync(f, 'utf8')).join('\n');

const has = (k) => k in catalog;
const missing = (keys) => [...new Set(keys)].filter((k) => !has(k)).sort();
const list = (re, text) => [...text.matchAll(re)].map((m) => m[1]);
const arr = (name, text = ui) => {
  const m = text.match(new RegExp(name + String.raw`[^=]*=\s*\[([^\]]*)\]`));
  assert.ok(m, `array ${name} not found`);
  return list(/'([^']+)'/g, m[1]);
};

test('every entry has three languages and matching placeholders', () => {
  const ph = (s) => [...new Set(list(/\{(\w+)[}:]/g, s))].sort().join(',');
  for (const [k, row] of Object.entries(catalog)) {
    assert.equal(row.length, 3, k);
    row.forEach((s, i) => assert.ok(typeof s === 'string' && s.trim(), `${k} is empty in language ${i}`));
    assert.equal(ph(row[1]), ph(row[0]), `${k}: Portuguese placeholders differ`);
    assert.equal(ph(row[2]), ph(row[0]), `${k}: Spanish placeholders differ`);
  }
});

test('static keys used by the interface exist', () => {
  const keys = [
    ...list(/\bt\(\s*'([^'\\]+)'\s*[,)]/g, ui),
    ...list(/\b(?:key|label):\s*'([a-z][a-z0-9_]*\.[a-z0-9_.]+)'/g, ui),
    // keys chosen by a condition, such as t(temp ? 'a.b' : 'a.c')
    ...[...ui.matchAll(/\bt\(([^()]*\?[^()]*)\)/g)].flatMap((m) => list(/'([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)'/g, m[1])),
  ];
  assert.deepEqual(missing(keys), []);
});

test('slides, tips and news have their descriptions', () => {
  const keys = list(/\bkey:\s*'((?:tour|lsi|tip|news\.0)\.[a-z0-9_]+)'/g, ui)
    .filter((k) => !k.startsWith('tour.p.'))
    .map((k) => k + '.d');
  assert.deepEqual(missing(keys), []);
});

test('dynamic key families are complete', () => {
  const features = list(/\{\s*id:\s*'([a-z_]+)',\s*group:/g, readFileSync(join(src, 'lib/features.ts'), 'utf8'));
  const groups = arr('GROUPS', readFileSync(join(src, 'lib/features.ts'), 'utf8'));
  const helpSrc = readFileSync(join(src, 'screens/help.tsx'), 'utf8');
  const guides = [...helpSrc.matchAll(/\{\s*id:\s*'(\w+)',\s*icon:\s*'\w+',\s*parts:\s*\[([^\]]*)\]/g)].map((m) => ({ id: m[1], parts: list(/'(\w+)'/g, m[2]) }));
  assert.ok(features.length > 30 && guides.length >= 2);
  const formats = list(/^\s*"([a-z]+)":\s*\{ID:/gm, readFileSync(join(root, 'internal/export/formats.go'), 'utf8'));
  const modes = ['usb_cloud', 'usb_only', 'cloud_only'];
  const units = arr('UNITS', readFileSync(join(src, 'lib/library.ts'), 'utf8'));
  const params = arr('PARAMS', readFileSync(join(src, 'lib/library.ts'), 'utf8'));
  const weightings = arr('WEIGHTINGS', readFileSync(join(src, 'lib/library.ts'), 'utf8'));
  const profileGo = readFileSync(join(root, 'internal/app/profile.go'), 'utf8');
  const syncStates = list(/\bSync[A-Z]\w*\s*=\s*"([a-z_]+)"/g, profileGo);
  const collections = list(/\bColl[A-Z]\w*\s*=\s*"([a-z_.]+)"/g, go);
  const reasons = list(/"(cycle\.(?:ambiguous_date|between_points|excluded|experiment_mismatch|missing_measurement_date|outside_all_points|sample_mismatch))"/g, go);
  const kinds = list(/Kind[A-Z]\w*\s*=\s*"([a-z]+)"/g, readFileSync(join(root, 'internal/export/formats.go'), 'utf8'));
  const presets = list(/Preset[A-Z]\w*\s*=\s*"([a-z]+)"/g, readFileSync(join(root, 'internal/export/formats.go'), 'utf8'));
  const sections = list(/\{"([a-z]+)",\s*\[\]string/g, readFileSync(join(root, 'internal/export/formats.go'), 'utf8'));
  const hints = list(/(?:Hint|Warning):\s*"([a-z_.]+)"/g, readFileSync(join(root, 'internal/export/formats.go'), 'utf8'));
  const activities = list(/\.begin\("([a-z_]+)"\)/g, go);
  const graphKinds = list(/Kind[A-Z]\w*\s*=\s*"((?:dls|parameter)_[a-z_]+)"/g, go);
  const exitSteps = arr('ORDER', readFileSync(join(src, 'screens/overlays.tsx'), 'utf8')).filter((k) => k !== 'done');
  const exitWarn = [...new Set(list(/a\.step\(&?r, "([a-z]+)", "?(?:warning|state)/g, go))];
  const science = list(/"([a-z]+)":/g, readFileSync(join(root, 'internal/app/api.go'), 'utf8').match(/Science:\s*map\[string\]string\{([^}]*)\}/)[1]);
  assert.ok(syncStates.length >= 8 && collections.length >= 7 && reasons.length >= 7 && activities.length >= 4);

  const keys = [
    ...features.flatMap((f) => ['', '.what', '.when', '.how'].map((s) => 'feat.' + f + s)),
    ...groups.map((g) => 'fgroup.' + g),
    ...guides.flatMap((g) => ['guide.' + g.id, 'guide.' + g.id + '.intro', ...g.parts.flatMap((p) => ['guide.' + g.id + '.' + p, 'guide.' + g.id + '.' + p + '.d'])]),
    ...formats.flatMap((f) => ['fmt.' + f, 'fmt.' + f + '.d']),
    ...hints,
    ...kinds.map((k) => 'export.kind.' + k),
    ...presets.flatMap((p) => ['export.preset.' + p, 'export.preset.' + p + '.d']),
    ...sections.map((s) => 'export.sec.' + s),
    ...modes.flatMap((m) => ['storage.' + m, 'storage.' + m + '.desc', 'stor.mode.d.' + m]),
    ...units.flatMap((u) => ['unit.' + u, 'point.' + u]),
    ...params.flatMap((p) => ['param.' + p, 'param.short.' + p]),
    ...weightings.map((w) => 'axis.' + w),
    ...syncStates.map((s) => 'sync.' + s),
    ...collections.filter((c) => !c.startsWith('_')).map((c) => 'stor.coll.' + c),
    ...reasons.map((r) => 'reason.' + r),
    ...activities.map((a) => 'activity.' + a),
    ...graphKinds.map((k) => 'graph.kind.' + k),
    ...exitSteps.map((s) => 'exit.step.' + s),
    ...exitWarn.map((s) => 'exit.warn.' + s),
    ...science.map((s) => 'about.science.' + s),
    ...['auto', 'confirmed', 'needs_confirmation', 'unassigned', 'missing'].map((s) => 'cycle.status.' + s),
    ...['manual', 'periodic', 'exit', 'restart', 'before-restore'].map((s) => 'bk.reason.' + s),
    ...['default', 'desktop', 'documents', 'downloads', 'drive'].map((s) => 'dest.' + s),
    ...['parsed', 'partial', 'failed'].map((s) => 'file.status.' + s),
    ...['parsed', 'partial', 'failed', 'duplicate'].map((s) => 'imp.status.' + s),
    ...['tab', 'semicolon', 'comma', 'whitespace'].map((s) => 'files.delim.' + s),
    ...['import', 'review', 'graph', 'cycle', 'present', 'export'].flatMap((s) => ['home.flow.' + s, 'home.flow.' + s + '.d']),
    ...['files', 'measurements', 'graphs', 'cycles'].map((s) => 'home.stat.' + s),
    ...['portable', 'temporary'].flatMap((m) => ['mode.' + m, 'mode.' + m + '.tip']),
    ...['action', 'page', 'feature', 'file', 'graph', 'cycle'].map((s) => 'pal.kind.' + s),
    ...['no_server', 'encrypted', 'no_telemetry', 'your_cloud', 'exports', 'mobile', 'updates', 'temporary', 'portable'].flatMap((s) => ['priv.' + s, 'priv.' + s + '.d']),
    ...['google', 'icloud', 'onedrive'].map((s) => 'prov.desc.' + s),
    ...['system', 'light', 'dark'].map((s) => 'set.theme.' + s),
    ...['general', 'profile', 'account', 'storage', 'backups', 'updates', 'mobile', 'privacy', 'about'].map((s) => 'settings.tab.' + s),
    ...['account', 'storage', 'portable', 'mobile', 'ls', 'exit'].map((s) => 'help.start.' + s),
    'upd.channel.stable',
    'notice.mobile.device_connected',
  ];
  assert.deepEqual(missing(keys), []);
});

test('every error and warning identifier of the core has a message', () => {
  const errs = list(/errors\.New\("([a-z_]+\.[a-z_.]+)"\)/g, go);
  const warns = list(/Warnings\s*=\s*append\([^,]+,\s*(?:fmt\.Sprintf\()?"([a-z_]+\.[a-z_]+)/g, go);
  const client = list(/(?:LinkError|ApiError)\('([a-z_]+\.[a-z_]+)'/g, ui).concat(list(/let code = '([a-z_.]+)'/g, ui));
  assert.ok(errs.length > 80 && warns.length > 8);
  assert.deepEqual(missing([...errs, ...client].map((c) => 'err.' + c)), []);
  assert.deepEqual(missing(warns.map((w) => 'warn.' + w)), []);
  const areas = [...new Set([...errs, ...client].map((c) => c.split('.')[0]))];
  assert.deepEqual(missing(areas.map((a) => 'err.' + a)), []);
});

test('every key the core translates exists in all three languages', () => {
  const goLocales = ['en', 'pt-BR', 'es'].map((l) => JSON.parse(readFileSync(join(root, 'internal/i18n/locales', l + '.json'), 'utf8')));
  const used = list(/\bT\("([a-z_.]+[a-z_])"/g, go);
  for (const loc of goLocales) assert.deepEqual(used.filter((k) => !(k in loc)), []);
  for (const k of Object.keys(goLocales[0])) for (const loc of goLocales) assert.ok(loc[k], k);
});

test('the phone loads every text it shows', async () => {
  // Files reachable from the phone entry point, following relative imports.
  const seen = new Set();
  const visit = (file) => {
    if (seen.has(file)) return;
    seen.add(file);
    for (const spec of list(/^import[^'"]*['"](\.[^'"]+)['"]/gm, readFileSync(file, 'utf8'))) {
      const base = join(dirname(file), spec);
      const hit = [base, base + '.ts', base + '.tsx', join(base, 'index.ts')].find((p) => { try { return statSync(p).isFile(); } catch { return false; } });
      if (hit && /\.tsx?$/.test(hit) && !hit.includes(join('src', 'i18n'))) visit(hit);
    }
  };
  visit(join(src, 'mobile/main.tsx'));
  const text = [...seen].map((f) => readFileSync(f, 'utf8')).join('\n');
  const phoneTables = readFileSync(join(src, 'i18n/phone.ts'), 'utf8');
  const loaded = {};
  for (const name of list(/from '\.\/(\w+)'/g, phoneTables)) {
    const code = transformSync(readFileSync(join(src, 'i18n', name + '.ts'), 'utf8'), { loader: 'ts', format: 'esm' }).code;
    const f = join(mkdtempSync(join(tmpdir(), 'mnelab-i18n-')), name + '.mjs');
    writeFileSync(f, code);
    for (const table of Object.values(await import(pathToFileURL(f).href))) Object.assign(loaded, table);
  }
  const goEn = JSON.parse(readFileSync(join(root, 'internal/i18n/locales/en.json'), 'utf8'));
  const keys = [...list(/\bt\(\s*'([^'\\]+)'\s*[,)]/g, text), ...[...text.matchAll(/\bt\(([^()]*\?[^()]*)\)/g)].flatMap((m) => list(/'([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)'/g, m[1]))];
  const prefixes = list(/\bt\(\s*'([a-z_.]+\.)'\s*\+/g, text);
  assert.deepEqual([...new Set(keys)].filter((k) => !(k in loaded) && !(k in goEn)).sort(), []);
  for (const p of prefixes) assert.ok(Object.keys(loaded).concat(Object.keys(goEn)).some((k) => k.startsWith(p)), `no phone texts for ${p}*`);
});
