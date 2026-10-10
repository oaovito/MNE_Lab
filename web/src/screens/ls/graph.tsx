// Graph view: build, inspect, compare and save a reproducible graph. The
// core computes and lays out the figure; changing the display never changes
// the data.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { get, post } from '../../lib/api';
import { statisticsHeaders } from '../../lib/analyses';
import { fmtDate, fmtNum, fmtQ, fmtTS } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { measurementName, PARAMS, useCycles, useFiles, warnText, WEIGHTINGS } from '../../lib/library';
import { navigate } from '../../lib/route';
import { openPanel, run, toast } from '../../lib/state';
import type { GraphDef, Series } from '../../lib/types';
import type { StatisticalAnalysis } from '../../lib/statistics';
import { FigureView, seriesLabel } from '../../plot/Figure';
import { defaultVisual, useRendered, useSize, type Rendered } from '../../plot/useRender';
import { Icon } from '../../ui/icons';
import { Badge, Button, confirmDialog, Empty, Field, Input, Notice, Seg, Select, Skeleton, Spinner, Switch, Tabs, useAsync } from '../../ui/kit';
import { presentGraphs } from './graphs';
import { MeasurementPicker } from './picker';

function newDef(q: URLSearchParams): GraphDef {
  const cycleId = q.get('cycle') || undefined;
  const kind = (q.get('kind') as GraphDef['kind']) || (cycleId ? 'parameter_time' : 'dls_distribution');
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

const norm = (d: GraphDef | null | undefined) => JSON.stringify(d ? { ...d, created: undefined, updated: undefined, schema: undefined } : null);

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
  return <Editor key={p.id} def={def} setDef={setDef} base={base} setBase={setBase} isNew={isNew} />;
}

function Editor(p: { def: GraphDef; setDef: (d: GraphDef) => void; base: GraphDef | null; setBase: (d: GraphDef) => void; isNew: boolean }) {
  const def = p.def;
  const set = (patch: Partial<GraphDef>) => p.setDef({ ...def, ...patch });
  const setVisual = (patch: Partial<GraphDef['visual']>) => set({ visual: { ...def.visual, ...patch } });
  const [ref, size] = useSize<HTMLDivElement>();
  const r = useRendered(def, size.w, size.h);
  const [busy, setBusy] = useState(false);
  const [picking, setPicking] = useState(false);
  const [tab, setTab] = useState<'series' | 'compare' | 'provenance' | 'notes'>('series');
  const [insp, setInsp] = useState(false);
  const dirty = p.isNew || norm(def) !== norm(p.base);
  const cycles = useCycles();
  const cycle = cycles.data?.find((c) => c.id === def.cycleId);
  const analysis = useAsync(() => def.analysisId ? get<StatisticalAnalysis>('/api/statistics/' + def.analysisId,{headers:statisticsHeaders()}) : Promise.resolve(null), [def.analysisId]);
  const series = r.data?.series || [];

  // Leaving with unsaved changes asks first.
  useEffect(() => {
    const h = (e: BeforeUnloadEvent) => {
      if (dirty) e.preventDefault();
    };
    addEventListener('beforeunload', h);
    return () => removeEventListener('beforeunload', h);
  }, [dirty]);

  const save = async (asNew = false) => {
    setBusy(true);
    const body = asNew ? { ...def, id: undefined, title: def.title ? t('graph.copy_of', { name: def.title }) : '' } : def;
    const g = await run(() => post<GraphDef>('/api/graphs', body));
    setBusy(false);
    if (!g) return;
    toast('success', t('graph.saved'));
    p.setBase(g);
    p.setDef(g);
    if (p.isNew || asNew) navigate('/ls/graphs/' + g.id, true);
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

  const setStyle = (id: string, st: Partial<{ label: string; color: string; hidden: boolean }>) => {
    const cur = def.series?.[id] || {};
    const next = { ...cur, ...st };
    if (!next.label) delete next.label;
    if (!next.color) delete next.color;
    if (!next.hidden) delete next.hidden;
    set({ series: { ...(def.series || {}), [id]: next } });
  };

  const cycleKind = def.kind === 'parameter_time' || def.kind === 'dls_by_time';
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
        <span class="spacer" />
        <Button size="sm" kind="ghost" icon="info" class="narrow-only" selected={insp} onClick={() => setInsp(!insp)}>
          {t('graph.details')}
        </Button>
        <Button size="sm" icon="present" disabled={!r.data} onClick={async () => { const id = await needSave(); if (id) presentGraphs([id]); }}>
          {t('pres.start')}
        </Button>
        <Button size="sm" icon="download" disabled={!r.data} onClick={async () => { const id = await needSave(); if (id) openPanel('export', { items: [{ kind: 'graph', id }] }); }}>
          {t('ui.export')}
        </Button>
        {!p.isNew && (
          <Button size="sm" kind="ghost" icon="copy" tip={t('graph.save_new')} disabled={busy} onClick={() => save(true)} />
        )}
        <Button size="sm" kind="primary" icon="save" busy={busy} disabled={!dirty || !!r.error} onClick={() => save()}>
          {p.isNew ? t('graph.save_library') : t('ui.save')}
        </Button>
      </div>
      <div class="split graph">
        {/* configuration */}
        <div class="side-panel">
          <div class="card pad group">
            <span class="section-title">{t('graph.type')}</span>
            <Select
              value={def.kind}
              label={t('graph.type')}
              onValue={(kind) => set({ kind: kind as GraphDef['kind'] })}
              options={[
                ...(def.analysisId ? [{ value: 'statistical_groups', label: t('graph.kind.statistical_groups') }] : [{ value: 'dls_distribution', label: t('graph.kind.dls_distribution') }]),
                ...(!def.analysisId ? [{ value: 'parameter_time', label: t('graph.kind.parameter_time'), disabled: !def.cycleId },
                { value: 'dls_by_time', label: t('graph.kind.dls_by_time'), disabled: !def.cycleId }] : []),
              ]}
            />
            {!def.cycleId && <span class="xs faint">{t('graph.cycle_kinds_hint')}</span>}
            {!def.cycleId && def.measurements.length > 0 && (
              <Button size="sm" icon="cycle" class="row-btn" onClick={() => openPanel('cycle-wizard', { measurements: def.measurements })}>
                {t('files.cycle')}
              </Button>
            )}
            {def.kind === 'statistical_groups' && <Field label={t('stat.error_bars')}><Select label={t('stat.error_bars')} value={def.errorBars||'sd'} onValue={(errorBars)=>set({errorBars:errorBars as 'sd'|'sem'|'ci'})} options={[{value:'sd',label:'SD'},{value:'sem',label:'SEM'},{value:'ci',label:t('stat.confidence_interval')}]}/><Button size="sm" onClick={()=>navigate('/ls/statistics/'+def.analysisId)}>{t('stat.title')}</Button></Field>}
            {def.kind === 'statistical_groups' && analysis.data && <div class="group">
              <Field label={t('stat.annotation_style')}><Select label={t('stat.annotation_style')} value={def.annotationStyle||'exact'} onValue={(value)=>set({annotationStyle:value as 'exact'|'stars'})} options={[{value:'exact',label:t('stat.annotation_exact')},{value:'stars',label:t('stat.annotation_stars')}]}/></Field>
              <span class="small muted">{t('stat.annotations_help')}</span>
              {analysis.data.results.comparisons.map(c=><label class="row small" key={c.id}>
                <input type="checkbox" checked={(def.annotations||[]).includes(c.id)} disabled={!(def.annotations||[]).includes(c.id)&&(def.annotations||[]).length>=8} onChange={e=>set({annotations:e.currentTarget.checked?[...(def.annotations||[]),c.id]:(def.annotations||[]).filter(id=>id!==c.id)})}/>
                <span>{c.contrast}{c.context?' · '+c.context:''} · p(adj)={c.adjustedP===0?t('stat.p_underflow'):String(c.adjustedP)}</span>
              </label>)}
              {def.annotationStyle==='stars'&&<span class="xs faint">{t('stat.star_thresholds')}</span>}
            </div>}
            {cycleKind && (
              <Field label={t('meta.cycle')}>
                <Select value={def.cycleId || ''} onValue={(cycleId) => set({ cycleId })} options={(cycles.data || []).map((c) => ({ value: c.id!, label: c.config.name }))} />
              </Field>
            )}
            {def.kind === 'parameter_time' && (
              <Field label={t('meta.parameter')}>
                <Select value={def.param || 'effective_diameter'} onValue={(param) => set({ param })} options={PARAMS.map((k) => ({ value: k, label: t('param.' + k) }))} />
              </Field>
            )}
            {(def.kind === 'dls_distribution' || def.kind === 'dls_by_time') && (
              <>
                <Field label={t('meta.weighting')}>
                  <Seg
                    size="sm"
                    value={def.weighting || 'intensity'}
                    onValue={(weighting) => set({ weighting })}
                    options={WEIGHTINGS.map((w) => ({ value: w, label: t('axis.' + w), tip: r.data?.weightings && !r.data.weightings.includes(w) ? t('graph.weighting_absent') : undefined }))}
                  />
                </Field>
                <Field label={t('graph.xscale')}>
                  <Seg
                    size="sm"
                    value={def.xScale || 'auto'}
                    onValue={(xScale) => set({ xScale })}
                    options={[
                      { value: 'auto', label: t('graph.scale.auto') },
                      { value: 'log', label: t('graph.scale.log') },
                      { value: 'linear', label: t('graph.scale.linear') },
                    ]}
                  />
                </Field>
              </>
            )}
          </div>
          {def.kind === 'dls_distribution' && (
            <div class="card pad group">
              <div class="row">
                <span class="section-title grow">{t('graph.measurements', { n: def.measurements.length })}</span>
                <Button size="sm" kind="ghost" icon="plus" onClick={() => setPicking(true)}>
                  {t('ui.add')}
                </Button>
              </div>
              <MeasurementList ids={def.measurements} onRemove={(id) => set({ measurements: def.measurements.filter((x) => x !== id) })} />
            </div>
          )}
          {cycleKind && cycle && (
            <div class="card pad group">
              <span class="section-title">{t('meta.cycle')}</span>
              <Button size="sm" kind="ghost" icon="cycle" class="row-btn" onClick={() => navigate('/ls/cycles/' + cycle.id)}>
                <span class="ellipsis">{cycle.config.name}</span>
              </Button>
              <span class="xs faint">{t('graph.cycle_measurements', { n: cycle.measurements.length })}</span>
            </div>
          )}
          <div class="card pad group">
            <span class="section-title">{t('graph.display')}</span>
            <Switch checked={def.visual.legend} onChange={(legend) => setVisual({ legend })} label={t('graph.legend')} />
            <Switch checked={def.visual.metadata} onChange={(metadata) => setVisual({ metadata })} label={t('graph.metadata')} />
            <Switch checked={def.visual.grid} onChange={(grid) => setVisual({ grid })} label={t('graph.grid')} />
            <Switch checked={def.visual.points} onChange={(points) => setVisual({ points })} label={t('graph.points')} />
            <Range label={t('graph.line_width')} value={def.visual.lineWidth} min={0.75} max={4} step={0.25} onValue={(lineWidth) => setVisual({ lineWidth })} fmt={(v) => fmtNum(v, 2) + ' pt'} />
            <Range label={t('graph.text_size')} value={def.visual.fontScale} min={0.8} max={1.6} step={0.05} onValue={(fontScale) => setVisual({ fontScale })} fmt={(v) => Math.round(v * 100) + '%'} />
          </div>
        </div>

        {/* figure */}
        <div class="col gap2" style={{ minHeight: 0, minWidth: 0 }}>
          <div class="figure-wrap" ref={ref}>
            {r.data && size.w > 0 && (
              <FigureView
                figure={r.data.figure}
                series={series}
                xUnit={r.data.x.unit}
                yUnit={r.data.y.unit}
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
                  <span class="xs" style={{ color: '#7c8696', padding: '6px 4px' }}>
                    <Icon name="zoom" size="sm" /> {t('graph.zoom_hint')}
                  </span>
                )}
              </div>
            )}
            {!r.data && !r.error && (
              <div class="state">
                <Spinner label={t('graph.rendering')} />
              </div>
            )}
            {r.error && (
              <div class="state">
                <Empty icon="chart" title={t('graph.cannot')} body={errText(r.error)}>
                  {def.kind === 'dls_distribution' && (
                    <Button icon="plus" onClick={() => setPicking(true)}>
                      {t('graph.add_measurements')}
                    </Button>
                  )}
                </Empty>
              </div>
            )}
            {r.loading && r.data && (
              <span class="busy">
                <span class="spin" style={{ width: 14, height: 14 }} />
              </span>
            )}
          </div>
          {!!r.data?.warnings?.length && (
            <Notice kind="warning">
              {r.data.warnings.map((w) => (
                <div>{warnText(w)}</div>
              ))}
            </Notice>
          )}
        </div>

        {/* inspector */}
        <div class={`panel inspector ${insp ? 'open' : ''}`}>
          <Tabs
            value={tab}
            onValue={setTab}
            tabs={[
              { value: 'series', label: t('graph.t.series') },
              { value: 'compare', label: t('graph.t.compare') },
              { value: 'provenance', label: t('graph.t.provenance') },
              { value: 'notes', label: t('graph.t.notes') },
            ]}
          />
          <div class="panel-body" style={{ padding: 'var(--s3)' }}>
            {!r.data ? (
              <Skeleton h={80} />
            ) : tab === 'series' ? (
              <SeriesPanel series={series} def={def} setStyle={setStyle} />
            ) : tab === 'compare' ? (
              <ComparePanel r={r.data} />
            ) : tab === 'provenance' ? (
              <ProvenancePanel r={r.data} def={def} />
            ) : (
              <NotesPanel def={def} set={set} />
            )}
          </div>
        </div>
      </div>
      {picking && (
        <MeasurementPicker
          title={t('graph.add_measurements')}
          initial={def.measurements}
          onClose={() => setPicking(false)}
          onPick={(ids) => {
            set({ measurements: ids });
            setPicking(false);
          }}
        />
      )}
    </>
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

function MeasurementList(p: { ids: string[]; onRemove: (id: string) => void }) {
  const files = useFiles();
  const byId = useMemo(() => {
    const m = new Map<string, { name: string; file: string; fileId: string }>();
    for (const f of files.data || []) for (const x of f.items || []) m.set(x.id, { name: measurementName(x, f), file: f.name, fileId: f.id });
    return m;
  }, [files.data]);
  if (!p.ids.length) return <span class="small faint">{t('graph.no_measurements')}</span>;
  return (
    <div class="col gap1">
      {p.ids.map((id) => {
        const m = byId.get(id);
        return (
          <div class="series-row">
            <Icon name="file" size="sm" />
            <span class="grow col" style={{ gap: 0, minWidth: 0 }}>
              <span class="ellipsis">{m?.name || t('graph.missing')}</span>
              {m && (
                <a
                  href={'/ls/files?file=' + m.fileId}
                  class="xs faint ellipsis"
                  onClick={(e) => {
                    e.preventDefault();
                    navigate('/ls/files?file=' + m.fileId);
                  }}
                >
                  {m.file}
                </a>
              )}
            </span>
            <Button size="sm" kind="ghost" icon="x" tip={t('ui.remove')} onClick={() => p.onRemove(id)} />
          </div>
        );
      })}
    </div>
  );
}

function SeriesPanel(p: { series: Series[]; def: GraphDef; setStyle: (id: string, st: any) => void }) {
  const [editing, setEditing] = useState<string | null>(null);
  const [val, setVal] = useState('');
  if (!p.series.length) return <span class="small faint">{t('graph.no_series')}</span>;
  return (
    <div class="series-list">
      <span class="xs faint" style={{ marginBottom: 6 }}>
        {t('graph.series.hint')}
      </span>
      {p.series.map((se) => {
        const st = p.def.series?.[se.id] || {};
        const hidden = !!st.hidden;
        return (
          <div class={`series-row ${hidden ? 'off' : ''}`}>
            <label class="swatch" style={{ '--c': se.color || '#0072B2' } as any} data-tip={t('graph.series.color')}>
              <input type="color" value={se.color || '#0072B2'} onInput={(e) => p.setStyle(se.id, { color: (e.target as HTMLInputElement).value.toUpperCase() })} aria-label={t('graph.series.color')} />
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
              <span class="grow ellipsis" onDblClick={() => (setEditing(se.id), setVal(st.label || seriesLabel(se.label)))} title={seriesLabel(se.label)}>
                {seriesLabel(se.label)}
              </span>
            )}
            <Button size="sm" kind="ghost" icon="pencil" tip={t('graph.series.rename')} onClick={() => (setEditing(se.id), setVal(st.label || seriesLabel(se.label)))} />
            <Button size="sm" kind="ghost" icon={hidden ? 'eyeOff' : 'eye'} tip={hidden ? t('graph.series.show') : t('graph.series.hide')} onClick={() => p.setStyle(se.id, { hidden: !hidden })} />
          </div>
        );
      })}
      {Object.keys(p.def.series || {}).length > 0 && (
        <Button size="sm" kind="ghost" icon="reset" class="" onClick={() => Object.keys(p.def.series || {}).forEach((id) => p.setStyle(id, { label: '', color: '', hidden: false }))}>
          {t('graph.series.reset')}
        </Button>
      )}
    </div>
  );
}

function ComparePanel(p: { r: Rendered }) {
  const rows = (p.r.series || []).filter((s) => s.meta);
  if (!rows.length) return <span class="small faint">{t('graph.compare.none')}</span>;
  const cell = (s: Series, k: string) => {
    const raw = s.meta?.[k];
    if (raw === undefined) return <span class="faint">—</span>;
    return fmtQ({ value: parseFloat(raw.replace(',', '.')), raw, unit: s.meta?.[k + '.unit'] });
  };
  return (
    <div class="col gap3">
      {rows.map((s) => (
        <div class="meas" style={{ gap: 6 }}>
          <div class="row gap2">
            <span class="dot" style={{ color: s.color }} />
            <b class="small ellipsis grow">{seriesLabel(s.label)}</b>
          </div>
          {s.meta?.measuredAt && (
            <span class="xs faint">
              {fmtTS({ time: s.meta.measuredAt, raw: '', tzKnown: s.meta['measuredAt.tz'] !== 'unknown', source: 'file' })}
            </span>
          )}
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

const transformKey = (s: string) => (s.startsWith('none') ? 'prov.tr.none' : s.startsWith('replicates') ? 'prov.tr.grouped' : '');

function ProvenancePanel(p: { r: Rendered; def: GraphDef }) {
  const pv = p.r.provenance;
  return (
    <div class="col gap3">
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
          {(pv.sources || []).map((s) => (
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
