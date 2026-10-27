// Graph Builder: the Series Explorer on the left, the live preview in the
// centre and the graph settings on the right, with Save, Duplicate,
// Combine, Analyze, Present and Export below. The core computes and lays
// out the figure; changing the display never changes the data, and choosing
// series only keeps their IDs.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { get, post } from '../../lib/api';
import { statisticsHeaders, useAnalyses } from '../../lib/analyses';
import { analyzeRoute, COMBINABLE, isDesignKind, saveGroup, useDesigns, useGroups } from '../../lib/groups';
import { fmtDate, fmtNum, fmtQ, fmtTS, unitText } from '../../lib/format';
import { errText, has, t } from '../../lib/i18n';
import { PARAMS, useCycles, useGraphs, useRelations, warnText, WEIGHTINGS } from '../../lib/library';
import { navigate } from '../../lib/route';
import { accepts, bySelection, selectionOf, seriesDetail, seriesName, useSeriesCatalog, variableLabel, variablesFor, withVariable } from '../../lib/series';
import { openPanel, run, toast } from '../../lib/state';
import type { DesignView, GraphDef, GraphKind, Group, LineStyle, MarkerShape, Series, SeriesEntry, SeriesStyle, Visual } from '../../lib/types';
import type { StatisticalAnalysis } from '../../lib/statistics';
import { FigureView, seriesLabel } from '../../plot/Figure';
import { defaultVisual, useRendered, useSize, type Rendered } from '../../plot/useRender';
import { Icon } from '../../ui/icons';
import { Badge, Button, Check, confirmDialog, Empty, Field, Input, Notice, Seg, Select, Skeleton, Switch, Tabs, useAsync } from '../../ui/kit';
import { CombineDialog } from './combine';
import { presentGraphs } from './graphs';
import { SeriesExplorer } from './series-explorer';

function newDef(q: URLSearchParams): GraphDef {
  const cycleId = q.get('cycle') || undefined;
  const designId = q.get('design') || undefined;
  const kind = (q.get('kind') as GraphKind) || (cycleId ? 'parameter_time' : designId ? 'design_time' : 'dls_distribution');
  const list = (k: string) => (q.get(k) || '').split(',').filter(Boolean);
  return {
    title: q.get('title') || '',
    kind,
    measurements: list('m'),
    weighting: q.get('weighting') || 'intensity',
    xScale: 'auto',
    cycleId,
    // Design graphs: the design revision, its conditions and groups (references only).
    ...(isDesignKind(kind) ? { designId, conditions: q.getAll('conditions').filter(Boolean).length ? q.getAll('conditions').filter(Boolean) : undefined, groups: list('groups').length ? list('groups') : undefined, pairedAt: q.get('at') || undefined } : {}),
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
      const q = new URLSearchParams(location.search);
      const d = newDef(q);
      setDef(d);
      setBase(null);
      // A saved selection opens with its series (references only).
      const gid = q.get('group');
      if (gid)
        get<Group>('/api/groups/' + gid, { headers: statisticsHeaders() })
          .then((g) => setDef({ ...d, seriesIds: g.seriesIds?.length ? g.seriesIds : undefined, measurements: g.measurements }))
          .catch(() => {});
    } else if (saved.data) {
      const link = new URLSearchParams(location.search).get('analysis');
      if (link && isDesignKind(saved.data.kind) && saved.data.analysisId !== link) {
        // Annotate: the analysis is linked in the editor; saving keeps it.
        setDef({ ...saved.data, analysisId: link, annotations: [], annotationStyle: saved.data.annotationStyle || 'exact' });
        toast('info', t('ga.linked'));
      } else setDef(saved.data);
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
  // A paired view draws only comparisons between conditions at its time point.
  const set = (patch: Partial<GraphDef>) => p.setDef(fitAnnotations({ ...def, ...patch }));
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
  const designKind = isDesignKind(def.kind);
  const designs = useDesigns();
  const heads = (designs.data || []).filter((v) => v.head);
  // The pinned design revision (it may be older than the head).
  const pinned = useAsync(() => (designKind && def.designId ? get<DesignView>('/api/designs/' + def.designId, { headers: statisticsHeaders() }) : Promise.resolve(null)), [designKind, def.designId]);
  const designView = pinned.data || null;
  const latest = designView ? heads.find((v) => v.design.root === designView.design.root) : undefined;
  const [combine, setCombine] = useState<string[] | null>(null);
  function fitAnnotations(d: GraphDef): GraphDef {
    if (d.kind !== 'paired_condition' || !d.annotations?.length || !analysis.data) return d;
    const ok = new Set(pairedComparisons(analysis.data, d.pairedAt || designView?.design.times[0]?.label || '').map((c) => c.id));
    return { ...d, annotations: d.annotations.filter((id) => ok.has(id)) };
  }

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
    if (isDesignKind(kind)) {
      // A design graph draws a saved design by experimental unit.
      const d = designView || heads[0];
      const param = (PARAMS as readonly string[]).includes(def.param || '') ? def.param : 'effective_diameter';
      set({ kind, designId: def.designId || d?.design.id, param, cycleId: undefined, seriesIds: undefined, measurements: [], focusSeries: [], pairedAt: kind === 'paired_condition' ? def.pairedAt || d?.design.times[d.design.times.length - 1]?.label : undefined, unitTrajectories: kind === 'design_time' ? def.unitTrajectories : undefined });
      return;
    }
    if (isDesignKind(def.kind)) {
      set({ kind, designId: undefined, conditions: undefined, pairedAt: undefined, groups: undefined, unitTrajectories: undefined, analysisId: undefined, annotations: undefined, errorBars: undefined, seriesIds: [], measurements: [], ...(kind === 'series_values' ? { param: variablesFor(kind, all)[0] || 'effective_diameter' } : { weighting: 'intensity' }) });
      return;
    }
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
  const errBars = def.kind === 'parameter_time' || def.kind === 'statistical_groups' || designKind ? ((r.data?.errorBars || def.errorBars || 'sd') as 'sd' | 'sem' | 'ci') : undefined;
  const route = analyzeRoute({ ...def, id: def.id || 'unsaved' });
  const analyze = async () => {
    const id = await needSave();
    const to = id && analyzeRoute({ ...def, id });
    if (to) navigate(to);
  };
  const lineage = def.derivedFrom || [];
  const combined = lineage.some((l) => l.relation === 'combine');
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
        {!!lineage.length && (
          <Badge kind="info" icon={combined ? 'layers' : 'history'} tip={lineage.map((l) => t('gb.derived_from', { name: l.title || '—' })).join(' · ')}>
            {combined ? t('gc.badge') : t('gb.copy')}
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
                <Notice kind="info">{designKind ? t('dg.data') : def.kind === 'statistical_groups' ? t('gb.data_from_analysis') : t('gb.data_from_cycle')}</Notice>
                <DataPanel def={def} cycleName={cycle?.config.name} cycleN={cycle?.measurements.length} selected={[]} all={all} onRemove={() => {}} design={designView} />
              </div>
            )
          ) : left === 'data' ? (
            <div class="panel-body pad">
              <DataPanel def={def} cycleName={cycle?.config.name} cycleN={cycle?.measurements.length} selected={selected} all={all} onRemove={(id) => select(selected.filter((x) => x !== id))} design={designView} />
            </div>
          ) : (
            <div class="panel-body pad">
              <GroupsPanel def={def} set={set} selectable={selectable} selected={selected} entries={entries} onSelect={select} />
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
                yUnit={unitText(r.data.y.unit, def.kind === 'parameter_time' || def.kind === 'series_values' || designKind ? def.param : undefined)}
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
              <Settings def={def} set={set} setVisual={setVisual} setKind={setKind} setVariable={setVariable} r={r.data} all={all} cycles={cycles.data || []} analysis={analysis.data || null} heads={heads} design={designView} latest={latest} />
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
          <Button size="sm" icon="layers" class="gb-combine" disabled={busy || !COMBINABLE.includes(def.kind) || !r.data} tip={t('gb.combine.d')} onClick={async () => { const id = await needSave(); if (id) setCombine([id]); }}>
            {t('gb.combine')}
          </Button>
          <Button size="sm" icon="sigma" class="gb-analyze" disabled={busy || !route || !r.data} tip={route ? t('gb.analyze.d') : t('ga.cannot')} onClick={analyze}>
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
      {combine && <CombineDialog ids={combine} onClose={() => setCombine(null)} />}
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
  heads: DesignView[];
  design: DesignView | null;
  latest?: DesignView;
}) {
  const { def, set, setVisual, r } = p;
  const designKind = isDesignKind(def.kind);
  const analyses = useAnalyses();
  // Only saved analyses of this design (any revision of it) and variable.
  const compatible = designKind && p.design ? (analyses.data || []).filter((a) => a.snapshot.experimentalDesign?.root === p.design!.design.root && a.snapshot.definition.variable === (def.param || 'effective_diameter')) : [];
  const maxAnn = designKind ? 12 : 8;
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
              def.kind === 'statistical_groups'
                ? [{ value: 'statistical_groups', label: t('graph.kind.statistical_groups') }]
                : [
                    { value: 'dls_distribution', label: t('graph.kind.dls_distribution') },
                    { value: 'series_values', label: t('graph.kind.series_values'), disabled: !values.length },
                    { value: 'parameter_time', label: t('graph.kind.parameter_time'), disabled: !def.cycleId },
                    { value: 'dls_by_time', label: t('graph.kind.dls_by_time'), disabled: !def.cycleId },
                    { value: 'design_time', label: t('graph.kind.design_time'), disabled: !p.heads.length && !def.designId },
                    { value: 'paired_condition', label: t('graph.kind.paired_condition'), disabled: !p.heads.length && !def.designId },
                  ]
            }
          />
        </Field>
        {!def.cycleId && !def.analysisId && !designKind && <span class="xs faint">{t('graph.cycle_kinds_hint')}</span>}
        {designKind && <DesignSettings def={def} set={set} heads={p.heads} design={p.design} latest={p.latest} />}
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
        {(def.kind === 'parameter_time' || designKind) && (
          <Field label={t('gb.y_variable')}>
            <Select label={t('gb.y_variable')} value={def.param || 'effective_diameter'} onValue={(param) => set({ param, ...(designKind && param !== def.param ? { analysisId: undefined, annotations: undefined, errorBars: def.errorBars === 'ci' ? 'sd' : def.errorBars } : {}) })} options={PARAMS.map((k) => ({ value: k, label: t('param.' + k) }))} />
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

      {(def.kind === 'parameter_time' || def.kind === 'statistical_groups' || designKind) && (
        <Section title={t('gb.s.bars')} class="gb-sec-bars">
          <Field label={t('stat.error_bars')} hint={designKind && !def.analysisId ? t('dg.bars.ci_needs') : t('gb.bars.d')}>
            <Seg
              size="sm"
              label={t('stat.error_bars')}
              value={def.errorBars || 'sd'}
              onValue={(errorBars) => set({ errorBars: errorBars as 'sd' | 'sem' | 'ci' })}
              options={[
                { value: 'sd', label: t('gb.bars.sd') },
                { value: 'sem', label: t('fig.err.sem') },
                ...(def.kind === 'statistical_groups' || (designKind && def.analysisId) ? [{ value: 'ci', label: t('fig.err.ci') }] : []),
              ]}
            />
          </Field>
          <span class="xs gb-bars-now">{t('gb.bars.now', { what: t('plot.error_bars.' + (r?.errorBars || def.errorBars || 'sd')) })}</span>
          <Switch checked={def.showIndividual !== false} onChange={(v) => set({ showIndividual: v ? undefined : false })} label={t('gb.individual')} />
          {(def.kind === 'parameter_time' || def.kind === 'design_time') && <Switch checked={def.connectMeans !== false} onChange={(v) => set({ connectMeans: v ? undefined : false })} label={t('gb.connect')} />}
          {def.kind === 'design_time' && <Switch checked={!!def.unitTrajectories} onChange={(v) => set({ unitTrajectories: v || undefined })} label={t('dg.trajectories')} />}
          {designKind && <span class="xs faint dg-n-rule">{t('dg.n_rule')}</span>}
          {def.kind === 'statistical_groups' && <Button size="sm" onClick={() => navigate('/ls/statistics/' + def.analysisId)}>{t('stat.title')}</Button>}
        </Section>
      )}
      {designKind && (
        <Section title={t('gb.s.stats')} class="gb-sec-stats">
          <Field label={t('an.analysis')} hint={compatible.length ? t('an.analysis.d') : t('an.analysis.empty')}>
            <Select
              label={t('an.analysis')}
              value={def.analysisId || ''}
              onValue={(id) => set({ analysisId: id || undefined, annotations: undefined, errorBars: !id && def.errorBars === 'ci' ? 'sd' : def.errorBars })}
              options={[{ value: '', label: t('an.analysis.none') }, ...compatible.map((a) => ({ value: a.id, label: a.snapshot.definition.title + ' · ' + fmtDate(a.created) })), ...(def.analysisId && !compatible.some((a) => a.id === def.analysisId) ? [{ value: def.analysisId, label: p.analysis?.snapshot.definition.title || def.analysisId }] : [])]}
            />
          </Field>
        </Section>
      )}
      {(def.kind === 'statistical_groups' || designKind) && p.analysis && (
        <Section title={designKind ? t('stat.annotation_style') : t('gb.s.stats')} class="gb-sec-annotations">
          <Field label={t('stat.annotation_style')}>
            <Select
              label={t('stat.annotation_style')}
              value={def.annotationStyle || 'exact'}
              onValue={(value) => set({ annotationStyle: value as 'exact' | 'stars' | 'effect' })}
              options={[{ value: 'exact', label: t('stat.annotation_exact') }, { value: 'stars', label: t('stat.annotation_stars') }, ...(designKind ? [{ value: 'effect', label: t('an.effect') }] : [])]}
            />
          </Field>
          <span class="small muted">{t('stat.annotations_help')}</span>
          {designKind && <span class="xs faint">{t('an.limit', { n: maxAnn })}</span>}
          {families(def.kind === 'paired_condition' ? pairedComparisons(p.analysis, def.pairedAt || p.design?.design.times[0]?.label || '') : p.analysis.results.comparisons).map(([fam, list]) => (
            <div class="col gap1 an-family" key={fam}>
              {designKind && <span class="section-title">{t('an.family.' + (fam === 'A_within_B' || fam === 'B_within_A' ? fam : 'other'))}</span>}
              {list.map((c) => (
                <label class="row small an-comparison" key={c.id}>
                  <input type="checkbox" checked={(def.annotations || []).includes(c.id)} disabled={!(def.annotations || []).includes(c.id) && (def.annotations || []).length >= maxAnn} onChange={(e) => set({ annotations: e.currentTarget.checked ? [...(def.annotations || []), c.id] : (def.annotations || []).filter((id) => id !== c.id) })} />
                  <span>
                    {c.contrast}
                    {c.context ? ' · ' + c.context : ''} · p(adj)={c.adjustedP === 0 ? t('stat.p_underflow') : String(c.adjustedP)}
                  </span>
                </label>
              ))}
            </div>
          ))}
          {!!r?.annotations?.length && <span class="xs muted an-drawn">{t('an.drawn', { n: r.annotations.length })}</span>}
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

/** The comparisons a paired view can draw: between conditions at its time point. */
const pairedComparisons = (a: StatisticalAnalysis, at: string) => a.results.comparisons.filter((c) => c.leftB === at && c.rightB === at && c.leftA !== c.rightA);

/** Comparisons by family (condition at each time, time within each condition), in stored order. */
function families(cs: StatisticalAnalysis['results']['comparisons']): [string, StatisticalAnalysis['results']['comparisons']][] {
  const out = new Map<string, StatisticalAnalysis['results']['comparisons']>();
  for (const c of cs) {
    const k = c.family || '';
    out.set(k, [...(out.get(k) || []), c]);
  }
  return [...out.entries()];
}

/** DesignSettings: the design revision, its conditions and the paired time point. */
function DesignSettings(p: { def: GraphDef; set: (patch: Partial<GraphDef>) => void; heads: DesignView[]; design: DesignView | null; latest?: DesignView }) {
  const { def, set } = p;
  const d = p.design?.design;
  if (!p.heads.length && !def.designId)
    return (
      <Notice kind="info" action={<Button size="sm" icon="layers" onClick={() => navigate('/ls/designs')}>{t('ls.tab.designs')}</Button>}>
        <b>{t('dg.design.none')}</b> {t('dg.design.none.d')}
      </Notice>
    );
  const options = p.heads.map((v) => ({ value: v.design.id, label: t('dg.design.revision', { name: v.design.name, n: v.design.revision }) }));
  if (d && !options.some((o) => o.value === d.id)) options.unshift({ value: d.id, label: t('dg.design.revision', { name: d.name, n: d.revision }) });
  const conds = d?.conditions.map((c) => c.label) || [];
  const chosen = def.conditions?.length ? def.conditions : conds;
  const toggle = (c: string) => {
    const next = chosen.includes(c) ? chosen.filter((x) => x !== c) : conds.filter((x) => x === c || chosen.includes(x));
    if (!next.length) return;
    set({ conditions: next.length === conds.length ? undefined : next });
  };
  const st = p.design?.structure;
  return (
    <div class="col gap2 dg-settings">
      <Field label={t('dg.design')}>
        <Select label={t('dg.design')} value={def.designId || ''} onValue={(designId) => set({ designId, conditions: undefined, pairedAt: undefined })} options={options} />
      </Field>
      {d && p.latest && p.latest.design.id !== d.id && (
        <Notice kind="info" action={<Button size="sm" class="dg-use-latest" onClick={() => set({ designId: p.latest!.design.id })}>{t('dg.design.use_latest', { n: p.latest.design.revision })}</Button>}>
          {t('dg.design.newer', { n: d.revision, latest: p.latest.design.revision })}
        </Notice>
      )}
      {st && (
        <span class="xs muted dg-summary">
          {t('dg.summary', { units: st.experimentalUnits, conditions: st.conditions.length, times: st.times.length })}
        </span>
      )}
      {conds.length > 0 && (
        <Field label={t('dg.conditions')} hint={def.kind === 'design_time' ? t('dg.conditions.d') : undefined}>
          <div class="col gap1 dg-conditions">
            {conds.map((c) => (
              <Check key={c} checked={chosen.includes(c)} onChange={() => toggle(c)} label={c} />
            ))}
          </div>
        </Field>
      )}
      {def.kind === 'paired_condition' && d && (
        <Field label={t('dg.paired_at')}>
          <Select label={t('dg.paired_at')} value={def.pairedAt || d.times[d.times.length - 1]?.label || ''} onValue={(pairedAt) => set({ pairedAt })} options={d.times.map((x) => ({ value: x.label, label: x.label }))} />
        </Field>
      )}
      {d && (
        <Button size="sm" kind="ghost" icon="layers" class="row-btn" onClick={() => navigate('/ls/designs/' + d.id)}>
          {t('dg.design.open')}
        </Button>
      )}
    </div>
  );
}

/**
 * GroupsPanel: saved groups and selections. A design graph can draw only
 * some groups; a series graph adds a group's series to its selection, and
 * the current selection can be saved as a group. References only.
 */
function GroupsPanel(p: { def: GraphDef; set: (patch: Partial<GraphDef>) => void; selectable: boolean; selected: string[]; entries: SeriesEntry[]; onSelect: (ids: string[]) => void }) {
  const groups = useGroups();
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const list = groups.data || [];
  const designKind = isDesignKind(p.def.kind);
  const chosen = p.def.groups || [];
  const add = (g: Group) => {
    const ids = p.entries.filter((e) => (g.seriesIds?.length ? g.seriesIds.includes(e.id) : g.measurements.includes(e.measurementId))).map((e) => e.id);
    const next = [...new Set([...p.selected, ...ids])];
    if (!ids.length) return toast('info', t('grp.none_for_variable'));
    p.onSelect(next);
    toast('success', t('grp.added', { n: next.length - p.selected.length }));
  };
  const saveSel = async () => {
    setBusy(true);
    const g = await run(() => saveGroup({ name: name.trim(), measurements: [], seriesIds: p.selected }));
    setBusy(false);
    if (!g) return;
    setName('');
    toast('success', t('grp.saved', { name: g.name }));
  };
  return (
    <div class="col gap3 gb-groups">
      <span class="xs faint">{t('grp.d')}</span>
      {groups.loading && !groups.data ? (
        <Skeleton h={60} />
      ) : !list.length ? (
        <Empty icon="layers" title={t('grp.empty')} body={t('grp.empty.d')} />
      ) : (
        <div class="col gap1">
          {designKind && <span class="section-title">{t('grp.filter')}</span>}
          {list.map((g) => (
            <div class="series-row grp-row" key={g.id}>
              {designKind ? (
                <Check checked={chosen.includes(g.id!)} onChange={(v) => { const next = v ? [...chosen, g.id!] : chosen.filter((x) => x !== g.id); p.set({ groups: next.length ? next : undefined }); }} aria={g.name} />
              ) : (
                <Icon name="layers" size="sm" />
              )}
              <span class="grow col" style={{ gap: 0, minWidth: 0 }}>
                <span class="ellipsis small">{g.name}</span>
                <span class="xs faint">
                  {t('grp.kind.' + (g.kind || 'group'))} · {g.seriesIds?.length ? t('grp.series', { n: g.seriesIds.length }) : t('grp.members', { n: g.measurements.length })}
                </span>
              </span>
              {p.selectable && (
                <Button size="sm" kind="ghost" icon="plus" tip={t('grp.add')} onClick={() => add(g)} />
              )}
            </div>
          ))}
          {designKind && <span class="xs faint">{t('grp.filter.d')}</span>}
        </div>
      )}
      {p.selectable && p.selected.length > 0 && (
        <Field label={t('grp.save_selection')} hint={t('grp.save_selection.d', { n: p.selected.length })}>
          <div class="row gap1">
            <div class="grow">
              <Input size="sm" value={name} onValue={setName} placeholder={t('grp.name.ph')} aria-label={t('grp.name')} maxLength={120} onKeyDown={(e: KeyboardEvent) => e.key === 'Enter' && name.trim() && saveSel()} />
            </div>
            <Button size="sm" icon="save" class="grp-save-selection" busy={busy} disabled={!name.trim()} onClick={saveSel}>
              {t('ui.save')}
            </Button>
          </div>
        </Field>
      )}
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
function DataPanel(p: { def: GraphDef; cycleName?: string; cycleN?: number; selected: string[]; all: SeriesEntry[]; onRemove: (id: string) => void; design?: DesignView | null }) {
  const def = p.def;
  const byId = useMemo(() => new Map(p.all.map((s) => [s.id, s])), [p.all]);
  if (isDesignKind(def.kind)) {
    const v = p.design;
    if (!v) return <span class="small faint">{def.designId ? <Skeleton h={40} /> : t('dg.design.none')}</span>;
    return (
      <div class="col gap2 dg-data">
        <span class="section-title">{t('dg.design')}</span>
        <Button size="sm" kind="ghost" icon="layers" class="row-btn" onClick={() => navigate('/ls/designs/' + v.design.id)}>
          <span class="ellipsis">{t('dg.design.revision', { name: v.design.name, n: v.design.revision })}</span>
        </Button>
        <span class="xs muted">{t('dg.summary', { units: v.structure.experimentalUnits, conditions: v.structure.conditions.length, times: v.structure.times.length })}</span>
        <span class="xs faint">{t('dg.summary.readings', { n: v.structure.assigned })}</span>
        {v.design.pairs.length > 0 && <span class="xs faint">{t('dg.summary.pairs', { n: v.design.pairs.length })}</span>}
        <span class="xs faint">{t('dg.n_rule')}</span>
      </div>
    );
  }
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
  const graphs = useGraphs();
  const mine = p.def.id ? rel.data?.graphs[p.def.id] : undefined;
  const derived = mine?.derived || [];
  const changed = mine?.changedParents || [];
  const titleOf = (id: string) => graphs.data?.find((g) => g.id === id)?.title || '—';
  const go = (id: string) => (e: Event) => {
    e.preventDefault();
    navigate('/ls/graphs/' + id);
  };
  return (
    <div class="col gap3">
      {(!!p.def.derivedFrom?.length || derived.length > 0) && (
        <div class="col gap1 gb-lineage">
          <span class="section-title">{t('gb.lineage')}</span>
          {(p.def.derivedFrom || []).map((l) => (
            <a href={'/ls/graphs/' + l.graphId} class="small lineage-parent" onClick={go(l.graphId)} key={l.graphId}>
              <Icon name={l.relation === 'combine' ? 'layers' : 'history'} size="sm" /> {t('gb.derived_from', { name: l.title || '—' })} · {t('gb.relation.' + l.relation)} · {fmtDate(l.at, true)}
            </a>
          ))}
          {changed.map((id) => (
            <Notice kind="info" key={'c' + id}>
              {t('gc.parent_changed', { name: (p.def.derivedFrom || []).find((l) => l.graphId === id)?.title || titleOf(id) })}
            </Notice>
          ))}
          {derived.length > 0 && (
            <>
              <span class="xs faint">{t('gb.derived_n', { n: derived.length })}</span>
              <span class="xs muted">{t('gc.used_by')}</span>
              {derived.map((id) => (
                <a href={'/ls/graphs/' + id} class="small lineage-child" onClick={go(id)} key={id}>
                  <Icon name="chart" size="sm" /> {titleOf(id)}
                </a>
              ))}
            </>
          )}
        </div>
      )}
      {p.r.design && (
        <div class="col gap1">
          <span class="section-title">{t('dg.design')}</span>
          <a href={'/ls/designs/' + p.r.design.id} class="small" onClick={(e) => (e.preventDefault(), navigate('/ls/designs/' + p.r.design!.id))}>
            <Icon name="layers" size="sm" /> {t('dg.design.revision', { name: p.r.design.name, n: p.r.design.revision })}
          </a>
          <span class="xs faint">{t('dg.n_rule')}</span>
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
