// A graph saved before selection by SeriesID (schema 1) keeps every
// measurement it refers to when the Builder reads it: a measurement the
// library does not resolve now is kept as a reference and shown as missing,
// never dropped. The IDs and values are invented.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { buildSync } from 'esbuild';

const web = join(dirname(fileURLToPath(import.meta.url)), '..');
const out = join(mkdtempSync(join(tmpdir(), 'mnelab-selection-')), 'selection.mjs');
buildSync({ entryPoints: [join(web, 'src/lib/selection.ts')], bundle: true, format: 'esm', platform: 'neutral', outfile: out, logLevel: 'silent' });
const s = await import(pathToFileURL(out).href);

const curve = (m, d, w, selected) => ({ id: s.curveId(m, d, w), kind: 'curve', measurementId: m, y: { key: w }, distribution: { id: d, selected } });

test('curve IDs are written as the core writes them (Go url.PathEscape)', () => {
  assert.equal(s.curveId('m-01', 'table', 'intensity'), 'm:m-01/d:table/w:intensity');
  // Reference strings printed by Go's url.PathEscape for the same inputs.
  assert.equal(s.curveId('a b/ü;,?$&+:=@~%#', 'cand 2', 'volume'), 'm:a%20b%2F%C3%BC%3B%2C%3F$&+:=@~%25%23/d:cand%202/w:volume');
});

test('a schema 1 graph keeps the measurements the library does not resolve now', () => {
  const catalog = [
    curve('m1', 'c1', 'intensity', true),
    // m2 is in the library but has no intensity curve now: its selected candidate is known.
    curve('m2', 'c7', 'number', true),
    // m3 (pinned) and m4 (not pinned) are not in the library at all.
  ];
  const def = { kind: 'dls_distribution', weighting: 'intensity', measurements: ['m1', 'm2', 'm3', 'm4'], distributions: { m3: 'p3' } };
  const sel = s.selectionOf(def, catalog);
  assert.deepEqual(sel, ['m:m1/d:c1/w:intensity', 'm:m2/d:c7/w:intensity', 'm:m3/d:p3/w:intensity']);
  // m4 cannot be named as a series: the Builder keeps the graph as it is and says so.
  assert.deepEqual(s.unnamedReferences(def, catalog), ['m4']);
  // A selection by SeriesID is the graph's own and is returned unchanged.
  assert.deepEqual(s.selectionOf({ ...def, seriesIds: ['m:x/d:y/w:intensity'] }, catalog), ['m:x/d:y/w:intensity']);
  assert.deepEqual(s.unnamedReferences({ ...def, seriesIds: ['m:x/d:y/w:intensity'] }, catalog), []);
});
