// Experimental designs: which independent unit (preparation) each
// measurement belongs to, under which condition and at which time point,
// and whether it is the unit's observation or a technical repeat. A design
// only references measurements; every save is a new revision whose changes
// are kept as its history. Name-based proposals are previewed and applied
// only after the person confirms them.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { get, post } from '../../lib/api';
import { statisticsHeaders } from '../../lib/analyses';
import { fmtTS } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { allMeasurements, measurementName, UNITS, useFiles, useRev } from '../../lib/library';
import { navigate } from '../../lib/route';
import { app, run } from '../../lib/state';
import { useStore } from '../../lib/store';
import type { Design, DesignAssignment, DesignChange, DesignCondition, DesignStructure, DesignSuggestion, DesignTime, DesignView } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Badge, Button, Check, Empty, Input, Modal, Notice, Skeleton, useAsync } from '../../ui/kit';
import { MeasurementPicker } from './picker';

type Unit = { id: string; label: string };
type Draft = Pick<Design, 'name' | 'notes' | 'units' | 'conditions' | 'times' | 'assignments' | 'pairs'> & { reason?: string; source?: 'user' | 'suggestion' };

const headers = () => statisticsHeaders();
const emptyDraft = (): Draft => ({ name: '', notes: '', units: [], conditions: [], times: [], assignments: [], pairs: [] });

export function DesignModule(p: { id?: string }) {
  const s = useStore(app, (x) => x.s);
  const key = `${s?.account?.id}/${s?.profile?.id}/${p.id || 'library'}`;
  if (!p.id) return <DesignLibrary key={key} />;
  return <DesignEditor key={key} id={p.id === 'new' ? undefined : p.id} />;
}

export const structureText = (s: DesignStructure) => t('dsg.detected.' + s.detected);

function DesignLibrary() {
  const rev = useRev();
  const list = useAsync(() => get<DesignView[]>('/api/designs', { headers: headers() }), [rev]);
  const [q, setQ] = useState('');
  const shown = (list.data || []).filter((v) => !q || v.design.name.toLowerCase().includes(q.toLowerCase()));
  return (
    <>
      <div class="toolbar">
        <div style={{ width: 'min(300px, 36vw)' }}>
          <Input icon="search" size="sm" value={q} onValue={setQ} placeholder={t('dsg.search')} aria-label={t('dsg.search')} />
        </div>
        <span class="spacer" />
        <Button size="sm" kind="primary" icon="plus" onClick={() => navigate('/ls/designs/new')}>
          {t('dsg.new')}
        </Button>
      </div>
      <div class="panel" style={{ flex: 1 }}>
        <div class="panel-body" style={{ padding: 'var(--s4)' }}>
          {list.error ? (
            <Empty icon="alert" title={t('err.title')} body={errText(list.error)} />
          ) : !list.data ? (
            <div class="cards">
              {[1, 2].map(() => (
                <Skeleton h={130} r="var(--r-lg)" />
              ))}
            </div>
          ) : !shown.length ? (
            <Empty icon="layers" title={t('dsg.empty')} body={t('dsg.empty.d')}>
              <Button kind="primary" icon="plus" onClick={() => navigate('/ls/designs/new')}>
                {t('dsg.new')}
              </Button>
            </Empty>
          ) : (
            <div class="cards">
              {shown.map((v) => (
                <div class="gcard design-card" role="button" tabIndex={0} onClick={() => navigate('/ls/designs/' + v.design.id)} onKeyDown={(e) => e.key === 'Enter' && navigate('/ls/designs/' + v.design.id)}>
                  <div class="body">
                    <span class="title ellipsis">{v.design.name}</span>
                    <div class="row gap1 wrap">
                      <Badge kind="accent" icon="users">
                        {t('dsg.n_units', { n: v.structure.experimentalUnits })}
                      </Badge>
                      <Badge>{t('dsg.n_measurements', { n: v.structure.measurements })}</Badge>
                      {v.heads > 1 && <Badge kind="warning">{t('dsg.diverged')}</Badge>}
                    </div>
                    <span class="meta">{structureText(v.structure)}</span>
                    <span class="meta row gap1">
                      <Icon name="history" size="sm" />
                      {t('dsg.revision_n', { n: v.design.revision })} · {fmtTS({ time: v.design.created, raw: '', tzKnown: true, source: 'file' })}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function toDraft(d: Design): Draft {
  return { name: d.name, notes: d.notes || '', units: d.units.map((u) => ({ ...u })), conditions: d.conditions.map((c) => ({ ...c })), times: d.times.map((x) => ({ ...x })), assignments: d.assignments.map((a) => ({ ...a })), pairs: d.pairs.map((p) => ({ ...p })) };
}

function DesignEditor(p: { id?: string }) {
  const rev = useRev();
  const saved = useAsync(() => (p.id ? get<DesignView>('/api/designs/' + p.id, { headers: headers() }) : Promise.resolve(null)), [p.id, rev]);
  const units = useAsync(() => get<Unit[]>('/api/experimental-units', { headers: headers() }), [rev]);
  const files = useFiles();
  const [draft, setDraft] = useState<Draft | null>(p.id ? null : emptyDraft());
  const [dirty, setDirty] = useState(false);
  const [review, setReview] = useState<DesignStructure | null>(null);
  const [reviewErr, setReviewErr] = useState('');
  const [picking, setPicking] = useState(false);
  const [proposals, setProposals] = useState<DesignSuggestion[] | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (saved.data && !dirty) setDraft(toDraft(saved.data.design));
  }, [saved.data]);

  const change = (patch: Partial<Draft>) => {
    setDraft((d) => ({ ...(d as Draft), ...patch }));
    setDirty(true);
  };

  useEffect(() => {
    if (!draft) return;
    const h = setTimeout(() => {
      post<DesignStructure>('/api/designs/review', { design: draft }, { headers: headers() })
        .then((s) => (setReview(s), setReviewErr('')))
        .catch((e) => setReviewErr(e.code || 'app.internal_error'));
    }, 200);
    return () => clearTimeout(h);
  }, [draft]);

  const byId = useMemo(() => allMeasurements(files.data || []), [files.data]);
  const unitLabel = (id?: string) => (id ? units.data?.find((u) => u.id === id)?.label || saved.data?.unitLabels[id] || draft?.units.find((u) => u.id === id)?.label || '?' : '—');
  const mName = (id: string) => {
    const e = byId.get(id);
    return e ? measurementName(e.m, e.f) : id;
  };

  if (saved.error) return <Notice kind="danger">{errText(saved.error)}</Notice>;
  if (!draft || (p.id && !saved.data)) return <Skeleton h={240} />;

  const save = async () => {
    setSaving(true);
    const v = await run(() => post<DesignView>('/api/designs', { design: { ...draft, module: 'lightscattering', source: draft.source || 'user' }, previousId: p.id || '' }, { headers: headers() }));
    setSaving(false);
    if (v) {
      setDirty(false);
      navigate('/ls/designs/' + v.design.id, true);
    }
  };
  const addUnit = async (label: string) => {
    const u = await run(() => post<Unit>('/api/experimental-units', { label }, { headers: headers() }));
    if (u) {
      units.reload();
      change({ units: [...draft.units, { id: u.id, label: u.label, kind: 'independent_preparation' }] });
    }
  };
  const suggest = async () => {
    const s = await run(() => post<DesignSuggestion[]>('/api/designs/suggest', { design: draft, measurements: draft.assignments.map((a) => a.measurementId) }, { headers: headers() }));
    if (s) setProposals(s);
  };
  const setAssign = (id: string, patch: Partial<DesignAssignment>) => change({ assignments: draft.assignments.map((a) => (a.measurementId === id ? { ...a, ...patch } : a)) });
  const head = !p.id || saved.data?.head;

  return (
    <div class="design-page">
      <div class="row gap2 design-bar">
        <Button size="sm" icon="back" onClick={() => navigate('/ls/designs')}>
          {t('dsg.back')}
        </Button>
        <div class="grow" style={{ minWidth: 0 }}>
          <Input size="sm" value={draft.name} onValue={(v) => change({ name: v })} placeholder={t('dsg.name')} aria-label={t('dsg.name')} />
        </div>
        {saved.data && <Badge icon="history">{t('dsg.revision_n', { n: saved.data.design.revision })}</Badge>}
        <Button size="sm" icon="sparkles" disabled={!draft.assignments.length} onClick={suggest}>
          {t('dsg.suggest')}
        </Button>
        <Button size="sm" icon="plus" onClick={() => setPicking(true)}>
          {t('dsg.add_measurements')}
        </Button>
        <Button size="sm" kind="primary" icon="save" busy={saving} disabled={!dirty || !draft.name.trim() || !head} onClick={save}>
          {t('dsg.save')}
        </Button>
      </div>
      {!head && <Notice kind="warning">{t('dsg.not_head')}</Notice>}
      {saved.data && saved.data.heads > 1 && <Notice kind="warning">{t('dsg.diverged.d')}</Notice>}
      <div class="design-grid">
        <section class="panel design-levels" aria-label={t('dsg.levels')}>
          <div class="panel-body col gap3">
            <UnitList draft={draft} units={units.data || []} onAdd={addUnit} onChange={change} />
            <ConditionList draft={draft} onChange={change} />
            <TimeList draft={draft} onChange={change} />
          </div>
        </section>
        <section class="panel design-main" aria-label={t('dsg.matrix')}>
          <div class="panel-body col gap3">
            <Matrix draft={draft} review={review} unitLabel={unitLabel} />
            <h3 class="design-h">{t('dsg.measurements')}</h3>
            {!draft.assignments.length ? (
              <Empty icon="files" title={t('dsg.no_measurements')} body={t('dsg.no_measurements.d')} />
            ) : (
              <div class="design-table-wrap">
                <table class="table compact design-table">
                  <thead>
                    <tr>
                      <th>{t('dsg.col.measurement')}</th>
                      <th>{t('dsg.col.unit')}</th>
                      <th>{t('dsg.col.condition')}</th>
                      <th>{t('dsg.col.time')}</th>
                      <th>{t('dsg.col.kind')}</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {[...draft.assignments].sort((x, y) => mName(x.measurementId).localeCompare(mName(y.measurementId), undefined, { numeric: true })).map((a) => {
                      const name = mName(a.measurementId);
                      return (
                        <tr>
                          <td class="ellipsis" title={byId.get(a.measurementId)?.f.name}>
                            {name}
                          </td>
                          <td>
                            <select class="select sm" aria-label={t('dsg.aria.unit', { name })} value={a.unitId || ''} onChange={(e) => setAssign(a.measurementId, { unitId: e.currentTarget.value || undefined })}>
                              <option value="">—</option>
                              {draft.units.map((u) => (
                                <option value={u.id}>{unitLabel(u.id)}</option>
                              ))}
                            </select>
                          </td>
                          <td>
                            <select class="select sm" aria-label={t('dsg.aria.condition', { name })} value={a.condition || ''} onChange={(e) => setAssign(a.measurementId, { condition: e.currentTarget.value || undefined })}>
                              <option value="">—</option>
                              {draft.conditions.map((c) => (
                                <option value={c.label}>{c.label}</option>
                              ))}
                            </select>
                          </td>
                          <td>
                            <select class="select sm" aria-label={t('dsg.aria.time', { name })} value={a.time || ''} onChange={(e) => setAssign(a.measurementId, { time: e.currentTarget.value || undefined })}>
                              <option value="">—</option>
                              {draft.times.map((x) => (
                                <option value={x.label}>{x.label}</option>
                              ))}
                            </select>
                          </td>
                          <td>
                            <select class="select sm design-kind" title={t('dsg.kind.d')} aria-label={t('dsg.aria.kind', { name })} value={a.replicateKind} onChange={(e) => setAssign(a.measurementId, { replicateKind: e.currentTarget.value as DesignAssignment['replicateKind'] })}>
                              <option value="experimental">{t('dsg.kind.experimental')}</option>
                              <option value="technical">{t('dsg.kind.technical')}</option>
                            </select>
                          </td>
                          <td>
                            <Button size="sm" kind="ghost" icon="x" label={t('dsg.aria.remove', { name })} onClick={() => change({ assignments: draft.assignments.filter((x) => x.measurementId !== a.measurementId) })} />
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </section>
        <section class="panel design-review" aria-label={t('dsg.review')}>
          <div class="panel-body col gap3">
            <Review draft={draft} review={review} error={reviewErr} unitLabel={unitLabel} mName={mName} onChange={change} />
            {saved.data?.history && <History history={saved.data.history} unitLabel={unitLabel} mName={mName} />}
            {!p.id && <p class="xs faint">{t('dsg.history.none')}</p>}
            <label class="col gap1 xs">
              <span>{t('dsg.reason')}</span>
              <textarea class="input" rows={2} aria-label={t('dsg.reason')} value={draft.reason || ''} onInput={(e) => setDraft({ ...draft, reason: e.currentTarget.value })} />
            </label>
          </div>
        </section>
      </div>
      {picking && (
        <MeasurementPicker
          title={t('dsg.add_measurements')}
          initial={draft.assignments.map((a) => a.measurementId)}
          onClose={() => setPicking(false)}
          onPick={(ids) => {
            setPicking(false);
            change({ assignments: ids.map((id) => draft.assignments.find((a) => a.measurementId === id) || { measurementId: id, replicateKind: 'experimental' }) });
          }}
        />
      )}
      {proposals && (
        <Proposals
          list={proposals}
          draft={draft}
          mName={mName}
          unitLabel={unitLabel}
          onClose={() => setProposals(null)}
          onConfirm={async (chosen) => {
            setProposals(null);
            let unitsNow = [...draft.units];
            const created: Record<string, string> = {};
            for (const s of [...chosen].sort((a, b) => (a.unitLabel || '').localeCompare(b.unitLabel || '', undefined, { numeric: true }))) {
              if (s.newUnit && s.unitLabel && !created[s.unitLabel.toLowerCase()]) {
                const existing = (units.data || []).find((u) => u.label.toLowerCase() === s.unitLabel!.toLowerCase());
                const u = existing || (await run(() => post<Unit>('/api/experimental-units', { label: s.unitLabel }, { headers: headers() })));
                if (!u) return;
                created[s.unitLabel.toLowerCase()] = u.id;
                if (!unitsNow.some((x) => x.id === u.id)) unitsNow.push({ id: u.id, label: u.label, kind: 'independent_preparation' });
              }
            }
            units.reload();
            const next = draft.assignments.map((a) => {
              const s = chosen.find((x) => x.measurementId === a.measurementId);
              if (!s) return a;
              const unitId = s.unitId || (s.unitLabel ? created[s.unitLabel.toLowerCase()] : undefined);
              return { ...a, unitId: unitId || a.unitId, condition: s.condition || a.condition, time: s.time || a.time };
            });
            const nd = { ...draft, units: unitsNow, assignments: next, source: 'suggestion' as const };
            setDraft({ ...nd, pairs: mergePairs(nd) });
            setDirty(true);
          }}
        />
      )}
    </div>
  );
}

/** mergePairs keeps the person's pairs and adds those implied by derived conditions. */
function mergePairs(d: Draft) {
  const out = [...d.pairs];
  const has = new Set(d.assignments.filter((a) => a.unitId && a.condition).map((a) => a.unitId + '\u0000' + a.condition));
  for (const u of d.units)
    for (const c of d.conditions)
      if (c.derivedFrom && has.has(u.id + '\u0000' + c.derivedFrom) && has.has(u.id + '\u0000' + c.label) && !out.some((p) => p.unitId === u.id && p.from === c.derivedFrom && p.to === c.label))
        out.push({ unitId: u.id, from: c.derivedFrom, to: c.label });
  return out;
}

function UnitList(p: { draft: Draft; units: Unit[]; onAdd: (label: string) => void; onChange: (patch: Partial<Draft>) => void }) {
  const [label, setLabel] = useState('');
  const [pick, setPick] = useState('');
  const others = p.units.filter((u) => !p.draft.units.some((x) => x.id === u.id));
  const used = (id: string) => p.draft.assignments.some((a) => a.unitId === id);
  return (
    <div class="col gap2">
      <h3 class="design-h" title={t('dsg.units.d')}>
        {t('dsg.units')}
      </h3>
      <p class="xs faint">{t('dsg.units.d')}</p>
      {p.draft.units.map((u) => (
        <div class="row gap2 design-level">
          <Icon name="flask" size="sm" />
          <span class="grow ellipsis">{p.units.find((x) => x.id === u.id)?.label || u.label}</span>
          <Button size="sm" kind="ghost" icon="x" disabled={used(u.id)} label={t('dsg.remove_unit')} onClick={() => p.onChange({ units: p.draft.units.filter((x) => x.id !== u.id), pairs: p.draft.pairs.filter((x) => x.unitId !== u.id) })} />
        </div>
      ))}
      <div class="row gap1">
        <Input size="sm" value={label} onValue={setLabel} placeholder={t('dsg.unit_label')} aria-label={t('dsg.unit_label')} />
        <Button size="sm" icon="plus" disabled={!label.trim()} label={t('dsg.add_unit')} onClick={() => (p.onAdd(label.trim()), setLabel(''))} />
      </div>
      {others.length > 0 && (
        <div class="row gap1">
          <select class="select sm grow" aria-label={t('dsg.existing_unit')} value={pick} onChange={(e) => setPick(e.currentTarget.value)}>
            <option value="">{t('dsg.existing_unit')}</option>
            {others.map((u) => (
              <option value={u.id}>{u.label}</option>
            ))}
          </select>
          <Button size="sm" icon="plus" disabled={!pick} label={t('dsg.add_unit')} onClick={() => (p.onChange({ units: [...p.draft.units, { id: pick, label: others.find((u) => u.id === pick)!.label, kind: 'independent_preparation' }] }), setPick(''))} />
        </div>
      )}
    </div>
  );
}

function ConditionList(p: { draft: Draft; onChange: (patch: Partial<Draft>) => void }) {
  const [label, setLabel] = useState('');
  const set = (i: number, c: Partial<DesignCondition>) => p.onChange({ conditions: p.draft.conditions.map((x, k) => (k === i ? { ...x, ...c } : x)) });
  return (
    <div class="col gap2">
      <h3 class="design-h">{t('dsg.conditions')}</h3>
      <p class="xs faint">{t('dsg.conditions.d')}</p>
      {p.draft.conditions.map((c, i) => (
        <div class="col gap1 design-level">
          <div class="row gap2">
            <b class="grow ellipsis">{c.label}</b>
            <Button size="sm" kind="ghost" icon="x" disabled={p.draft.assignments.some((a) => a.condition === c.label)} label={t('dsg.remove_level', { name: c.label })} onClick={() => p.onChange({ conditions: p.draft.conditions.filter((_, k) => k !== i), pairs: p.draft.pairs.filter((x) => x.from !== c.label && x.to !== c.label) })} />
          </div>
          <select class="select sm" aria-label={t('dsg.derived_from', { name: c.label })} value={c.derivedFrom || ''} onChange={(e) => set(i, { derivedFrom: e.currentTarget.value || undefined, process: e.currentTarget.value ? c.process : undefined })}>
            <option value="">{t('dsg.base_condition')}</option>
            {p.draft.conditions
              .filter((o) => o.label !== c.label && !o.derivedFrom)
              .map((o) => (
                <option value={o.label}>{t('dsg.derived_option', { name: o.label })}</option>
              ))}
          </select>
          {c.derivedFrom && <Input size="sm" value={c.process || ''} onValue={(v) => set(i, { process: v })} placeholder={t('dsg.process')} aria-label={t('dsg.process_for', { name: c.label })} />}
        </div>
      ))}
      <div class="row gap1">
        <Input size="sm" value={label} onValue={setLabel} placeholder={t('dsg.condition_label')} aria-label={t('dsg.condition_label')} />
        <Button size="sm" icon="plus" disabled={!label.trim() || p.draft.conditions.some((c) => c.label.toLowerCase() === label.trim().toLowerCase())} label={t('dsg.add_condition')} onClick={() => (p.onChange({ conditions: [...p.draft.conditions, { label: label.trim() }] }), setLabel(''))} />
      </div>
    </div>
  );
}

function TimeList(p: { draft: Draft; onChange: (patch: Partial<Draft>) => void }) {
  const [label, setLabel] = useState('');
  const [offset, setOffset] = useState('');
  const [unit, setUnit] = useState('days');
  const add = () => {
    const tl: DesignTime = { label: label.trim() };
    const v = Number(offset.replace(',', '.'));
    if (offset.trim() !== '' && Number.isFinite(v) && v >= 0) Object.assign(tl, { offset: v, unit });
    p.onChange({ times: [...p.draft.times, tl] });
    setLabel('');
    setOffset('');
  };
  return (
    <div class="col gap2">
      <h3 class="design-h">{t('dsg.times')}</h3>
      <p class="xs faint">{t('dsg.times.d')}</p>
      {p.draft.times.map((x, i) => (
        <div class="row gap2 design-level">
          <Icon name="clock" size="sm" />
          <span class="grow ellipsis">
            {x.label}
            {x.offset !== undefined && <span class="faint"> · {x.offset} {t('unit.' + x.unit)}</span>}
          </span>
          <Button size="sm" kind="ghost" icon="x" disabled={p.draft.assignments.some((a) => a.time === x.label)} label={t('dsg.remove_level', { name: x.label })} onClick={() => p.onChange({ times: p.draft.times.filter((_, k) => k !== i) })} />
        </div>
      ))}
      <Input size="sm" value={label} onValue={setLabel} placeholder={t('dsg.time_label')} aria-label={t('dsg.time_label')} />
      <div class="row gap1">
        <Input size="sm" value={offset} onValue={setOffset} placeholder={t('dsg.time_offset')} aria-label={t('dsg.time_offset')} inputMode="decimal" />
        <select class="select sm" aria-label={t('dsg.time_unit')} value={unit} onChange={(e) => setUnit(e.currentTarget.value)}>
          {UNITS.map((u) => (
            <option value={u}>{t('unit.' + u)}</option>
          ))}
        </select>
        <Button size="sm" icon="plus" disabled={!label.trim() || p.draft.times.some((x) => x.label.toLowerCase() === label.trim().toLowerCase())} label={t('dsg.add_time')} onClick={add} />
      </div>
    </div>
  );
}

function Matrix(p: { draft: Draft; review: DesignStructure | null; unitLabel: (id?: string) => string }) {
  const conds = p.draft.conditions.length ? p.draft.conditions.map((c) => c.label) : [''];
  const times = p.draft.times.length ? p.draft.times.map((x) => x.label) : [''];
  const units = p.draft.units.map((u) => u.id);
  const cell = (u: string, c: string, x: string) => p.review?.cells.find((k) => k.unitId === u && (k.condition || '') === c && (k.time || '') === x);
  const paired = (u: string) => p.draft.pairs.some((x) => x.unitId === u);
  if (!units.length) return <Notice kind="info">{t('dsg.matrix.empty')}</Notice>;
  return (
    <div class="design-matrix-wrap">
      <table class="table compact design-matrix" aria-label={t('dsg.matrix')}>
        <thead>
          <tr>
            <th rowSpan={2}>{t('dsg.col.unit')}</th>
            {conds.map((c) => (
              <th colSpan={times.length} class="design-c">
                {c || t('dsg.no_condition')}
              </th>
            ))}
          </tr>
          <tr>
            {conds.flatMap(() =>
              times.map((x) => (
                <th class="design-c xs">{x || t('dsg.no_time')}</th>
              )),
            )}
          </tr>
        </thead>
        <tbody>
          {units.map((u) => (
            <tr>
              <th scope="row" class="ellipsis">
                {p.unitLabel(u)}
                {paired(u) && (
                  <span class="design-paired" title={t('dsg.paired')}>
                    {' '}
                    ↔
                  </span>
                )}
              </th>
              {conds.flatMap((c) =>
                times.map((x) => {
                  const k = cell(u, c, x);
                  const n = k ? k.experimental + k.technical : 0;
                  return (
                    <td class={`design-cell ${!k ? 'miss' : k.experimental > 1 ? 'warn' : 'ok'}`} title={k ? t('dsg.cell.d', { e: k.experimental, tech: k.technical }) : t('dsg.cell.missing')}>
                      {!k ? '—' : k.technical ? `${k.experimental}+${k.technical}` : String(n)}
                    </td>
                  );
                }),
              )}
            </tr>
          ))}
        </tbody>
      </table>
      <p class="xs faint design-legend">{t('dsg.matrix.legend')}</p>
    </div>
  );
}

function Review(p: { draft: Draft; review: DesignStructure | null; error: string; unitLabel: (id?: string) => string; mName: (id: string) => string; onChange: (patch: Partial<Draft>) => void }) {
  const r = p.review;
  if (p.error) return <Notice kind="danger">{errText(p.error)}</Notice>;
  if (!r) return <Skeleton h={120} />;
  const pairKey = (u: string, from: string, to: string) => p.draft.pairs.some((x) => x.unitId === u && x.from === from && x.to === to);
  const derived = p.draft.conditions.filter((c) => c.derivedFrom);
  return (
    <div class="col gap2">
      <h3 class="design-h">{t('dsg.review')}</h3>
      <div class="design-stats">
        <div>
          <b>{r.experimentalUnits}</b>
          <span>{t('dsg.stat.units')}</span>
        </div>
        <div>
          <b>{r.measurements}</b>
          <span>{t('dsg.stat.measurements')}</span>
        </div>
        <div>
          <b>{r.technicalReadings}</b>
          <span>{t('dsg.stat.technical')}</span>
        </div>
      </div>
      <p class="xs">{t('dsg.n_explained', { units: r.experimentalUnits, n: r.measurements })}</p>
      <Notice kind={r.detected === 'paired_longitudinal' ? 'success' : 'info'} icon="layers">
        <b>{t('dsg.detected.' + r.detected)}</b>
        {r.conditions.length > 0 && <div class="xs">{t(r.conditionWithin ? 'dsg.factor.condition_within' : 'dsg.factor.condition_between', { levels: r.conditions.join(' / ') })}</div>}
        {r.times.length > 0 && <div class="xs">{t(r.timeWithin ? 'dsg.factor.time_within' : 'dsg.factor.time_between', { levels: r.times.join(' / ') })}</div>}
        {r.recommended && <div class="xs">{t('dsg.recommended.' + r.recommended)}</div>}
      </Notice>
      {r.findings.map((f) => (
        <Notice kind={f.code === 'design.incomplete' || f.code === 'design.measurement_without_unit' ? 'info' : 'warning'}>
          {t('dsg.finding.' + f.code.replace('design.', ''), { units: (f.units || []).map((u) => p.unitLabel(u)).join(', '), subject: f.subject || '', n: (f.measurements || []).length })}
        </Notice>
      ))}
      {derived.length > 0 && p.draft.units.length > 0 && (
        <div class="col gap1">
          <h4 class="design-h">{t('dsg.pairs')}</h4>
          <p class="xs faint">{t('dsg.pairs.d')}</p>
          {p.draft.units.flatMap((u) =>
            derived.map((c) => (
              <Check
                checked={pairKey(u.id, c.derivedFrom!, c.label)}
                aria={t('dsg.pair_label', { from: c.derivedFrom!, to: c.label, unit: p.unitLabel(u.id) })}
                label={t('dsg.pair_label', { from: c.derivedFrom!, to: c.label, unit: p.unitLabel(u.id) })}
                onChange={(on) => p.onChange({ pairs: on ? [...p.draft.pairs, { unitId: u.id, from: c.derivedFrom!, to: c.label }] : p.draft.pairs.filter((x) => !(x.unitId === u.id && x.from === c.derivedFrom && x.to === c.label)) })}
              />
            )),
          )}
        </div>
      )}
    </div>
  );
}

function changeText(c: DesignChange, unitLabel: (id?: string) => string, mName: (id: string) => string) {
  const v = (s?: string) => (c.field === 'unit' ? (s ? unitLabel(s) : '—') : c.field === 'replicate_kind' && s ? t('dsg.kind.' + s) : s || '—');
  const who = c.measurementId ? mName(c.measurementId) : c.field.startsWith('unit_') || c.field.startsWith('pair_') ? unitLabel(c.subject) : c.subject || '';
  return t('dsg.change.' + c.field, { who, from: v(c.from), to: v(c.to) });
}

function History(p: { history: Design[]; unitLabel: (id?: string) => string; mName: (id: string) => string }) {
  return (
    <div class="col gap2">
      <h3 class="design-h">{t('dsg.history')}</h3>
      {p.history.map((d) => (
        <details class="design-rev" open={d === p.history[0]}>
          <summary>
            <b>{t('dsg.revision_n', { n: d.revision })}</b> · {fmtTS({ time: d.created, raw: '', tzKnown: true, source: 'file' })}
            {d.source === 'suggestion' && <Badge kind="info">{t('dsg.source.suggestion')}</Badge>}
          </summary>
          {d.reason && <p class="xs">{d.reason}</p>}
          <ul class="xs design-changes">
            {d.changes.slice(0, 60).map((c) => (
              <li>{changeText(c, p.unitLabel, p.mName)}</li>
            ))}
            {d.changes.length > 60 && <li>{t('dsg.more_changes', { n: d.changes.length - 60 })}</li>}
            {!d.changes.length && <li>{t('dsg.no_changes')}</li>}
          </ul>
        </details>
      ))}
    </div>
  );
}

function Proposals(p: { list: DesignSuggestion[]; draft: Draft; mName: (id: string) => string; unitLabel: (id?: string) => string; onClose: () => void; onConfirm: (chosen: DesignSuggestion[]) => void }) {
  const usable = p.list.filter((s) => s.condition || s.unitId || s.unitLabel || s.time);
  const [on, setOn] = useState<Set<string>>(new Set(usable.map((s) => s.measurementId)));
  const toggle = (id: string) => {
    const n = new Set(on);
    n.has(id) ? n.delete(id) : n.add(id);
    setOn(n);
  };
  return (
    <Modal
      title={t('dsg.proposals')}
      sub={t('dsg.proposals.d')}
      icon="sparkles"
      size="xwide"
      onClose={p.onClose}
      foot={
        <>
          <Button onClick={p.onClose}>{t('dsg.reject')}</Button>
          <Button kind="primary" icon="check" disabled={!on.size} onClick={() => p.onConfirm(usable.filter((s) => on.has(s.measurementId)))}>
            {t('dsg.confirm', { n: on.size })}
          </Button>
        </>
      }
    >
      {!p.list.length ? (
        <Empty icon="search" title={t('dsg.proposals.none')} body={t('dsg.proposals.none.d')} />
      ) : (
        <table class="table compact">
          <thead>
            <tr>
              <th />
              <th>{t('dsg.col.measurement')}</th>
              <th>{t('dsg.col.evidence')}</th>
              <th>{t('dsg.col.unit')}</th>
              <th>{t('dsg.col.condition')}</th>
              <th>{t('dsg.col.time')}</th>
            </tr>
          </thead>
          <tbody>
            {p.list.map((s) => {
              const name = p.mName(s.measurementId);
              const ok = usable.includes(s);
              return (
                <tr>
                  <td>{ok && <Check checked={on.has(s.measurementId)} aria={t('dsg.aria.accept', { name })} onChange={() => toggle(s.measurementId)} />}</td>
                  <td class="ellipsis">{name}</td>
                  <td class="xs">
                    {t('dsg.evidence.' + s.source)}: <code>{s.evidence}</code>
                    {(s.notes || []).map((n) => (
                      <div class="faint">{t('dsg.note.' + n.replace('design.', ''))}</div>
                    ))}
                  </td>
                  <td>{s.unitId ? p.unitLabel(s.unitId) : s.unitLabel ? <span>{s.unitLabel} <Badge kind="info">{t('dsg.new_unit')}</Badge></span> : '—'}</td>
                  <td>{s.condition || '—'}</td>
                  <td>{s.time || '—'}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Modal>
  );
}
