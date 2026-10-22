// Value formatting: units are shown only as the data carries them, and a
// quantity that has a unit but whose file does not state it is marked
// instead of shown as a bare number.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { buildSync } from 'esbuild';

const web = join(dirname(fileURLToPath(import.meta.url)), '..');
const out = join(mkdtempSync(join(tmpdir(), 'mnelab-format-')), 'format.mjs');
buildSync({
  stdin: {
    contents: `export * from './src/lib/format'; export { addCatalogs, setLang } from './src/lib/i18n'; export { core } from './src/i18n/core';`,
    resolveDir: web,
    loader: 'ts',
  },
  bundle: true,
  format: 'esm',
  platform: 'neutral',
  outfile: out,
  logLevel: 'silent',
});
globalThis.document = { documentElement: {} };
const f = await import(pathToFileURL(out).href);
f.addCatalogs(f.core);

const ed = (unit, extra = {}) => ({ value: 60.4, raw: '60.40', unit, ...extra });

test('fmtQ keeps the decimals of the file and the stated unit', () => {
  f.setLang('en');
  f.setDecimal('.');
  assert.equal(f.fmtQ(ed('nm'), 'effective_diameter'), '60.40 nm');
  assert.equal(f.fmtQ(ed('nm')), '60.40 nm');
  assert.equal(f.fmtQ(ed('nm'), 'effective_diameter', false), '60.40');
  assert.equal(f.fmtQ(undefined, 'effective_diameter'), '—');
});

test('fmtQ marks a missing unit only for quantities that have one', () => {
  f.setLang('en');
  for (const k of ['effective_diameter', 'count_rate', 'average_count_rate']) assert.equal(f.fmtQ(ed(''), k), '60.40 (unit not stated)', k);
  for (const k of ['polydispersity', 'baseline_index']) assert.equal(f.fmtQ(ed(undefined), k), '60.40', k);
  assert.equal(f.fmtQ(ed(undefined)), '60.40', 'no key: nothing is assumed');
  f.setLang('pt-BR');
  f.setDecimal(',');
  assert.equal(f.fmtQ(ed(undefined), 'effective_diameter'), '60,40 (unidade não informada)');
  f.setLang('es');
  assert.equal(f.fmtQ(ed(undefined), 'count_rate'), '60,40 (unidad no indicada)');
  f.setLang('en');
  f.setDecimal('.');
});

test('unitText and squared never invent a unit', () => {
  assert.equal(f.unitText('nm', 'effective_diameter'), 'nm');
  assert.equal(f.unitText('', 'effective_diameter'), 'unit not stated');
  assert.equal(f.unitText('', 'polydispersity'), '');
  assert.equal(f.unitText(undefined, undefined), '');
  assert.equal(f.squared('nm'), 'nm²');
  assert.equal(f.squared('kcps'), 'kcps²');
  assert.equal(f.squared('µm'), 'µm²');
  assert.equal(f.squared('counts/s'), '(counts/s)²');
  assert.equal(f.squared(''), '');
});

test('commonUnit prints one unit only when every value states the same one', () => {
  assert.deepEqual(f.commonUnit([ed('nm'), ed('nm')], 'effective_diameter'), { kind: 'unit', unit: 'nm' });
  assert.deepEqual(f.commonUnit([ed('nm'), ed('µm')], 'effective_diameter'), { kind: 'mixed' });
  assert.deepEqual(f.commonUnit([ed('nm'), ed('')], 'effective_diameter'), { kind: 'mixed' });
  assert.deepEqual(f.commonUnit([ed(''), undefined, ed(undefined)], 'effective_diameter'), { kind: 'not_stated' });
  assert.deepEqual(f.commonUnit([ed(''), ed('')], 'polydispersity'), { kind: 'none' });
  assert.equal(f.unitNote({ kind: 'unit', unit: 'nm' }), '');
  assert.equal(f.unitNote({ kind: 'none' }), '');
  assert.match(f.unitNote({ kind: 'not_stated' }), /does not state the unit/);
  assert.match(f.unitNote({ kind: 'mixed' }), /different units/);
});

test('cycleUnit follows the units the core states per parameter', () => {
  const items = [
    { id: 'a', params: { effective_diameter: ed('nm'), polydispersity: ed('') } },
    { id: 'b', params: { effective_diameter: ed('') } },
  ];
  const view = { units: { effective_diameter: 'nm', count_rate: '', polydispersity: '' }, mixedUnits: ['average_count_rate'], series: {}, items };
  assert.deepEqual(f.cycleUnit(view, 'effective_diameter'), { kind: 'unit', unit: 'nm' });
  assert.deepEqual(f.cycleUnit(view, 'count_rate'), { kind: 'not_stated' });
  assert.deepEqual(f.cycleUnit(view, 'polydispersity'), { kind: 'none' });
  assert.deepEqual(f.cycleUnit(view, 'average_count_rate'), { kind: 'mixed' });
  // Without the core's statement, the measurements the summaries use decide.
  const older = { series: { effective_diameter: [{ sources: ['a'] }] }, items };
  assert.deepEqual(f.cycleUnit(older, 'effective_diameter'), { kind: 'unit', unit: 'nm' });
  assert.deepEqual(f.cycleUnit({ series: {}, items }, 'effective_diameter'), { kind: 'mixed' });
  assert.deepEqual(f.cycleUnit({ series: null, items: null }, 'count_rate'), { kind: 'not_stated' });
});
