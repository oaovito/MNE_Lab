// LIGHTSCATTERING library data, reloaded whenever the core reports a change.
import { get } from './api';
import { t } from './i18n';
import { app } from './state';
import { useStore } from './store';
import type { CycleDoc, FileView, GraphDef, MeasurementSummary, Relations } from './types';
import { useAsync } from '../ui/kit';

export const useRev = () => useStore(app, (s) => s.libraryRev);

export function useFiles(trash = false) {
  const rev = useRev();
  return useAsync(() => get<FileView[] | null>('/api/files' + (trash ? '?trash=1' : '')).then((x) => x || []), [rev, trash]);
}
export function useGraphs(trash = false) {
  const rev = useRev();
  return useAsync(() => get<GraphDef[] | null>('/api/graphs' + (trash ? '?trash=1' : '')).then((x) => x || []), [rev, trash]);
}
export function useCycles(trash = false) {
  const rev = useRev();
  return useAsync(() => get<CycleDoc[] | null>('/api/cycles' + (trash ? '?trash=1' : '')).then((x) => x || []), [rev, trash]);
}
export function useRelations() {
  const rev = useRev();
  return useAsync(() => get<Relations>('/api/relations'), [rev]);
}

export const PARAMS = ['effective_diameter', 'polydispersity', 'count_rate', 'baseline_index'] as const;
export const WEIGHTINGS = ['intensity', 'volume', 'number'] as const;
export const UNITS = ['hours', 'days', 'weeks', 'months', 'years'] as const;

/** measurementName identifies a measurement the way people recognize it. */
export function measurementName(m: MeasurementSummary, f?: { name: string; measurements?: string[] | null }) {
  if (m.label) return m.label;
  const base = m.sampleId || f?.name || t('ls.measurement');
  return f && (f.measurements?.length || 0) > 1 ? `${base} · ${t('ls.run', { n: m.index + 1 })}` : base;
}

/** warnText turns "code:arg:line" parser and engine warnings into text. */
export function warnText(w: string): string {
  const [code, ...rest] = w.split(':');
  const args: Record<string, string> = {};
  if (code === 'ls.unit_missing') {
    args.field = t('param.' + rest[0]) !== 'param.' + rest[0] ? t('param.' + rest[0]) : rest[0];
    args.n = rest[1];
  } else if (code.startsWith('graph.skipped_')) args.n = rest.join(':'); // a measurement's name
  else if (rest.length) args.n = rest[rest.length - 1];
  const key = 'warn.' + code;
  const s = t(key, args);
  return s === key ? code : s;
}

export function allMeasurements(files: FileView[]) {
  const out = new Map<string, { m: MeasurementSummary; f: FileView }>();
  for (const f of files) for (const m of f.items || []) out.set(m.id, { m, f });
  return out;
}
