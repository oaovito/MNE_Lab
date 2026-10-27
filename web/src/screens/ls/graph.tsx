// Graph Builder: the Series Explorer on the left, the live preview in the
// centre and the graph settings on the right, with Save, Duplicate,
// Combine, Analyze, Present and Export below. The core computes and lays
// out the figure; changing the display never changes the data, and choosing
// series only keeps their IDs.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { get, post } from '../../lib/api';
import { statisticsHeaders } from '../../lib/analyses';
import { fmtDate, fmtNum, fmtQ, fmtTS, unitText } from '../../lib/format';
import { errText, has, t } from '../../lib/i18n';
import { PARAMS, useCycles, useRelations, warnText, WEIGHTINGS } from '../../lib/library';
import { navigate } from '../../lib/route';
import { accepts, bySelection, selectionOf, seriesDetail, seriesName, useSeriesCatalog, variableLabel, variablesFor, withVariable } from '../../lib/series';
import { openPanel, run, toast } from '../../lib/state';
import type { GraphDef, GraphKind, LineStyle, MarkerShape, Series, SeriesEntry, SeriesStyle, Visual } from '../../lib/types';
import type { StatisticalAnalysis } from '../../lib/statistics';
import { FigureView, seriesLabel } from '../../plot/Figure';
import { defaultVisual, useRendered, useSize, type Rendered } from '../../plot/useRender';
import { Icon } from '../../ui/icons';
import { Badge, Button, confirmDialog, Empty, Field, Input, Notice, Seg, Select, Skeleton, Switch, Tabs, useAsync } from '../../ui/kit';
import { presentGraphs } from './graphs';
import { SeriesExplorer } from './series-explorer';

function newDef(q: URLSearchParams): GraphDef {
  const cycleId = q.get('cycle') || undefined;
  const kind = (q.get('kind') as GraphKind) || (cycleId ? 'parameter_time' : 'dls_distribution');
  return {
    title: '',
    kind,
    measurements: (q.get('m') || '').split(',').filter(Boolean),
    weighting: 'intensity',
    xScale: 'auto',
    cycleId,
    param: q.get('param') || 'effective_diameter',
    series: {},
    visual: defaultVisual(),
    tags: [],
    notes: '',
  };
}

/** timeUnit localizes the time unit of a cycle axis (days, weeks…). */
const timeUnit = (u: string) => (u && has('unit.' + u) ? t('unit.' + u) : u);

const norm = (d: GraphDef | null | undefined) => JSON.stringify(d ? { ...d, created: undefined, updated: undefined, schema: undefined, sourceFiles: undefined, parserVersions: undefined, calculationVersion: undefined } : null);

/** Units shown on request, by dimension, with exact factors only (decision D6; the core converts). */
const DISPLAY_UNITS = [
  ['nm', 'µm', 'm'],
  ['cps', 'kcps', 'Mcps'],
  ['s', 'min', 'h', 'd'],
];
const MARKERS: MarkerShape[] = ['circle', 'square', 'triangle', 'diamond', 'triangle_down', 'cross'];
const LINES: LineStyle[] = ['solid', 'dashed', 'dotted', 'dashdot', 'longdash', 'dashdotdot'];

/** Display presets: they only set how the graph looks. */
const PRESETS: Record<string, Partial<Visual>> = {
  screen: { legend: true, metadata: true, grid: true, lineWidth: 1.75, fontScale: 1, margin: 0 },
  presentation: { legend: true, metadata: false, grid: true, lineWidth: 2.5, fontScale: 1.3, margin: 0 },
  publication: { legend: true, metadata: false, grid: false, lineWidth: 1.25, fontScale: 1, margin: 4 },
  print: { legend: true, metadata: true, grid: true, lineWidth: 1.5, fontScale: 1, margin: 6 },
};

export function GraphView(p: { id: string }) {
  const isNew = p.id === 'new';
  const saved = useAsync(() => (isNew ? Promise.resolve(null) : get<GraphDef>('/api/graphs/' + p.id)), [p.id]);
  const [def, setDef] = useState<GraphDef | null>(null);
  const [base, setBase] = useState<GraphDef | null>(null);
  useEffect(() => {
    if (isNew) {
      const d = newDef(new URLSearchParams(location.search));
      setDef(d);
      setBase(null);
    } else if (saved.data) {
      setDef(saved.data);
      setBase(saved.data);
    }
  }, [p.id, saved.data]);

  if (!isNew && saved.error)
    return (
      <div class="panel" style={{ flex: 1 }}>
        <Empty icon="chart" title={t('graph.not_found')} body={errText(saved.error)}>
          <Button onClick={() => navigate('/ls/graphs')}>{t('graph.back')}</Button>
        </Empty>
      </div>
    );
  if (!def) return <Skeleton h="100%" r="var(--r-lg)" />;
  return <Builder key={p.id} def={def} setDef={setDef} base={base} setBase={setBase} isNew={isNew} />;
}

type LeftTab = 'series' | 'data' | 'groups';
type RightTab = 'settings' | 'style' | 'details';

function Builder(p: { def: GraphDef; setDef: (d: GraphDef) => void; base: GraphDef | null; setBase: (d: GraphDef) => void; isNew: boolean }) {
  const def = p.def;
  const set = (patch: Partial<GraphDef>) => p.setDef({ ...def, ...patch });
  const setVisual = (patch: Partial<Visual>) => set({ visual: { ...def.visual, ...patch } });
  const [ref, size] = useSize<HTMLDivElement>();
  const r = useRendered(def, size.w, size.h);
  const [busy, setBusy] = useState(false);
  const [left, setLeft] = useState<LeftTab>('series');
  const [right, setRight] = useState<RightTab>('settings');
  const [insp, setInsp] = useState(false);
  const [slow, setSlow] = useState(false);
  const dirty = p.isNew || norm(def) !== norm(p.base);
  const cycles = useCycles();
  const cycle = cycles.data?.find((c) => c.id === def.cycleId);
  const catalog = useSeriesCatalog();
  const all = catalog.data?.series || [];
  const analysis = useAsync(() => (def.analysisId ? get<StatisticalAnalysis>('/api/statistics/' + def.analysisId, { headers: statisticsHeaders() }) : Promise.resolve(null)), [def.analysisId]);
  const series = r.data?.series || [];
  const selectable = bySelection(def.kind);

  // The progress note appears only when a preview takes a noticeable time.
  useEffect(() => {
    if (r.stage !== 'computing') return setSlow(false);
    const tm = setTimeout(() => setSlow(true), 450);
    return () => clearTimeout(tm);
  }, [r.stage, r.since]);

  // Leaving with unsaved changes asks first.
  useEffect(() => {
    const h = (e: BeforeUnloadEvent) => {
      if (dirty) e.preventDefault();
    };
    addEventListener('beforeunload', h);
    return () => removeEventListener('beforeunload', h);
  }, [dirty]);

  // ---- selection (SeriesIDs) ----
  const variable = def.kind === 'dls_distribution' ? def.weighting || 'intensity' : def.param || 'effective_diameter';
  const selected = useMemo(() => (selectable ? selectionOf(def, all) : []), [def.seriesIds, def.measurements, def.distributions, def.weighting, def.kind, all]);
  const entries = useMemo(() => all.filter((s) => accepts(def.kind, s) && s.y.key === variable), [all, def.kind, variable]);
  const styleKey = (id: string) => def.series?.[id] || def.series?.[all.find((s) => s.id === id)?.measurementId || ''] || {};
  const select = (ids: string[]) => set({ seriesIds: ids, measurements: [...new Set(ids.map((id) => all.find((s) => s.id === id)?.measurementId).filter(Boolean) as string[])], focusSeries: (def.focusSeries || []).filter((x) => ids.includes(x)) });
  const setStyle = (id: string, st: Partial<SeriesStyle>) => {
    const next: SeriesStyle = { ...(def.series?.[id] || (selectable ? styleKey(id) : {})), ...st };
    for (const k of Object.keys(next) as (keyof SeriesStyle)[]) if (!next[k]) delete next[k];
    // Editing a series of a graph saved before SeriesIDs keeps its selection explicit.
    const extra = selectable && !def.seriesIds?.length ? { seriesIds: selected } : {};
    set({ ...extra, series: { ...(def.series || {}), [id]: next } });
  };
  const setVariable = (key: string) => {
    const ids = withVariable(selected, key, all);
    if (def.kind === 'dls_distribution') set({ weighting: key, seriesIds: ids.length || selected.length ? ids : undefined });
    else set({ param: key, seriesIds: ids });
  };
  const setKind = (kind: GraphKind) => {
    if (kind === def.kind) return;
    if (bySelection(kind) && bySelection(def.kind)) {
      // Curves and single values are different series: start a new selection.
      const v = variablesFor(kind, all)[0];
      set({ kind, seriesIds: [], measurements: [], focusSeries: [], ...(kind === 'series_values' ? { param: v || 'effective_diameter' } : { weighting: 'intensity' }) });
      if (selected.length) toast('info', t('gb.kind_reset'));
      return;
    }
    set({ kind });
  };

  // ---- actions ----
  const save = async () => {
    setBusy(true);
    const g = await run(() => post<GraphDef>('/api/graphs', def));
    setBusy(false);
    if (!g) return;
    toast('success', t('graph.saved'));
    p.setBase(g);
    p.setDef(g);
    if (p.isNew) navigate('/ls/graphs/' + g.id, true);
  };
  const back = async () => {
    if (dirty && !(await confirmDialog({ title: t('graph.discard.q'), body: t('graph.discard.d'), confirm: t('graph.discard'), danger: true }))) return;
    navigate(def.cycleId && p.isNew ? '/ls/cycles/' + def.cycleId : '/ls/graphs');
  };
  const needSave = async () => {
    if (!dirty && def.id) return def.id;
    const ok = await confirmDialog({ title: t('graph.save_first.q'), body: t('graph.save_first.d'), confirm: t('ui.save') });
    if (!ok) return null;
    setBusy(true);
    const g = await run(() => post<GraphDef>('/api/graphs', def));
    setBusy(false);
    if (!g) return null;
    p.setBase(g);
    p.setDef(g);
    if (p.isNew) navigate('/ls/graphs/' + g.id, true);
    return g.id!;
  };
  const duplicate = async () => {
    const id = await needSave();
    if (!id) return;
    const g = await run(() => post<GraphDef>(`/api/graphs/${id}/duplicate`, { title: t('graph.copy_of', { name: def.title || t('graph.kind.' + def.kind) }) }));
    if (!g) return;
    toast('success', t('gb.duplicated'));
    navigate('/ls/graphs/' + g.id);
  };

  const colors = useMemo(() => Object.fromEntries(series.map((s) => [s.id, s.color])), [series]);
  const hidden = Object.entries(def.series || {}).filter(([, v]) => v.hidden).map(([k]) => k);
  const errBars = def.kind === 'parameter_time' ? def.errorBars || 'sd' : def.kind === 'statistical_groups' ? def.errorBars || 'sd' : undefined;
  return (
    <>
      <div class="page-head" style={{ minHeight: 0 }}>
        <Button kind="ghost" size="sm" icon="back" onClick={back}>
          {t('ls.tab.graphs')}
        </Button>
        <div class="grow" style={{ maxWidth: 520 }}>
          <Input value={def.title} onValue={(title) => set({ title })} placeholder={t('graph.title.ph')} aria-label={t('graph.title')} maxLength={160} style={{ fontWeight: 600, fontSize: 'var(--fs-lg)', background: 'transparent', borderColor: 'transparent' }} />
        </div>
        {dirty && !p.isNew && <Badge kind="warning">{t('graph.unsaved')}</Badge>}
        {!!def.derivedFrom?.length && (
          <Badge kind="info" icon="history" tip={t('gb.derived_from', { name: def.derivedFrom[0].title || '—' })}>
            {t('gb.copy')}
          </Badge>
        )}
        <span class="spacer" />
        <Button size="sm" kind="ghost" icon="sliders" class="narrow-only" selected={insp} onClick={() => setInsp(!insp)}>
          {t('gb.settings')}
        </Button>
      </div>
      <div class="builder">
        {/* left: Series Explorer / Data / Groups */}
        <div class="panel builder-left">
          <Tabs
            value={left}
            onValue={setLeft}
            tabs={[
              { value: 'series', label: t('sx.title'), count: selectable ? selected.length : undefined },
              { value: 'data', label: t('gb.t.data') },
              { value: 'groups', label: t('gb.t.groups') },
            ]}
          />
          {left === 'series' ? (
            selectable ? (
              catalog.loading && !catalog.data ? (
                <div class="panel-body pad">
                  <Skeleton h={200} />
                </div>
              ) : (
                <SeriesExplorer
                  entries={entries}
                  selected={selected}
                  onSelect={select}
                  hidden={hidden}
                  onHidden={(id, h) => setStyle(id, { hidden: h })}
                  focus={def.focusSeries || []}
                  onFocus={(ids) => set({ focusSeries: ids, ...(def.seriesIds?.length ? {} : { seriesIds: selected }) })}
                  colors={colors}
                />
              )
            ) : (
              <div class="panel-body pad col gap3">
                <Notice kind="info">{def.kind === 'statistical_groups' ? t('gb.data_from_analysis') : t('gb.data_from_cycle')}</Notice>
                <DataPanel def={def} cycleName={cycle?.config.name} cycleN={cycle?.measurements.length} selected={[]} all={all} onRemove={() => {}} />
              </div>
            )
          ) : left === 'data' ? (
            <div class="panel-body pad">
              <DataPanel def={def} cycleName={cycle?.config.name} cycleN={cycle?.measurements.length} selected={selected} all={all} onRemove={(id) => select(selected.filter((x) => x !== id))} />
            </div>
          ) : (
            <div class="panel-body pad">
              <Empty icon="layers" title={t('gb.groups.title')} body={t('gb.groups.d')} />
            </div>
          )}
        </div>

        {/* centre: live preview */}
        <div class="col gap2 builder-centre">
          <div class="figure-wrap" ref={ref}>
            {r.data && size.w > 0 && (
              <FigureView
                figure={r.data.figure}
                series={series}
                xUnit={def.kind === 'parameter_time' ? timeUnit(r.data.x.unit) : r.data.x.unit}
                yUnit={unitText(r.data.y.unit, def.kind === 'parameter_time' || def.kind === 'series_values' ? def.param : undefined)}
                errStat={errBars}
                interactive
                onZoom={(view) => setVisual({ view })}
                onReset={() => setVisual({ view: null })}
              />
            )}
            {r.data && (
              <div class="fig-tools">
                {def.visual.view ? (
                  <Button size="sm" icon="reset" onClick={() => setVisual({ view: null })}>
                    {t('graph.reset_zoom')}
                  </Button>
                ) : (
                  <span class="xs" style={{ color: '#59606b', padding: '6px 4px' }}>
                    <Icon name="zoom" size="sm" /> {t('graph.zoom_hint')}
                  </span>
                )}
              </div>
            )}
            {!r.data && !r.error && r.stage !== 'idle' && (
              <div class="state">
                <Spinnerish />
              </div>
            )}
            {!r.data && !r.error && r.stage === 'idle' && selectable && (
              <div class="state">
                <Empty icon="layers" title={t('gb.nothing')} body={t('gb.nothing.d')} />
              </div>
            )}
            {r.error && (
              <div class="state">
                <Empty icon="chart" title={selectable && !selected.length ? t('gb.nothing') : t('graph.cannot')} body={selectable && !selected.length ? t('gb.nothing.d') : errText(r.error)} />
              </div>
            )}
            {r.loading && r.data && !slow && (
              <span class="busy">
                <span class="spin" style={{ width: 14, height: 14 }} />
              </span>
            )}
            {slow && r.stage === 'computing' && (
              <div class="fig-progress" role="status">
                <span class="bar" />
                <span class="xs">{t('gb.computing')}</span>
                <Button size="sm" kind="ghost" onClick={r.cancel}>
                  {t('gb.stop')}
                </Button>
              </div>
            )}
          </div>
          {!!r.data?.warnings?.length && (
            <Notice kind="warning">
              <div class="warn-list">
                {[...new Set(r.data.warnings.map(warnText))].map((w) => (
                  <div>{w}</div>
                ))}
              </div>
            </Notice>
          )}
        </div>

        {/* right: configuration */}
        <div class={`panel inspector builder-right ${insp ? 'open' : ''}`}>
          <Tabs
            value={right}
            onValue={setRight}
            tabs={[
              { value: 'settings', label: t('gb.t.settings') },
              { value: 'style', label: t('graph.t.series') },
              { value: 'details', label: t('graph.details') },
            ]}
          />
          <div class="panel-body" style={{ padding: 'var(--s3)' }}>
            {right === 'settings' ? (
              <Settings def={def} set={set} setVisual={setVisual} setKind={setKind} setVariable={setVariable} r={r.data} all={all} cycles={cycles.data || []} analysis={analysis.data || null} />
            ) : right === 'style' ? (
              !r.data ? <Skeleton h={80} /> : <StylePanel series={series} def={def} setStyle={setStyle} setVisual={setVisual} />
            ) : (
              <DetailsPanel r={r.data} def={def} set={set} />
            )}
          </div>
        </div>

        {/* actions */}
        <div class="builder-bar">
          <span class="xs muted grow ellipsis">{r.stage === 'computing' ? t('gb.computing') : r.data ? t('gb.drawn', { n: series.filter((s) => !s.hidden).length }) : ''}</span>
          <Button size="sm" kind="primary" icon="save" class="gb-save" busy={busy} disabled={!dirty || !!r.error} onClick={save}>
            {p.isNew ? t('graph.save_library') : t('ui.save')}
          </Button>
          <Button size="sm" icon="copy" class="gb-duplicate" disabled={busy || (p.isNew && !r.data)} tip={t('gb.duplicate.d')} onClick={duplicate}>
            {t('gb.duplicate')}
          </Button>
          <Button size="sm" icon="layers" class="gb-combine" disabled tip={t('gb.combine.d')}>
            {t('gb.combine')}
          </Button>
          <Button size="sm" icon="sigma" class="gb-analyze" disabled tip={t('gb.analyze.d')}>
            {t('gb.analyze')}
          </Button>
          <Button size="sm" icon="present" class="graph-present" disabled={!r.data} onClick={async () => { const id = await needSave(); if (id) presentGraphs([id]); }}>
            {t('pres.start')}
          </Button>
          <Button size="sm" icon="download" class="graph-export" disabled={!r.data} onClick={async () => { const id = await needSave(); if (id) openPanel('export', { items: [{ kind: 'graph', id }] }); }}>
            {t('ui.export')}
          </Button>
        </div>
      </div>
    </>
  );
}

const Spinnerish = () => (
  <span class="row gap2 small muted">
    <span class="spin" style={{ width: 16, height: 16 }} />
    {t('graph.rendering')}
  </span>
);

function Section(p: { title: string; children: any; open?: boolean; class?: string }) {
  return (
    <details class={`gb-sec ${p.class || ''}`} open={p.open ?? true}>
      <summary class="section-title">{p.title}</summary>
      <div class="group">{p.children}</div>
    </details>
  );
}

function Settings(p: {
  def: GraphDef;
  set: (patch: Partial<GraphDef>) => void;
  setVisual: (patch: Partial<Visual>) => void;
  setKind: (k: GraphKind) => void;
  setVariable: (key: string) => void;
  r: Rendered | null;
  all: SeriesEntry[];
  cycles: { id?: string; config: { name: string } }[];
  analysis: StatisticalAnalysis | null;
}) {
  const { def, set, setVisual, r } = p;
  const cycleKind = def.kind === 'parameter_time' || def.kind === 'dls_by_time';
  const stored = r?.y.conversion?.from || r?.y.unit || '';
  const unitGroup = DISPLAY_UNITS.find((g) => g.includes(stored));
  const values = variablesFor('series_values', p.all);
  const [noteText, setNoteText] = useState('');
  const notes = def.textNotes || [];
  const addNote = () => {
    const text = noteText.trim();
    if (!text || !r) return;
    const x = r.x.categories?.length ? (r.x.categories.length + 1) / 2 : r.x.scale === 'log' && r.x.min > 0 ? Math.sqrt(r.x.min * r.x.max) : (r.x.min + r.x.max) / 2;
    set({ textNotes: [...notes, { text, x, y: r.y.min + (r.y.max - r.y.min) * 0.85 }] });
    setNoteText('');
  };
  return (
    <div class="col gap3 gb-settings">
      <Section title={t('gb.s.data')}>
        <Field label={t('graph.type')}>
          <Select
            value={def.kind}
            label={t('graph.type')}
            onValue={(k) => p.setKind(k as GraphKind)}
            options={
              def.analysisId
                ? [{ value: 'statistical_groups', label: t('graph.kind.statistical_groups') }]
                : [
                    { value: 'dls_distribution', label: t('graph.kind.dls_distribution') },
                    { value: 'series_values', label: t('graph.kind.series_values'), disabled: !values.length },
                    { value: 'parameter_time', label: t('graph.kind.parameter_time'), disabled: !def.cycleId },
                    { value: 'dls_by_time', label: t('graph.kind.dls_by_time'), disabled: !def.cycleId },
                  ]
            }
          />
        </Field>
        {!def.cycleId && !def.analysisId && <span class="xs faint">{t('graph.cycle_kinds_hint')}</span>}
        {cycleKind && (
          <Field label={t('meta.cycle')}>
            <Select value={def.cycleId || ''} onValue={(cycleId) => set({ cycleId })} options={p.cycles.map((c) => ({ value: c.id!, label: c.config.name }))} />
          </Field>
        )}
        <div class="gb-axes">
          <span class="xs muted">{t('gb.x')}</span>
          <span class="small">{r ? r.x.title || t(r.x.label) : '—'}</span>
        </div>
        {(def.kind === 'dls_distribution' || def.kind === 'dls_by_time') && (
          <Field label={t('gb.y_variable')}>
            <Seg
              size="sm"
              label={t('gb.y_variable')}
              value={def.weighting || 'intensity'}
              onValue={(w) => (def.kind === 'dls_distribution' ? p.setVariable(w) : set({ weighting: w }))}
              options={WEIGHTINGS.map((w) => ({ value: w, label: t('axis.' + w), tip: r?.weightings && !r.weightings.includes(w) ? t('graph.weighting_absent') : undefined }))}
            />
          </Field>
        )}
        {def.kind === 'parameter_time' && (
          <Field label={t('gb.y_variable')}>
            <Select value={def.param || 'effective_diameter'} onValue={(param) => set({ param })} options={PARAMS.map((k) => ({ value: k, label: t('param.' + k) }))} />
          </Field>
        )}
        {def.kind === 'series_values' && (
          <Field label={t('gb.y_variable')}>
            <Select value={def.param || values[0] || 'effective_diameter'} onValue={p.setVariable} options={(values.length ? values : ['effective_diameter']).map((k) => ({ value: k, label: variableLabel(k) }))} />
          </Field>
        )}
      </Section>

      <Section title={t('gb.s.axes')} class="gb-sec-axes">
        {(def.kind === 'dls_distribution' || def.kind === 'dls_by_time') && (
          <Field label={t('graph.xscale')}>
            <Seg
              size="sm"
              label={t('graph.xscale')}
              value={def.xScale || 'auto'}
              onValue={(xScale) => set({ xScale })}
              options={[
                { value: 'auto', label: t('graph.scale.auto') },
                { value: 'log', label: t('graph.scale.log') },
                { value: 'linear', label: t('graph.scale.linear') },
              ]}
            />
          </Field>
        )}
        <Field label={t('gb.yscale')}>
          <Seg
            size="sm"
            label={t('gb.yscale')}
            value={def.yScale || ''}
            onValue={(v) => set({ yScale: v as '' | 'log' })}
            options={[
              { value: '', label: t('graph.scale.linear') },
              { value: 'log', label: t('graph.scale.log') },
            ]}
          />
        </Field>
        <Field label={t('gb.y_unit')} hint={r?.y.conversion ? t('gb.y_unit.converted', { from: r.y.conversion.from, to: r.y.conversion.to, factor: r.y.conversion.factor }) : undefined}>
          {unitGroup ? (
            <Select
              label={t('gb.y_unit')}
              value={def.displayUnit && unitGroup.includes(def.displayUnit) ? def.displayUnit : ''}
              onValue={(u) => set({ displayUnit: u || undefined })}
              options={[{ value: '', label: t('gb.y_unit.stored', { unit: stored }) }, ...unitGroup.filter((u) => u !== stored).map((u) => ({ value: u, label: u }))]}
            />
          ) : (
            <span class="small gb-unit">{r ? unitText(stored, def.kind === 'parameter_time' || def.kind === 'series_values' ? def.param : undefined) || t('gb.y_unit.none') : '—'}</span>
          )}
        </Field>
        <Field label={t('gb.x_title')}>
          <Input size="sm" value={def.xLabel || ''} onValue={(xLabel) => set({ xLabel: xLabel || undefined })} placeholder={r ? t(r.x.label) : ''} maxLength={120} />
        </Field>
        <Field label={t('gb.y_title')}>
          <Input size="sm" value={def.yLabel || ''} onValue={(yLabel) => set({ yLabel: yLabel || undefined })} placeholder={r ? t(r.y.label) : ''} maxLength={120} />
        </Field>
      </Section>

      {(def.kind === 'parameter_time' || def.kind === 'statistical_groups') && (
        <Section title={t('gb.s.bars')} class="gb-sec-bars">
          <Field label={t('stat.error_bars')} hint={t('gb.bars.d')}>
            <Seg
              size="sm"
              label={t('stat.error_bars')}
              value={def.errorBars || 'sd'}
              onValue={(errorBars) => set({ errorBars: errorBars as 'sd' | 'sem' | 'ci' })}
              options={[
                { value: 'sd', label: t('gb.bars.sd') },
                { value: 'sem', label: t('fig.err.sem') },
                ...(def.kind === 'statistical_groups' ? [{ value: 'ci', label: t('fig.err.ci') }] : []),
              ]}
            />
          </Field>
          <span class="xs gb-bars-now">{t('gb.bars.now', { what: t('plot.error_bars.' + (r?.errorBars || def.errorBars || 'sd')) })}</span>
          <Switch checked={def.showIndividual !== false} onChange={(v) => set({ showIndividual: v ? undefined : false })} label={t('gb.individual')} />
          {def.kind === 'parameter_time' && <Switch checked={def.connectMeans !== false} onChange={(v) => set({ connectMeans: v ? undefined : false })} label={t('gb.connect')} />}
          {def.kind === 'statistical_groups' && <Button size="sm" onClick={() => navigate('/ls/statistics/' + def.analysisId)}>{t('stat.title')}</Button>}
        </Section>
      )}
      {def.kind === 'statistical_groups' && p.analysis && (
        <Section title={t('gb.s.stats')}>
          <Field label={t('stat.annotation_style')}>
            <Select label={t('stat.annotation_style')} value={def.annotationStyle || 'exact'} onValue={(value) => set({ annotationStyle: value as 'exact' | 'stars' })} options={[{ value: 'exact', label: t('stat.annotation_exact') }, { value: 'stars', label: t('stat.annotation_stars') }]} />
          </Field>
          <span class="small muted">{t('stat.annotations_help')}</span>
          {p.analysis.results.comparisons.map((c) => (
            <label class="row small" key={c.id}>
              <input type="checkbox" checked={(def.annotations || []).includes(c.id)} disabled={!(def.annotations || []).includes(c.id) && (def.annotations || []).length >= 8} onChange={(e) => set({ annotations: e.currentTarget.checked ? [...(def.annotations || []), c.id] : (def.annotations || []).filter((id) => id !== c.id) })} />
              <span>
                {c.contrast}
                {c.context ? ' · ' + c.context : ''} · p(adj)={c.adjustedP === 0 ? t('stat.p_underflow') : String(c.adjustedP)}
              </span>
            </label>
          ))}
          {def.annotationStyle === 'stars' && <span class="xs faint">{t('stat.star_thresholds')}</span>}
        </Section>
      )}

      <Section title={t('gb.s.lines')} class="gb-sec-lines">
        {(def.kind === 'dls_distribution' || def.kind === 'dls_by_time') && <Switch checked={def.visual.points} onChange={(points) => setVisual({ points })} label={t('graph.points')} />}
        <Range label={t('graph.line_width')} value={def.visual.lineWidth} min={0.75} max={4} step={0.25} onValue={(lineWidth) => setVisual({ lineWidth, preset: undefined })} fmt={(v) => fmtNum(v, 2) + ' pt'} />
        <span class="xs faint">{t('gb.lines.d')}</span>
      </Section>

      <Section title={t('graph.legend')}>
        <Switch checked={def.visual.legend} onChange={(legend) => setVisual({ legend, preset: undefined })} label={t('gb.legend.show')} />
        {def.visual.legend && (
          <Field label={t('gb.legend.mode')}>
            <Seg
              size="sm"
              value={def.visual.legendMode || 'visible'}
              onValue={(legendMode) => setVisual({ legendMode: legendMode as Visual['legendMode'] })}
              options={[
                { value: 'visible', label: t('gb.legend.visible') },
                { value: 'all', label: t('gb.legend.all') },
                { value: 'selected', label: t('gb.legend.selected') },
              ]}
            />
          </Field>
        )}
        {def.visual.legend && def.visual.legendMode === 'selected' && <span class="xs faint">{t('gb.legend.selected.d')}</span>}
      </Section>

      <Section title={t('gb.s.labels')} open={false}>
        <Field label={t('gb.subtitle')}>
          <Input size="sm" value={def.subtitle || ''} onValue={(subtitle) => set({ subtitle: subtitle || undefined })} maxLength={200} />
        </Field>
        <Field label={t('gb.notes')} hint={t('gb.notes.d')}>
          <div class="row gap1">
            <div class="grow">
              <Input size="sm" value={noteText} onValue={setNoteText} maxLength={200} onKeyDown={(e: KeyboardEvent) => e.key === 'Enter' && addNote()} />
            </div>
            <Button size="sm" icon="plus" tip={t('ui.add')} disabled={!noteText.trim() || !r} onClick={addNote} />
          </div>
        </Field>
        {notes.map((n, i) => (
          <div class="gb-note" key={i}>
            <span class="grow ellipsis small">{n.text}</span>
            <input class="input sm num" type="number" step="any" aria-label="X" value={n.x} onChange={(e) => set({ textNotes: notes.map((m, j) => (j === i ? { ...m, x: parseFloat(e.currentTarget.value) || 0 } : m)) })} />
            <input class="input sm num" type="number" step="any" aria-label="Y" value={n.y} onChange={(e) => set({ textNotes: notes.map((m, j) => (j === i ? { ...m, y: parseFloat(e.currentTarget.value) || 0 } : m)) })} />
            <Button size="sm" kind="ghost" icon="x" tip={t('ui.remove')} onClick={() => set({ textNotes: notes.filter((_, j) => j !== i) })} />
          </div>
        ))}
      </Section>

      <Section title={t('gb.s.publication')} open={false}>
        <Field label={t('gb.preset')}>
          <Select
            label={t('gb.preset')}
            value={def.visual.preset || ''}
            onValue={(name) => (name ? setVisual({ ...PRESETS[name], preset: name }) : setVisual({ preset: undefined }))}
            options={[{ value: '', label: t('gb.preset.custom') }, ...Object.keys(PRESETS).map((k) => ({ value: k, label: t('gb.preset.' + k) }))]}
          />
        </Field>
        <Switch checked={def.visual.metadata} onChange={(metadata) => setVisual({ metadata, preset: undefined })} label={t('graph.metadata')} />
        <Switch checked={def.visual.grid} onChange={(grid) => setVisual({ grid, preset: undefined })} label={t('graph.grid')} />
        <Range label={t('graph.text_size')} value={def.visual.fontScale} min={0.8} max={1.6} step={0.05} onValue={(fontScale) => setVisual({ fontScale, preset: undefined })} fmt={(v) => Math.round(v * 100) + '%'} />
        <Range label={t('gb.margin')} value={def.visual.margin || 0} min={0} max={20} step={1} onValue={(margin) => setVisual({ margin, preset: undefined })} fmt={(v) => (v ? fmtNum(v, 0) + ' mm' : t('gb.margin.auto'))} />
      </Section>
    </div>
  );
}

function Range(p: { label: string; value: number; min: number; max: number; step: number; onValue: (v: number) => void; fmt: (v: number) => string }) {
  return (
    <label class="col gap1 small">
      <span class="row">
        <span class="grow muted">{p.label}</span>
        <span class="num xs faint">{p.fmt(p.value)}</span>
      </span>
      <input type="range" min={p.min} max={p.max} step={p.step} value={p.value} onInput={(e) => p.onValue(parseFloat((e.target as HTMLInputElement).value))} style={{ accentColor: 'var(--accent)' }} />
    </label>
  );
}

/** DataPanel lists what the graph draws: its series, cycle or analysis. */
function DataPanel(p: { def: GraphDef; cycleName?: string; cycleN?: number; selected: string[]; all: SeriesEntry[]; onRemove: (id: string) => void }) {
  const def = p.def;
  const byId = useMemo(() => new Map(p.all.map((s) => [s.id, s])), [p.all]);
  if (def.cycleId && (def.kind === 'parameter_time' || def.kind === 'dls_by_time'))
    return (
      <div class="col gap2">
        <span class="section-title">{t('meta.cycle')}</span>
        <Button size="sm" kind="ghost" icon="cycle" class="row-btn" onClick={() => navigate('/ls/cycles/' + def.cycleId)}>
          <span class="ellipsis">{p.cycleName || def.cycleId}</span>
        </Button>
        {p.cycleN !== undefined && <span class="xs faint">{t('graph.cycle_measurements', { n: p.cycleN })}</span>}
      </div>
    );
  if (def.kind === 'statistical_groups')
    return (
      <Button size="sm" kind="ghost" icon="sigma" class="row-btn" onClick={() => navigate('/ls/statistics/' + def.analysisId)}>
        {t('stat.title')}
      </Button>
    );
  if (!p.selected.length) return <span class="small faint">{t('gb.data.none')}</span>;
  return (
    <div class="col gap1">
      <span class="section-title">{t('gb.data.selected', { n: p.selected.length })}</span>
      {p.selected.map((id) => {
        const s = byId.get(id);
        return (
          <div class="series-row" key={id}>
            <Icon name="file" size="sm" />
            <span class="grow col" style={{ gap: 0, minWidth: 0 }}>
              <span class="ellipsis small">{s ? seriesName(s) : t('gb.data.missing')}</span>
              {s && (
                <a
                  href={'/ls/files?file=' + s.fileId}
                  class="xs faint ellipsis"
                  onClick={(e) => {
                    e.preventDefault();
                    navigate('/ls/files?file=' + s.fileId);
                  }}
                >
                  {seriesDetail(s)}
                </a>
              )}
            </span>
            <Button size="sm" kind="ghost" icon="x" tip={t('ui.remove')} onClick={() => p.onRemove(id)} />
          </div>
        );
      })}
      {def.kind === 'dls_distribution' && !def.cycleId && (
        <Button size="sm" icon="cycle" class="row-btn" onClick={() => openPanel('cycle-wizard', { measurements: def.measurements })}>
          {t('files.cycle')}
        </Button>
      )}
    </div>
  );
}

/** StylePanel: name, colour, marker, line pattern, legend and visibility of each drawn series. */
function StylePanel(p: { series: Series[]; def: GraphDef; setStyle: (id: string, st: Partial<SeriesStyle>) => void; setVisual: (patch: Partial<Visual>) => void }) {
  const [editing, setEditing] = useState<string | null>(null);
  const [val, setVal] = useState('');
  if (!p.series.length) return <span class="small faint">{t('graph.no_series')}</span>;
  const legendSel = p.def.visual.legendMode === 'selected';
  const inLegend = p.def.visual.legendSeries || [];
  const st = (id: string) => p.def.series?.[id] || {};
  return (
    <div class="series-list">
      <span class="xs faint" style={{ marginBottom: 6 }}>
        {t('graph.series.hint')}
      </span>
      {p.series.map((se) => {
        const s = st(se.id);
        const hidden = !!se.hidden;
        return (
          <div class={`series-style ${hidden ? 'off' : ''} ${se.muted ? 'muted' : ''}`} key={se.id}>
            <div class="series-row">
              <label class="swatch" style={{ '--c': se.color || '#2C7FB8' } as any} data-tip={t('graph.series.color')}>
                <input type="color" value={se.color || '#2C7FB8'} onInput={(e) => p.setStyle(se.id, { color: (e.target as HTMLInputElement).value.toUpperCase() })} aria-label={t('graph.series.color')} />
              </label>
              {editing === se.id ? (
                <input
                  class="input sm grow"
                  value={val}
                  autoFocus
                  maxLength={120}
                  onInput={(e) => setVal((e.target as HTMLInputElement).value)}
                  onBlur={() => (p.setStyle(se.id, { label: val.trim() }), setEditing(null))}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
                    if (e.key === 'Escape') setEditing(null);
                  }}
                />
              ) : (
                <span class="grow ellipsis small" onDblClick={() => (setEditing(se.id), setVal(s.label || seriesLabel(se.label)))} title={seriesLabel(se.label)}>
                  {seriesLabel(se.label)}
                </span>
              )}
              {legendSel && (
                <Button size="sm" kind="ghost" icon="list" selected={inLegend.includes(se.id)} tip={t('gb.legend.include')} onClick={() => p.setVisual({ legendSeries: inLegend.includes(se.id) ? inLegend.filter((x) => x !== se.id) : [...inLegend, se.id] })} />
              )}
              <Button size="sm" kind="ghost" icon="pencil" tip={t('graph.series.rename')} onClick={() => (setEditing(se.id), setVal(s.label || seriesLabel(se.label)))} />
              <Button size="sm" kind="ghost" icon={hidden ? 'eyeOff' : 'eye'} tip={hidden ? t('graph.series.show') : t('graph.series.hide')} onClick={() => p.setStyle(se.id, { hidden: !hidden })} />
            </div>
            <div class="row gap1 series-shape">
              <Select size="sm" label={t('gb.marker')} value={(s.marker || '') as string} onValue={(m) => p.setStyle(se.id, { marker: (m || undefined) as SeriesStyle['marker'] })} options={[{ value: '', label: t('gb.marker.auto') }, ...MARKERS.map((m) => ({ value: m, label: t('gb.marker.' + m) })), ...(se.kind === 'line' ? [{ value: 'none', label: t('gb.marker.none') }] : [])]} />
              {se.kind !== 'individual' && <Select size="sm" label={t('gb.line')} value={(s.lineStyle || '') as string} onValue={(l) => p.setStyle(se.id, { lineStyle: (l || undefined) as LineStyle })} options={[{ value: '', label: t('gb.line.auto') }, ...LINES.map((l) => ({ value: l, label: t('gb.line.' + l) }))]} />}
            </div>
          </div>
        );
      })}
      {Object.keys(p.def.series || {}).length > 0 && (
        <Button size="sm" kind="ghost" icon="reset" onClick={() => Object.keys(p.def.series || {}).forEach((id) => p.setStyle(id, { label: '', color: '', hidden: false, marker: undefined, lineStyle: undefined }))}>
          {t('graph.series.reset')}
        </Button>
      )}
    </div>
  );
}

function DetailsPanel(p: { r: Rendered | null; def: GraphDef; set: (patch: Partial<GraphDef>) => void }) {
  const [tab, setTab] = useState<'provenance' | 'compare' | 'notes'>('provenance');
  return (
    <div class="col gap3">
      <Seg
        size="sm"
        value={tab}
        onValue={setTab}
        options={[
          { value: 'provenance', label: t('graph.t.provenance') },
          { value: 'compare', label: t('graph.t.compare') },
          { value: 'notes', label: t('graph.t.notes') },
        ]}
      />
      {tab === 'notes' ? <NotesPanel def={p.def} set={p.set} /> : !p.r ? <Skeleton h={80} /> : tab === 'compare' ? <ComparePanel r={p.r} /> : <ProvenancePanel r={p.r} def={p.def} />}
    </div>
  );
}

function ComparePanel(p: { r: Rendered }) {
  const rows = (p.r.series || []).filter((s) => s.meta);
  if (!rows.length) return <span class="small faint">{t('graph.compare.none')}</span>;
  const cell = (s: Series, k: string) => {
    const raw = s.meta?.[k];
    if (raw === undefined) return <span class="faint">—</span>;
    return fmtQ({ value: parseFloat(raw.replace(',', '.')), raw, unit: s.meta?.[k + '.unit'] }, k);
  };
  return (
    <div class="col gap3">
      {rows.slice(0, 60).map((s) => (
        <div class="meas" style={{ gap: 6 }}>
          <div class="row gap2">
            <span class="dot" style={{ color: s.color }} />
            <b class="small ellipsis grow">{seriesLabel(s.label)}</b>
          </div>
          {s.meta?.measuredAt && <span class="xs faint">{fmtTS({ time: s.meta.measuredAt, raw: '', tzKnown: s.meta['measuredAt.tz'] !== 'unknown', source: 'file' })}</span>}
          <div class="params">
            {PARAMS.map((k) => (
              <div class="param">
                <div class="k">{t('param.short.' + k)}</div>
                <div class="v">{cell(s, k)}</div>
              </div>
            ))}
          </div>
        </div>
      ))}
      <span class="xs faint">{t('graph.compare.d')}</span>
    </div>
  );
}

const transformKey = (s: string) => (s.startsWith('none') ? 'prov.tr.none' : s.startsWith('replicates') ? 'prov.tr.grouped' : s.startsWith('display unit') ? 'prov.tr.display_unit' : '');

function ProvenancePanel(p: { r: Rendered; def: GraphDef }) {
  const pv = p.r.provenance;
  const rel = useRelations();
  const derived = (p.def.id && rel.data?.graphs[p.def.id]?.derived) || [];
  return (
    <div class="col gap3">
      {(!!p.def.derivedFrom?.length || derived.length > 0) && (
        <div class="col gap1 gb-lineage">
          <span class="section-title">{t('gb.lineage')}</span>
          {(p.def.derivedFrom || []).map((l) => (
            <a
              href={'/ls/graphs/' + l.graphId}
              class="small"
              onClick={(e) => {
                e.preventDefault();
                navigate('/ls/graphs/' + l.graphId);
              }}
            >
              <Icon name="history" size="sm" /> {t('gb.derived_from', { name: l.title || '—' })} · {t('gb.relation.' + l.relation)} · {fmtDate(l.at, true)}
            </a>
          ))}
          {derived.length > 0 && <span class="xs faint">{t('gb.derived_n', { n: derived.length })}</span>}
        </div>
      )}
      <dl class="kv">
        <dt>{t('meta.engine')}</dt>
        <dd class="mono xs">{pv.engine}</dd>
        <dt>{t('col.spec')}</dt>
        <dd class="mono xs">{pv.spec}</dd>
        <dt>{t('prov.computed')}</dt>
        <dd>{fmtDate(pv.computedAt, true)}</dd>
        {p.def.created && (
          <>
            <dt>{t('prov.created')}</dt>
            <dd>{fmtDate(p.def.created, true)}</dd>
          </>
        )}
        {p.def.calculationVersion && (
          <>
            <dt>{t('gb.saved_with')}</dt>
            <dd class="mono xs">{p.def.calculationVersion}</dd>
          </>
        )}
        {!!p.def.parserVersions?.length && (
          <>
            <dt>{t('gb.parsers')}</dt>
            <dd class="mono xs">{p.def.parserVersions.join(', ')}</dd>
          </>
        )}
        {!!p.def.seriesIds?.length && (
          <>
            <dt>{t('gb.series_ids')}</dt>
            <dd class="xs">{t('gb.series_ids.n', { n: p.def.seriesIds.length })}</dd>
          </>
        )}
      </dl>
      <div class="col gap1">
        <span class="section-title">{t('prov.transformations')}</span>
        {(pv.transformations || []).map((x) => (
          <span class="small muted">{transformKey(x) ? t(transformKey(x)) : x}</span>
        ))}
        {pv.statistics && <span class="small muted">{t('plot.meta.stats')}</span>}
      </div>
      <div class="col gap2">
        <span class="section-title">{t('prov.sources', { n: pv.sources?.length || 0 })}</span>
        <div class="prov">
          {(pv.sources || []).slice(0, 100).map((s) => (
            <div class="prov-item col gap1">
              <a
                href={'/ls/files?file=' + s.fileId}
                class="small ellipsis"
                onClick={(e) => {
                  e.preventDefault();
                  navigate('/ls/files?file=' + s.fileId);
                }}
              >
                <Icon name="file" size="sm" /> {s.fileName}
              </a>
              <span class="faint">
                {[s.sampleId, s.sourceSheet && t('ls.source_sheet', { name: s.sourceSheet }), s.method && t('ls.method.' + s.method), s.format && t('ls.layout.' + s.format), s.measuredAt && fmtTS({ time: s.measuredAt, raw: '', tzKnown: false, source: 'file' }), s.lines && t('prov.lines', { l: s.lines })].filter(Boolean).join(' · ')}
              </span>
              <span class="mono faint" title={s.sha256}>
                {s.parser} · {s.sha256.slice(0, 12)}…
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function NotesPanel(p: { def: GraphDef; set: (patch: Partial<GraphDef>) => void }) {
  const [tag, setTag] = useState('');
  const tags = p.def.tags || [];
  const add = () => {
    const v = tag.trim().replace(/^#/, '');
    if (v && !tags.includes(v)) p.set({ tags: [...tags, v] });
    setTag('');
  };
  return (
    <div class="col gap3">
      <Field label={t('files.f.tags')}>
        <div class="col gap2">
          {tags.length > 0 && (
            <div class="tags">
              {tags.map((x) => (
                <span class="tag">
                  #{x}
                  <button aria-label={t('ui.remove')} onClick={() => p.set({ tags: tags.filter((y) => y !== x) })}>
                    <Icon name="x" />
                  </button>
                </span>
              ))}
            </div>
          )}
          <Input size="sm" icon="tag" value={tag} onValue={setTag} placeholder={t('files.tag.add')} onKeyDown={(e: KeyboardEvent) => (e.key === 'Enter' || e.key === ',') && (e.preventDefault(), add())} onBlur={add} />
        </div>
      </Field>
      <Field label={t('files.f.notes')}>
        <textarea class="textarea" rows={6} value={p.def.notes || ''} maxLength={4000} onInput={(e) => p.set({ notes: (e.target as HTMLTextAreaElement).value })} />
      </Field>
    </div>
  );
}
