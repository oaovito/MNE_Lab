// The series catalog on the interface side: every plottable series of the
// library with its stable SeriesID. Choosing series for a graph only keeps
// their IDs; the other series and every record stay as they are.
import { get } from './api';
import { statisticsHeaders } from './analyses';
import { t } from './i18n';
import { useRev } from './library';
import type { GraphKind, SeriesCatalog, SeriesEntry } from './types';
import { useAsync } from '../ui/kit';

export function useSeriesCatalog() {
  const rev = useRev();
  return useAsync(() => get<SeriesCatalog>('/api/series', { headers: statisticsHeaders() }), [rev]);
}

/** Graph kinds whose data is chosen in the Series Explorer. */
export const SELECTION_KINDS: GraphKind[] = ['dls_distribution', 'series_values'];
export const bySelection = (k: GraphKind) => SELECTION_KINDS.includes(k);

/** accepts says whether a graph kind can draw a series. */
export function accepts(kind: GraphKind, s: SeriesEntry): boolean {
  if (kind === 'dls_distribution') return s.kind === 'curve';
  if (kind === 'series_values') return s.kind === 'scalar' || s.kind === 'reading';
  return false;
}

/** The variable a series measures: a weighting for curves, a parameter otherwise. */
export const variableOf = (s: SeriesEntry) => s.y.key;

/** seriesName identifies a series the way people recognize it. */
export function seriesName(s: SeriesEntry): string {
  const base = s.label || s.sample || s.fileName;
  return s.reading ? t('series.reading', { name: base, run: s.reading.run }) : base;
}

/** seriesDetail is the secondary line: file and organization. */
export function seriesDetail(s: SeriesEntry): string {
  return [s.fileName, s.experimentalUnit, s.condition, s.time?.label].filter(Boolean).join(' · ');
}

export const unitOf = (s: SeriesEntry) => s.y.unit || '';

/** Replicate kinds as the explorer filters them. */
export type ReplicateFilter = '' | 'experimental' | 'technical' | 'instrument_reading' | 'unassigned';
export const replicateOf = (s: SeriesEntry): Exclude<ReplicateFilter, ''> =>
  s.kind === 'reading' || s.replicateKind === 'instrument_reading' ? 'instrument_reading' : s.replicateKind === 'technical' ? 'technical' : s.replicateKind === 'experimental' ? 'experimental' : 'unassigned';

export type SeriesFilter = { q: string; file: string; condition: string; time: string; replicate: ReplicateFilter; unit: string };
export const emptyFilter = (): SeriesFilter => ({ q: '', file: '', condition: '', time: '', replicate: '', unit: '' });
export const filterActive = (f: SeriesFilter) => !!(f.file || f.condition || f.time || f.replicate || f.unit);

export function applyFilter(list: SeriesEntry[], f: SeriesFilter): SeriesEntry[] {
  const words = f.q.toLowerCase().split(/\s+/).filter(Boolean);
  return list.filter((s) => {
    if (f.file && s.fileId !== f.file) return false;
    if (f.condition && (s.condition || '') !== f.condition) return false;
    if (f.time && (s.time?.label || '') !== f.time) return false;
    if (f.replicate && replicateOf(s) !== f.replicate) return false;
    if (f.unit && unitOf(s) !== f.unit) return false;
    if (!words.length) return true;
    const hay = [seriesName(s), s.fileName, s.sample, s.label, s.experimentalUnit, s.condition, s.time?.label, unitOf(s), s.reading?.run].filter(Boolean).join(' ').toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

export { selectionOf, unnamedReferences } from './selection';

/** withVariable maps a selection to another weighting or parameter of the same data. */
export function withVariable(ids: string[], key: string, catalog: SeriesEntry[]): string[] {
  const byId = new Map(catalog.map((s) => [s.id, s]));
  const out: string[] = [];
  for (const id of ids) {
    const s = byId.get(id);
    if (!s) continue;
    const twin = catalog.find((c) => c.kind === s.kind && c.measurementId === s.measurementId && c.y.key === key && c.distribution?.id === s.distribution?.id && c.reading?.index === s.reading?.index);
    if (twin && !out.includes(twin.id)) out.push(twin.id);
  }
  return out;
}

/** Variables offered for a graph kind, from what the library holds. */
export function variablesFor(kind: GraphKind, catalog: SeriesEntry[]): string[] {
  const seen: string[] = [];
  for (const s of catalog) if (accepts(kind, s) && !seen.includes(s.y.key)) seen.push(s.y.key);
  return seen;
}

/** variableLabel names a weighting or parameter key. */
export const variableLabel = (key: string) => (['intensity', 'volume', 'number'].includes(key) ? t('axis.' + key) : t('param.' + key) !== 'param.' + key ? t('param.' + key) : key);
