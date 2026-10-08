// Cycle view: the plan, every association with its reason, the statistics
// per point and the graphs made from the cycle. Decisions here are stored in
// the cycle; the measurements and their real timestamps never change.
import { useMemo, useRef, useState } from 'preact/hooks';
import { get, post } from '../../lib/api';
import { fmtNum, fmtTS } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { measurementName, PARAMS, useFiles, useGraphs, useRelations, useRev } from '../../lib/library';
import { navigate } from '../../lib/route';
import { openPanel, run, toast } from '../../lib/state';
import type { Assignment, CycleView as View, MeasurementSummary, PointResult } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Badge, Button, confirmDialog, Empty, Menu, MenuButton, Notice, Seg, Skeleton, useAsync, type MenuEntry } from '../../ui/kit';
import { presentGraphs } from './graphs';
import { MeasurementPicker } from './picker';

export function CycleView(p: { id: string }) {
  const rev = useRev();
  const v = useAsync(() => get<View>('/api/cycles/' + p.id), [p.id, rev]);
  if (v.error)
    return (
      <div class="panel" style={{ flex: 1 }}>
        <Empty icon="cycle" title={t('cycle.not_found')} body={errText(v.error)}>
          <Button onClick={() => navigate('/ls/cycles')}>{t('cycle.back')}</Button>
        </Empty>
      </div>
    );
  if (!v.data) return <Skeleton h="100%" r="var(--r-lg)" />;
  return <Detail v={v.data} set={v.set} />;
}

function Detail(p: { v: View; set: (v: View) => void }) {
  const v = p.v;
  const c = v.config;
  const graphs = useGraphs();
  const files = useFiles();
  const rel = useRelations();
  const [picking, setPicking] = useState(false);
  const [param, setParam] = useState(c.params[0] || 'effective_diameter');
  const items = useMemo(() => new Map((v.items || []).map((m) => [m.id, m])), [v.items]);
  const as = v.assignments || [];
  const pts = v.points || [];
  const review = as.filter((a) => a.status === 'needs_confirmation' || a.status === 'unassigned');
  const cycleGraphs = (graphs.data || []).filter((g) => g.cycleId === v.id);
  const name = (id: string) => {
    const m = items.get(id);
    return m ? measurementName(m) : '…';
  };

  const assign = async (a: Assignment, point: number) => {
    const r = await run(() => post<View>(`/api/cycles/${v.id}/assign`, { measurement: a.measurementId, point, replicate: point === a.point ? a.replicate || 0 : 0 }));
    if (r) p.set(r);
  };
  const clear = async (a: Assignment) => {
    const r = await run(() => post<View>(`/api/cycles/${v.id}/assign`, { measurement: a.measurementId, clear: true }));
    if (r) p.set(r);
  };
  const addMeasurements = async (ids: string[]) => {
    setPicking(false);
    const doc = { ...v, measurements: [...new Set([...v.measurements, ...ids])] } as any;
    for (const k of ['points', 'assignments', 'items', 'series', 'files']) delete doc[k];
    const saved = await run(() => post('/api/cycles', doc));
    if (saved) toast('success', t('cycle.added', { n: ids.filter((id) => !v.measurements.includes(id)).length }));
  };
  const trash = async () => {
    const used = rel.data?.cycles[v.id!]?.graphs?.length || 0;
    if (used && !(await confirmDialog({ title: t('cycle.trash.q'), body: t('cycle.trash.used', { n: used }), confirm: t('ui.trash'), danger: true }))) return;
    const ok = await run(() => post(`/api/cycles/${v.id}/trash`, { trashed: true }));
    if (ok === undefined) return;
    navigate('/ls/cycles');
    toast('info', t('cycle.trashed'), { label: t('ui.undo'), run: () => run(() => post(`/api/cycles/${v.id}/trash`, { trashed: false })) });
  };
  const graphItems: MenuEntry[] = [
    { heading: t('graph.kind.parameter_time') },
    ...c.params.map((k) => ({ label: t('param.' + k), icon: 'chart' as const, run: () => navigate(`/ls/graphs/new?cycle=${v.id}&kind=parameter_time&param=${k}`) })),
    { sep: true },
    { label: t('graph.kind.dls_by_time'), icon: 'spline', run: () => navigate(`/ls/graphs/new?cycle=${v.id}&kind=dls_by_time`) },
  ];
  const auto = as.filter((a) => a.status === 'auto').length;
  const confirmed = as.filter((a) => a.status === 'confirmed').length;
  const filled = new Set(as.filter((a) => a.point >= 0 && (a.status === 'auto' || a.status === 'confirmed')).map((a) => a.point));

  return (
    <>
      <div class="page-head" style={{ minHeight: 0 }}>
        <Button kind="ghost" size="sm" icon="back" onClick={() => navigate('/ls/cycles')}>
          {t('ls.tab.cycles')}
        </Button>
        <div class="col" style={{ gap: 2, minWidth: 0 }}>
          <h1 class="ellipsis" style={{ fontSize: 'var(--fs-lg)' }}>
            {c.name}
          </h1>
          <div class="row gap1 wrap">
            <Badge kind="accent" icon="clock">
              {t('cycles.every', { n: c.interval, unit: t('unit.' + c.unit) })}
            </Badge>
            <Badge>{t('cycles.duration', { n: c.duration, unit: t('unit.' + c.unit) })}</Badge>
            {c.sampleId && <Badge icon="flask">{c.sampleId}</Badge>}
            {c.experiment && <Badge icon="microscope">{c.experiment}</Badge>}
          </div>
        </div>
        <span class="spacer" />
        <Button size="sm" icon="pencil" onClick={() => openPanel('cycle-wizard', { measurements: v.measurements, edit: stripView(v) })}>
          {t('ui.edit')}
        </Button>
        <Button size="sm" icon="plus" onClick={() => setPicking(true)}>
          {t('cycle.add_measurements')}
        </Button>
        <Button size="sm" icon="present" disabled={!cycleGraphs.length} tip={cycleGraphs.length ? undefined : t('cycle.present_none')} onClick={() => presentGraphs(cycleGraphs.map((g) => g.id!))}>
          {t('pres.start')}
        </Button>
        <Button size="sm" icon="download" onClick={() => openPanel('export', { items: [{ kind: 'cycle', id: v.id }] })}>
          {t('ui.export')}
        </Button>
        <MenuButton size="sm" kind="primary" icon="chart" trail="down" items={graphItems} align="end">
          {t('cycle.create_graph')}
        </MenuButton>
        <Button size="sm" kind="ghost" icon="trash" tip={t('ui.trash')} onClick={trash} />
      </div>

      <div class="split cycle">
        <div class="col gap3" style={{ minHeight: 0, minWidth: 0 }}>
          <div class="card pad col gap3">
            <div class="row gap2 wrap small">
              <Badge kind="accent">{t('cycles.points', { n: pts.length })}</Badge>
              <Badge kind="success">{t('cycles.auto_assigned', { n: auto })}</Badge>
              {confirmed > 0 && <Badge kind="info">{t('cycle.confirmed_n', { n: confirmed })}</Badge>}
              {review.length > 0 && <Badge kind="warning">{t('cycles.need_review', { n: review.length })}</Badge>}
              <span class="spacer" />
              <span class="xs faint">{t('cycle.rule')}</span>
            </div>
            <div class="timeline" aria-label={t('cycle.timeline')}>
              {pts.map((pt) => (
                <span class={filled.has(pt.index) ? '' : 'miss'} data-tip={t('point.' + pt.unit, { n: pt.offset })} />
              ))}
            </div>
          </div>
          <div class="panel" style={{ flex: 1, minHeight: 0 }}>
            <div class="panel-body" style={{ padding: 'var(--s3)' }}>
              {!v.measurements.length ? (
                <Empty icon="files" title={t('cycle.no_measurements')} body={t('cycle.no_measurements.d')}>
                  <Button kind="primary" icon="plus" onClick={() => setPicking(true)}>
                    {t('cycle.add_measurements')}
                  </Button>
                </Empty>
              ) : (
                <div class="points">
                  {pts.map((pt) => {
                    const here = as.filter((a) => a.point === pt.index && a.status !== 'unassigned').sort((x, y) => (x.replicate || 0) - (y.replicate || 0));
                    return (
                      <div class={`point ${here.length ? '' : 'miss'}`}>
                        <div class="when">
                          <b>{t('point.' + pt.unit, { n: pt.offset })}</b>
                          <span>{fmtTS({ time: pt.target, raw: '', tzKnown: c.startTzKnown, source: 'file' })}</span>
                        </div>
                        <div class="row wrap gap1">
                          {here.length ? (
                            here.map((a) => <Chip a={a} m={items.get(a.measurementId)} points={pts.length} onAssign={(n) => assign(a, n)} onClear={() => clear(a)} pointLabel={(i) => t('point.' + pts[i].unit, { n: pts[i].offset })} />)
                          ) : (
                            <span class="xs faint">{t('cycle.status.missing')}</span>
                          )}
                        </div>
                        <span class="xs faint">{here.length ? t('cycles.n_meas', { n: here.length }) : ''}</span>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>

        <div class="side-panel">
          {review.length > 0 && (
            <div class="card pad group">
              <span class="section-title">{t('cycle.review')}</span>
              {review.map((a) => (
                <div class="col gap1" style={{ paddingBottom: 'var(--s2)', borderBottom: '1px solid var(--border)' }}>
                  <div class="row gap2">
                    <Icon name={a.status === 'unassigned' ? 'ban' : 'warning'} size="sm" />
                    <span class="ellipsis grow small">{name(a.measurementId)}</span>
                    <Chip a={a} m={items.get(a.measurementId)} points={pts.length} onAssign={(n) => assign(a, n)} onClear={() => clear(a)} pointLabel={(i) => t('point.' + pts[i].unit, { n: pts[i].offset })} compact />
                  </div>
                  <span class="xs muted">{a.reason ? t('reason.' + a.reason) : t('cycle.status.' + a.status)}</span>
                  {items.get(a.measurementId)?.measuredAt && <span class="xs faint">{t('cycle.measured', { when: fmtTS(items.get(a.measurementId)!.measuredAt) })}</span>}
                </div>
              ))}
            </div>
          )}
          <div class="card pad group">
            <div class="row">
              <span class="section-title grow">{t('cycle.results')}</span>
              <Icon name="sigma" size="sm" />
            </div>
            {c.params.length > 1 && <Seg size="sm" value={param} onValue={setParam} options={c.params.filter((k) => (PARAMS as readonly string[]).includes(k)).map((k) => ({ value: k, label: t('param.short.' + k) }))} />}
            <Stats rows={v.series?.[param] || []} unit={unitOf(v.items, param)} pointUnit={c.unit} />
            <span class="xs faint">{t('stats.definition')}</span>
          </div>
          <div class="card pad group">
            <span class="section-title">{t('cycle.graphs')}</span>
            {cycleGraphs.length ? (
              cycleGraphs.map((g) => (
                <Button size="sm" kind="ghost" icon="chart" class="row-btn" onClick={() => navigate('/ls/graphs/' + g.id)}>
                  <span class="ellipsis">{g.title || t('graph.kind.' + g.kind)}</span>
                </Button>
              ))
            ) : (
              <span class="xs faint">{t('cycle.graphs.none')}</span>
            )}
          </div>
          <div class="card pad group">
            <span class="section-title">{t('cycle.files', { n: v.files?.length || 0 })}</span>
            {(v.files || []).slice(0, 12).map((id) => (
              <Button size="sm" kind="ghost" icon="file" class="row-btn" onClick={() => navigate('/ls/files?file=' + id)}>
                <span class="ellipsis">{files.data?.find((f) => f.id === id)?.name || '…'}</span>
              </Button>
            ))}
            {(v.files?.length || 0) > 12 && <span class="xs faint">+{(v.files?.length || 0) - 12}</span>}
          </div>
          {!c.startTzKnown && <Notice icon="clock">{t('cycle.tz_note')}</Notice>}
        </div>
      </div>
      {picking && <MeasurementPicker title={t('cycle.add_measurements')} initial={v.measurements} onClose={() => setPicking(false)} onPick={addMeasurements} confirm={t('ui.add')} />}
    </>
  );
}

/** Chip shows one association and the decisions available for it. */
function Chip(p: { a: Assignment; m?: MeasurementSummary; points: number; onAssign: (point: number) => void; onClear: () => void; pointLabel: (i: number) => string; compact?: boolean }) {
  const ref = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const a = p.a;
  const items: MenuEntry[] = [];
  if (a.status === 'needs_confirmation' && a.point >= 0) items.push({ label: t('cycle.confirm_here', { point: p.pointLabel(a.point) }), icon: 'check', run: () => p.onAssign(a.point) });
  items.push({ heading: t('cycle.move_to') });
  for (let i = 0; i < p.points; i++) items.push({ label: p.pointLabel(i), run: () => p.onAssign(i), checked: a.point === i && a.status !== 'unassigned' });
  items.push({ sep: true });
  if (a.reason !== 'cycle.excluded') items.push({ label: t('cycle.exclude'), icon: 'ban', run: () => p.onAssign(-1) });
  if (a.status === 'confirmed' || a.reason === 'cycle.excluded') items.push({ label: t('cycle.use_rule'), icon: 'reset', run: p.onClear });
  const tip = a.reason ? t('reason.' + a.reason) : t('cycle.status.' + a.status) + (a.replicate ? ' · ' + t('cycle.replicate', { n: a.replicate }) : '');
  return (
    <>
      {p.compact ? (
        <Button size="sm" btnRef={ref} trail="down" onClick={() => setOpen(!open)}>
          {t('cycle.decide')}
        </Button>
      ) : (
        <button ref={ref} type="button" class={`assign ${a.status}`} data-tip={tip} onClick={() => setOpen(!open)} aria-haspopup="menu">
          <span class="ellipsis">{p.m ? measurementName(p.m) : '…'}</span>
          {a.replicate ? <span class="xs faint">#{a.replicate}</span> : null}
          {a.status === 'needs_confirmation' ? <Icon name="warning" size="sm" /> : a.status === 'confirmed' ? <Icon name="check" size="sm" /> : <Icon name="down" size="sm" />}
        </button>
      )}
      {open && ref.current && <Menu anchor={ref.current} items={items} onClose={() => setOpen(false)} />}
    </>
  );
}

function Stats(p: { rows: PointResult[]; unit?: string; pointUnit: string }) {
  if (!p.rows.length) return <span class="xs faint">{t('cycle.results.none')}</span>;
  return (
    <table class="table compact">
      <thead>
        <tr>
          <th>{t('col.point')}</th>
          <th class="num">{t('col.mean_sd')}</th>
          <th class="num">n</th>
        </tr>
      </thead>
      <tbody>
        {p.rows.map((r) => (
          <tr>
            <td>{t('point.' + p.pointUnit, { n: r.offset })}</td>
            <td class="num">{r.missing || r.summary.mean === undefined ? <span class="faint">{t('cycle.status.missing')}</span> : `${fmtNum(r.summary.mean)}${r.summary.sd !== undefined ? ' ± ' + fmtNum(r.summary.sd) : ''}${p.unit ? ' ' + p.unit : ''}`}</td>
            <td class="num">{r.summary.n}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function unitOf(items: MeasurementSummary[] | null, param: string) {
  for (const m of items || []) if (m.params[param]?.unit) return m.params[param].unit;
  return undefined;
}


function stripView(v: View) {
  const { points, assignments, items, series, files, ...doc } = v;
  return doc;
}
