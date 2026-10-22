import { lang, t } from './i18n';
import type { CycleView } from './types';

let decimal = '.';
export const setDecimal = (d: string) => (decimal = d === ',' ? ',' : '.');
export const getDecimal = () => decimal;

/** Number with the chosen decimal separator and a true minus sign. */
export function fmtNum(v: number | null | undefined, digits?: number): string {
  if (v === null || v === undefined || !isFinite(v)) return '—';
  let s: string;
  if (digits === undefined) {
    s = String(+v.toPrecision(10));
    if (s.includes('e')) s = v.toExponential(3);
  } else s = v.toFixed(digits);
  if (s.startsWith('-')) s = '−' + s.slice(1);
  return decimal === ',' ? s.replace('.', ',') : s;
}

/** Decimals written in the original text (never invent precision). */
export function decimalsOf(raw: string | undefined): number | undefined {
  if (!raw) return undefined;
  const m = raw.match(/[.,](\d+)\s*$/) || raw.match(/[.,](\d+)/);
  return m ? m[1].length : 0;
}

export type Quantity = { value: number; raw: string; unit?: string; label?: string; line?: number; unitOrigin?: string; unitLine?: number; sourceColumn?: number };

/**
 * Parameters that carry a physical unit. Polydispersity and baseline index
 * are dimensionless: they have no unit to state, so nothing is shown.
 */
export const UNIT_PARAMS: ReadonlySet<string> = new Set(['effective_diameter', 'count_rate', 'average_count_rate']);
export const needsUnit = (key: string | undefined) => !!key && UNIT_PARAMS.has(key);

/**
 * unitText is the unit to print after a value of parameter key: the unit the
 * data carries, or a short marker when the file does not state one for a
 * quantity that has a unit. Dimensionless and unknown keys print nothing.
 */
export function unitText(unit: string | undefined, key?: string): string {
  if (unit) return unit;
  return needsUnit(key) ? t('qty.unit_not_stated') : '';
}

/** squared writes a unit squared (variances, sums of squares). */
export function squared(unit: string): string {
  if (!unit) return '';
  return /^[\p{L}µμ°]+$/u.test(unit) ? unit + '²' : '(' + unit + ')²';
}

/**
 * fmtQ writes a quantity as its value with the decimals of the file and its
 * unit. With the parameter key, a missing unit on a quantity that has one is
 * marked instead of silently dropped.
 */
export function fmtQ(q: Quantity | undefined, key?: string, withUnit = true): string {
  if (!q) return '—';
  const n = fmtNum(q.value, decimalsOf(q.raw));
  if (!withUnit) return n;
  const u = q.unit || (needsUnit(key) ? `(${t('qty.unit_not_stated')})` : '');
  return u ? `${n} ${u}` : n;
}

/**
 * The unit shared by the quantities of one parameter: 'unit' when every one
 * states the same unit, 'none' when the parameter is dimensionless or no
 * value has a unit and none is needed, 'not_stated' when none states it,
 * and 'mixed' when they disagree or only some state it.
 */
export type CommonUnit = { kind: 'unit'; unit: string } | { kind: 'none' } | { kind: 'not_stated' } | { kind: 'mixed' };
export function commonUnit(qs: (Quantity | undefined)[], key: string): CommonUnit {
  const units = new Set<string>();
  for (const q of qs) if (q) units.add(q.unit || '');
  if (units.size > 1) return { kind: 'mixed' };
  const [u] = [...units];
  if (u) return { kind: 'unit', unit: u };
  return needsUnit(key) ? { kind: 'not_stated' } : { kind: 'none' };
}

/**
 * cycleUnit is the unit to print after the per-point summaries of a cycle.
 * The core states it per parameter (units, mixedUnits); for a view without
 * them it is worked out from the measurements the summaries use.
 */
export function cycleUnit(v: Pick<CycleView, 'units' | 'mixedUnits' | 'series' | 'items'>, param: string): CommonUnit {
  if (v.mixedUnits?.includes(param)) return { kind: 'mixed' };
  if (v.units && param in v.units) {
    const u = v.units[param];
    if (u) return { kind: 'unit', unit: u };
    return needsUnit(param) ? { kind: 'not_stated' } : { kind: 'none' };
  }
  const used = new Set((v.series?.[param] || []).flatMap((r) => r.sources || []));
  const items = (v.items || []).filter((m) => !used.size || used.has(m.id));
  return commonUnit(items.map((m) => m.params[param]), param);
}

/** unitNote is the sentence shown when a common unit cannot be printed. */
export function unitNote(c: CommonUnit): string {
  return c.kind === 'not_stated' ? t('qty.unit_not_stated.d') : c.kind === 'mixed' ? t('qty.mixed_units.d') : '';
}

export type Timestamp = { time: string; raw: string; tzKnown: boolean; ambiguous?: boolean; confirmed?: boolean; source: string };

/** A measurement time: without a known time zone it is shown as written. */
export function fmtTS(ts: Timestamp | undefined | null, withTime = true): string {
  if (!ts) return '—';
  const d = new Date(ts.time);
  const o: Intl.DateTimeFormatOptions = withTime
    ? { dateStyle: 'medium', timeStyle: 'short' }
    : { dateStyle: 'medium' };
  if (!ts.tzKnown) o.timeZone = 'UTC';
  return new Intl.DateTimeFormat(lang(), o).format(d);
}

export function fmtDate(iso: string | undefined, withTime = false): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(+d) || d.getFullYear() < 2000) return '—';
  return new Intl.DateTimeFormat(lang(), withTime ? { dateStyle: 'medium', timeStyle: 'short' } : { dateStyle: 'medium' }).format(d);
}

export function fmtRel(iso: string | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  const s = (Date.now() - d.getTime()) / 1000;
  const rtf = new Intl.RelativeTimeFormat(lang(), { numeric: 'auto' });
  if (Math.abs(s) < 45) return rtf.format(0, 'second');
  if (Math.abs(s) < 3600) return rtf.format(-Math.round(s / 60), 'minute');
  if (Math.abs(s) < 86400) return rtf.format(-Math.round(s / 3600), 'hour');
  if (Math.abs(s) < 86400 * 30) return rtf.format(-Math.round(s / 86400), 'day');
  return fmtDate(iso);
}

export function fmtBytes(n: number): string {
  const u = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024;
    i++;
  }
  return `${fmtNum(n, i === 0 ? 0 : n < 10 ? 1 : 0)} ${u[i]}`;
}

/** toLocalInput formats a date for <input type="datetime-local">. */
export function toLocalInput(iso: string | undefined, utc = false): string {
  const d = iso ? new Date(iso) : new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  if (utc) return `${d.getUTCFullYear()}-${p(d.getUTCMonth() + 1)}-${p(d.getUTCDate())}T${p(d.getUTCHours())}:${p(d.getUTCMinutes())}`;
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
