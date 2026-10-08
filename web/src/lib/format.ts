import { lang } from './i18n';

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

export type Quantity = { value: number; raw: string; unit?: string; label?: string; line?: number };

export function fmtQ(q: Quantity | undefined, withUnit = true): string {
  if (!q) return '—';
  const n = fmtNum(q.value, decimalsOf(q.raw));
  return withUnit && q.unit ? `${n} ${q.unit}` : n;
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
