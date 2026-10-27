// Searchable unit-of-measure selector: search, compatible-dimension filter
// (from the field), favourites and recently used units. Archived units are
// never offered, but a value that already uses one is still shown as is.
// Typing a registered compatible symbol exactly selects it; spelling
// variants are different units, so matching is case- and accent-exact.
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { get, post } from '../lib/api';
import { statisticsHeaders } from '../lib/analyses';
import { t } from '../lib/i18n';
import { Icon } from './icons';
import { Portal, useAsync } from './kit';
import { run } from '../lib/state';

export type MeasurementUnit = {
  id: string;
  symbol: string;
  name?: string;
  dimension: string;
  system: boolean;
  archived: boolean;
  archivedAt?: string;
  favorite: boolean;
  lastUsed?: string;
  created?: string;
  uses?: number;
};

/** The registry is bound to the open Account and Profile. */
/** codePoints spells non-ASCII symbols so look-alike variants (µm, μm) differ visibly. */
export const codePoints = (s: string) => (/[^\x20-\x7e]/.test(s) ? [...s].filter((c) => c.charCodeAt(0) > 0x7e).map((c) => 'U+' + c.codePointAt(0)!.toString(16).toUpperCase().padStart(4, '0')).join(' ') : '');

export const unitHeaders = (h?: Record<string, string>) => h || statisticsHeaders();

export function listUnits(query: Record<string, string>, headers?: Record<string, string>) {
  const qs = new URLSearchParams(query).toString();
  return get<MeasurementUnit[]>('/api/measurement-units' + (qs ? '?' + qs : ''), { headers: unitHeaders(headers) });
}

let seq = 0;

type Option = { kind: 'unit'; unit: MeasurementUnit } | { kind: 'none' } | { kind: 'create'; symbol: string };

export function UnitSelect(p: { value: string; onValue: (v: string) => void; field: string; label: string; disabled?: boolean; headers?: Record<string, string> }) {
  const [id] = useState(() => 'unitsel' + ++seq);
  const units = useAsync(() => listUnits({ field: p.field, archived: '1' }, p.headers), [p.field, p.headers?.['X-Profile-ID']]);
  const all = units.data || [];
  const offered = all.filter((u) => !u.archived);
  const [text, setText] = useState(p.value);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [pos, setPos] = useState<{ left: number; top: number; width: number; maxHeight: number } | null>(null);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!open) setText(p.value);
  }, [p.value, open]);

  const commit = (symbol: string) => {
    if (symbol !== p.value) {
      p.onValue(symbol);
      const u = offered.find((x) => x.symbol === symbol);
      if (u) post('/api/measurement-units/' + encodeURIComponent(u.id) + '/used', {}, { headers: unitHeaders(p.headers) }).catch(() => {});
    }
  };
  const searching = text !== '' && text !== p.value;
  const q = text.toLowerCase();
  const match = (u: MeasurementUnit) => u.symbol.toLowerCase().includes(q) || (u.name || '').toLowerCase().includes(q);
  const groups: { title?: string; options: Option[] }[] = [];
  if (searching) {
    const found = offered.filter(match).sort((a, b) => Number(b.symbol === text) - Number(a.symbol === text));
    const opts: Option[] = found.map((unit) => ({ kind: 'unit', unit }));
    if (!offered.some((u) => u.symbol === text) && !all.some((u) => u.symbol === text) && text.trim() === text && text.length <= 64) opts.push({ kind: 'create', symbol: text });
    groups.push({ options: opts });
  } else {
    const fav = offered.filter((u) => u.favorite);
    const recent = offered.filter((u) => u.lastUsed && !u.favorite).sort((a, b) => (b.lastUsed! > a.lastUsed! ? 1 : -1)).slice(0, 5);
    groups.push({ options: [{ kind: 'none' }] });
    if (fav.length) groups.push({ title: t('units.favorites'), options: fav.map((unit) => ({ kind: 'unit', unit })) });
    if (recent.length) groups.push({ title: t('units.recent'), options: recent.map((unit) => ({ kind: 'unit', unit })) });
    groups.push({ title: t('units.compatible'), options: offered.map((unit) => ({ kind: 'unit', unit })) });
  }
  const flat = groups.flatMap((g) => g.options);

  const place = () => {
    const r = input.current?.getBoundingClientRect();
    if (!r) return;
    const below = innerHeight - r.bottom - 8;
    const above = r.top - 8;
    const up = below < 180 && above > below;
    const maxHeight = Math.max(120, Math.min(300, up ? above : below));
    setPos({ left: Math.min(r.left, innerWidth - Math.max(r.width, 240) - 8), top: up ? r.top - maxHeight - 4 : r.bottom + 4, width: Math.max(r.width, 240), maxHeight });
  };
  useLayoutEffect(() => {
    if (!open) return;
    place();
    const h = () => place();
    addEventListener('resize', h);
    addEventListener('scroll', h, true);
    return () => (removeEventListener('resize', h), removeEventListener('scroll', h, true));
  }, [open]);

  const choose = async (o: Option) => {
    if (o.kind === 'none') commit('');
    else if (o.kind === 'unit') commit(o.unit.symbol);
    else {
      const u = await run(() => post<MeasurementUnit>('/api/measurement-units', { symbol: o.symbol, field: p.field }, { headers: unitHeaders(p.headers) }));
      if (!u) return;
      units.reload();
      p.onValue(u.symbol);
    }
    setOpen(false);
    input.current?.blur();
  };
  const onInput = (v: string) => {
    setText(v);
    setOpen(true);
    setActive(0);
    if (v === '') commit('');
    else if (offered.some((u) => u.symbol === v)) commit(v);
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setOpen(true);
      setActive((a) => (flat.length ? (a + (e.key === 'ArrowDown' ? 1 : flat.length - 1)) % flat.length : 0));
    } else if (e.key === 'Enter' && open && flat[active]) {
      e.preventDefault();
      choose(flat[active]);
    } else if (e.key === 'Escape' && open) {
      e.stopPropagation();
      setOpen(false);
      setText(p.value);
    }
  };
  const current = p.value ? all.find((u) => u.symbol === p.value) : undefined;
  const note = !p.value || units.loading ? '' : !current ? t('units.not_registered') : current.archived ? t('units.archived_value') : '';
  let n = -1;
  return (
    <div class="col" style={{ gap: 2 }}>
      <input
        ref={input}
        class="input"
        role="combobox"
        aria-label={p.label}
        aria-expanded={open}
        aria-controls={id}
        aria-autocomplete="list"
        aria-activedescendant={open && flat[active] ? `${id}-${active}` : undefined}
        value={text}
        maxLength={64}
        disabled={p.disabled}
        spellcheck={false}
        autocomplete="off"
        onFocus={() => setOpen(true)}
        onBlur={() => (setOpen(false), setText(p.value))}
        onInput={(e) => onInput((e.target as HTMLInputElement).value)}
        onKeyDown={onKey}
      />
      {note && <span class="xs faint">{p.value} · {note}</span>}
      {open && pos && (
        <Portal>
        <div id={id} class="menu" role="listbox" aria-label={p.label} style={{ position: 'fixed', left: pos.left, top: pos.top, width: pos.width, maxWidth: pos.width, maxHeight: pos.maxHeight, overflow: 'auto' }} onMouseDown={(e) => e.preventDefault()}>
          {groups.map((g) => (
            <>
              {g.title && <div class="menu-label">{g.title}</div>}
              {g.options.map((o) => {
                const i = ++n;
                const selected = o.kind === 'unit' ? o.unit.symbol === p.value : o.kind === 'none' && !p.value;
                return (
                  <div id={`${id}-${i}`} role="option" aria-selected={selected} class="menu-item" style={i === active ? { background: 'var(--surface-hover)' } : undefined} onMouseEnter={() => setActive(i)} onClick={() => choose(o)}>
                    {o.kind === 'unit' && (
                      <>
                        <Icon name={o.unit.favorite ? 'star' : 'ruler'} size="sm" />
                        <b style={{ fontFamily: 'var(--mono)' }}>{o.unit.symbol}</b>
                        <span class="meta">{[o.unit.name || t(o.unit.system ? 'units.badge.system' : 'units.badge.custom'), codePoints(o.unit.symbol)].filter(Boolean).join(' · ')}</span>
                      </>
                    )}
                    {o.kind === 'none' && <span class="muted">{t('units.no_unit')}</span>}
                    {o.kind === 'create' && (
                      <>
                        <Icon name="plus" size="sm" />
                        <span>{t('units.create_inline', { symbol: o.symbol })}</span>
                      </>
                    )}
                  </div>
                );
              })}
            </>
          ))}
          {searching && !flat.length && <div class="menu-label">{t('units.no_match')}</div>}
        </div>
        </Portal>
      )}
    </div>
  );
}
