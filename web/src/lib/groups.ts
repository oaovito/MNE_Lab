// Groups and saved selections, experimental designs for graphs, Combine
// and Graph → Analyze. Groups store references only; the core checks
// every combination and never mixes variables, units or X domains.
import { api, get, post } from './api';
import { statisticsHeaders } from './analyses';
import { useRev } from './library';
import type { CombineCheck, DesignView, GraphDef, GraphKind, Group } from './types';
import { useAsync } from '../ui/kit';

/** Requests that name the open Account and Profile. */
export const scoped = () => ({ headers: statisticsHeaders() });

export function useGroups() {
  const rev = useRev();
  return useAsync(() => get<Group[]>('/api/groups', scoped()).then((x) => x || []), [rev]);
}

export function useDesigns() {
  const rev = useRev();
  return useAsync(() => get<DesignView[]>('/api/designs', scoped()).then((x) => x || []), [rev]);
}

export const saveGroup = (g: Group) => post<Group>('/api/groups', g, scoped());
export const deleteGroup = (id: string) => api('DELETE', '/api/groups/' + id, undefined, scoped());

export const DESIGN_KINDS: GraphKind[] = ['design_time', 'paired_condition'];
export const isDesignKind = (k: GraphKind) => DESIGN_KINDS.includes(k);

/** Kinds whose saved graphs can be combined (same kind, variable and X domain). */
export const COMBINABLE: GraphKind[] = ['design_time', 'series_values', 'dls_distribution'];

export const checkCombine = (ids: string[], displayUnit = '') => post<CombineCheck>('/api/graphs/combine/check', { ids, displayUnit }, scoped());
export const combineGraphs = (ids: string[], title: string, displayUnit = '') =>
  post<{ graph: GraphDef; check: CombineCheck }>('/api/graphs/combine', { ids, title, displayUnit }, scoped());

/**
 * analyzeRoute opens the statistics editor prefilled from what a graph
 * draws: the design (units, conditions, time) and its variable, or the
 * cycle. The person reviews the structure there; nothing is rebuilt.
 */
export function analyzeRoute(def: GraphDef): string | null {
  if (!def.id) return null;
  const v = encodeURIComponent(def.param || 'effective_diameter');
  const only = (def.conditions || []).map((c) => '&conditions=' + encodeURIComponent(c)).join('') + (def.groups?.length ? '&group=' + def.groups.join(',') : '');
  if (isDesignKind(def.kind) && def.designId) return `/ls/statistics/new?design=${def.designId}&variable=${v}${only}&graph=${def.id}`;
  if (def.kind === 'parameter_time' && def.cycleId) return `/ls/statistics/new?cycle=${def.cycleId}&variable=${v}&graph=${def.id}`;
  if (def.kind === 'statistical_groups' && def.analysisId) return '/ls/statistics/' + def.analysisId;
  return null;
}

/** Text of a combine problem or note (stable identifiers of the core). */
export const combineKey = (code: string) => 'gc.' + code.replace(/^graph\.combine_/, '');
