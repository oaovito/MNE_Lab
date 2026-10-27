// Series Explorer: every series a graph can draw, searchable and filterable
// by file, condition, time, replicate kind and unit, in a windowed list that
// stays fast with dozens or hundreds of series. Selecting series only keeps
// their IDs in the graph; series left out stay in the library untouched.
import { useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import { t } from '../../lib/i18n';
import { applyFilter, emptyFilter, filterActive, replicateOf, seriesDetail, seriesName, unitOf, type ReplicateFilter, type SeriesFilter } from '../../lib/series';
import type { SeriesEntry } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Button, Check, Empty, Input, Select } from '../../ui/kit';

const ROW = 46; // px; fixed so only the visible rows are drawn
const OVERSCAN = 6;

type Props = {
  /** Series this graph type can draw (the current variable). */
  entries: SeriesEntry[];
  selected: string[];
  onSelect: (ids: string[]) => void;
  hidden: string[];
  onHidden: (id: string, hidden: boolean) => void;
  focus: string[];
  onFocus: (ids: string[]) => void;
  /** Colours of the drawn series, by SeriesID. */
  colors?: Record<string, string | undefined>;
};

export function SeriesExplorer(p: Props) {
  const [f, setF] = useState<SeriesFilter>(emptyFilter());
  const [only, setOnly] = useState(false);
  const [showFilters, setShowFilters] = useState(false);
  const anchor = useRef<string | null>(null);
  const sel = useMemo(() => new Set(p.selected), [p.selected]);
  const list = useMemo(() => {
    const l = applyFilter(p.entries, f);
    return only ? l.filter((s) => sel.has(s.id)) : l;
  }, [p.entries, f, only, sel]);

  const opts = useMemo(() => {
    const files = new Map<string, string>();
    const cond = new Set<string>(),
      time = new Set<string>(),
      unit = new Set<string>(),
      rep = new Set<string>();
    for (const s of p.entries) {
      files.set(s.fileId, s.fileName);
      if (s.condition) cond.add(s.condition);
      if (s.time?.label) time.add(s.time.label);
      unit.add(unitOf(s));
      rep.add(replicateOf(s));
    }
    return { files: [...files].sort((a, b) => a[1].localeCompare(b[1])), cond: [...cond].sort(), time: [...time], unit: [...unit].sort(), rep: [...rep] };
  }, [p.entries]);

  const set = (patch: Partial<SeriesFilter>) => setF({ ...f, ...patch });
  const toggle = (id: string, e?: MouseEvent | KeyboardEvent) => {
    if (e?.shiftKey && anchor.current) {
      // Range: everything listed between the last clicked row and this one.
      const a = list.findIndex((s) => s.id === anchor.current),
        b = list.findIndex((s) => s.id === id);
      if (a >= 0 && b >= 0) {
        const range = list.slice(Math.min(a, b), Math.max(a, b) + 1).map((s) => s.id);
        p.onSelect([...p.selected, ...range.filter((x) => !sel.has(x))]);
        anchor.current = id;
        return;
      }
    }
    anchor.current = id;
    p.onSelect(sel.has(id) ? p.selected.filter((x) => x !== id) : [...p.selected, id]);
  };
  const selectAll = () => p.onSelect([...p.selected, ...list.map((s) => s.id).filter((x) => !sel.has(x))]);
  const allOn = list.length > 0 && list.every((s) => sel.has(s.id));

  // Windowing: only the rows in view (plus a margin) are in the page.
  const scroller = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, h: 400 });
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setView({ top: el.scrollTop, h: el.clientHeight }));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const first = Math.max(0, Math.floor(view.top / ROW) - OVERSCAN);
  const last = Math.min(list.length, Math.ceil((view.top + view.h) / ROW) + OVERSCAN);

  const repLabel = (r: string) => t('sx.rep.' + r);
  return (
    <div class="sx">
      <div class="sx-head">
        <div class="row gap1">
          <div class="grow">
            <Input icon="search" size="sm" value={f.q} onValue={(q) => set({ q })} placeholder={t('sx.search')} aria-label={t('sx.search')} />
          </div>
          <Button size="sm" kind="ghost" icon="filter" selected={showFilters || filterActive(f)} tip={t('sx.filters')} onClick={() => setShowFilters(!showFilters)} class="sx-filter-btn" />
        </div>
        {showFilters && (
          <div class="sx-filters">
            <Select size="sm" label={t('sx.f.file')} value={f.file} onValue={(file) => set({ file })} options={[{ value: '', label: t('sx.f.all_files') }, ...opts.files.map(([v, l]) => ({ value: v, label: l }))]} />
            <Select size="sm" label={t('sx.f.condition')} value={f.condition} onValue={(condition) => set({ condition })} options={[{ value: '', label: t('sx.f.all_conditions') }, ...opts.cond.map((v) => ({ value: v, label: v }))]} disabled={!opts.cond.length} />
            <Select size="sm" label={t('sx.f.time')} value={f.time} onValue={(time) => set({ time })} options={[{ value: '', label: t('sx.f.all_times') }, ...opts.time.map((v) => ({ value: v, label: v }))]} disabled={!opts.time.length} />
            <Select size="sm" label={t('sx.f.replicate')} value={f.replicate} onValue={(replicate) => set({ replicate: replicate as ReplicateFilter })} options={[{ value: '', label: t('sx.f.all_replicates') }, ...opts.rep.map((v) => ({ value: v, label: repLabel(v) }))]} />
            <Select size="sm" label={t('sx.f.unit')} value={f.unit} onValue={(unit) => set({ unit })} options={[{ value: '', label: t('sx.f.all_units') }, ...opts.unit.map((v) => ({ value: v, label: v || t('sx.no_unit') }))]} />
            {filterActive(f) && (
              <Button size="sm" kind="ghost" icon="reset" onClick={() => setF({ ...emptyFilter(), q: f.q })}>
                {t('sx.f.clear')}
              </Button>
            )}
          </div>
        )}
        <div class="sx-bar">
          <Check checked={allOn} indeterminate={!allOn && list.some((s) => sel.has(s.id))} onChange={(on) => (on ? selectAll() : p.onSelect(p.selected.filter((x) => !list.some((s) => s.id === x))))} aria={t('sx.select_all')} />
          <span class="xs muted grow ellipsis sx-count">
            {t('sx.count', { n: p.selected.length, total: p.entries.length })}
            {(f.q || filterActive(f) || only) && ' · ' + t('sx.shown', { n: list.length })}
          </span>
          <Button size="sm" kind="ghost" class="sx-clear" onClick={() => p.onSelect([])} disabled={!p.selected.length}>
            {t('sx.clear')}
          </Button>
          <Button size="sm" kind="ghost" icon="listChecks" class="sx-only" selected={only} tip={t('sx.only_selected')} onClick={() => setOnly(!only)} />
        </div>
      </div>
      <div class="sx-list" ref={scroller} role="listbox" aria-multiselectable="true" aria-label={t('sx.title')} onScroll={(e) => setView({ top: (e.target as HTMLElement).scrollTop, h: (e.target as HTMLElement).clientHeight })}>
        {!p.entries.length ? (
          <Empty icon="layers" title={t('sx.empty')} body={t('sx.empty.d')} />
        ) : !list.length ? (
          <Empty icon="search" title={t('sx.none_match')} />
        ) : (
          <div style={{ height: list.length * ROW, position: 'relative' }}>
            {list.slice(first, last).map((s, k) => {
              const i = first + k;
              const on = sel.has(s.id);
              const hid = p.hidden.includes(s.id);
              const foc = p.focus.includes(s.id);
              const rep = replicateOf(s);
              return (
                <div
                  key={s.id}
                  class={`sx-row ${on ? 'on' : ''} ${on && hid ? 'off' : ''}`}
                  style={{ top: i * ROW, height: ROW }}
                  role="option"
                  aria-selected={on}
                  tabIndex={0}
                  data-series={s.id}
                  onClick={(e) => toggle(s.id, e)}
                  onKeyDown={(e) => (e.key === ' ' || e.key === 'Enter') && (e.preventDefault(), toggle(s.id, e))}
                >
                  <span onClick={(e) => e.stopPropagation()}>
                    <Check checked={on} onChange={() => toggle(s.id)} aria={seriesName(s)} />
                  </span>
                  {on && <span class="dot" style={{ color: p.colors?.[s.id] || 'var(--text-3)' }} />}
                  <span class="grow col sx-text">
                    <span class="ellipsis small" title={seriesName(s)}>
                      {seriesName(s)}
                    </span>
                    <span class="ellipsis xs faint" title={seriesDetail(s)}>
                      {[seriesDetail(s), unitOf(s) || t('sx.no_unit')].join(' · ')}
                    </span>
                  </span>
                  {(rep === 'technical' || rep === 'instrument_reading') && <span class={`sx-kind ${rep}`}>{repLabel(rep)}</span>}
                  {on && (
                    <span class="row sx-acts" onClick={(e) => e.stopPropagation()}>
                      <Button size="sm" kind="ghost" icon="dot" selected={foc} tip={foc ? t('sx.unfocus') : t('sx.focus')} onClick={() => p.onFocus(foc ? p.focus.filter((x) => x !== s.id) : [...p.focus, s.id])} class="sx-focus" />
                      <Button size="sm" kind="ghost" icon={hid ? 'eyeOff' : 'eye'} tip={hid ? t('graph.series.show') : t('graph.series.hide')} onClick={() => p.onHidden(s.id, !hid)} class="sx-eye" />
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
      {p.focus.length > 0 && (
        <div class="sx-foot row gap1">
          <Icon name="dot" size="sm" />
          <span class="xs grow">{t('sx.focus_on', { n: p.focus.length })}</span>
          <Button size="sm" kind="ghost" onClick={() => p.onFocus([])}>
            {t('sx.focus_end')}
          </Button>
        </div>
      )}
    </div>
  );
}
