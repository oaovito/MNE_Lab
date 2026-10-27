// The series a graph draws, as SeriesIDs. Pure functions only (no imports
// beyond types), so they can be tested without a browser.
import type { GraphDef, SeriesEntry } from './types';

// pathEscape matches Go's url.PathEscape, which the core uses to build
// SeriesIDs: unreserved characters and $&+:=@ stay, the rest is %XX.
function pathEscape(s: string): string {
  let out = '';
  for (const b of new TextEncoder().encode(s)) {
    const c = String.fromCharCode(b);
    if (/[A-Za-z0-9\-_.~$&+:=@]/.test(c)) out += c;
    else out += '%' + b.toString(16).toUpperCase().padStart(2, '0');
  }
  return out;
}

/** curveId is the core's SeriesID of one weighting of one distribution candidate. */
export const curveId = (measurementId: string, distributionId: string, weighting: string) => 'm:' + pathEscape(measurementId) + '/d:' + pathEscape(distributionId) + '/w:' + pathEscape(weighting);

// The candidate a schema 1 graph draws for a measurement: the pinned one,
// or the one the library marks as selected for it in any weighting.
function candidateOf(def: GraphDef, m: string, catalog: SeriesEntry[]): string | undefined {
  return def.distributions?.[m] || catalog.find((s) => s.kind === 'curve' && s.measurementId === m && s.distribution?.selected)?.distribution?.id;
}

/**
 * selectionOf returns the SeriesIDs a graph draws: its own selection, or for
 * a graph saved before selection by SeriesID, the curves of its
 * measurements (pinned candidate and weighting). A measurement the library
 * does not resolve now (in the trash, read again, or without this weighting)
 * is kept as a reference, shown as missing, and never dropped.
 */
export function selectionOf(def: GraphDef, catalog: SeriesEntry[]): string[] {
  if (def.seriesIds?.length) return def.seriesIds;
  if (def.kind !== 'dls_distribution') return [];
  const w = def.weighting || 'intensity';
  const out: string[] = [];
  for (const m of def.measurements) {
    const pin = def.distributions?.[m];
    const hit = catalog.find((s) => s.kind === 'curve' && s.measurementId === m && s.y.key === w && (pin ? s.distribution?.id === pin : s.distribution?.selected || s.distribution?.id === 'table'));
    if (hit) {
      out.push(hit.id);
      continue;
    }
    const dist = candidateOf(def, m, catalog);
    if (dist) out.push(curveId(m, dist, w));
  }
  return out;
}

/**
 * unnamedReferences lists the measurements of a schema 1 graph that cannot
 * be written as SeriesIDs: not in the library now and with no pinned
 * candidate, so which curve the graph meant is not known here. Converting
 * the graph to a SeriesID selection would lose them, so the Builder keeps
 * the graph as it is unless the person chooses otherwise.
 */
export function unnamedReferences(def: GraphDef, catalog: SeriesEntry[]): string[] {
  if (def.seriesIds?.length || def.kind !== 'dls_distribution') return [];
  return def.measurements.filter((m) => !candidateOf(def, m, catalog));
}
