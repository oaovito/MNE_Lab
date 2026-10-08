// Cycle Library and the cycle wizard. A cycle references measurements and
// plans temporal points; it never changes the original data.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { post } from '../../lib/api';
import { markSeen, seen } from '../../lib/actions';
import { fmtDate, fmtTS, toLocalInput } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { allMeasurements, measurementName, PARAMS, UNITS, useCycles, useFiles, useRelations } from '../../lib/library';
import { navigate } from '../../lib/route';
import { closePanel, openPanel, run } from '../../lib/state';
import type { CycleConfig, CycleDoc, CycleView } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Badge, Button, Check, Empty, Field, Input, Modal, Notice, Select, Skeleton, Spinner } from '../../ui/kit';
import { MeasurementPicker } from './picker';

export function CycleLibrary() {
  const [trash, setTrash] = useState(false);
  const cycles = useCycles(trash);
  const rel = useRelations();
  const [q, setQ] = useState('');
  const list = (cycles.data || []).filter((c) => !q || [c.config.name, c.config.sampleId, c.config.experiment, ...(c.tags || [])].join(' ').toLowerCase().includes(q.toLowerCase()));
  return (
    <>
      <div class="toolbar">
        <div style={{ width: 'min(300px, 36vw)' }}>
          <Input icon="search" size="sm" value={q} onValue={setQ} placeholder={t('cycles.search')} aria-label={t('cycles.search')} />
        </div>
        <span class="spacer" />
        <Button size="sm" kind={trash ? 'default' : 'ghost'} icon="trash" selected={trash} onClick={() => setTrash(!trash)}>
          {t('ui.trash_bin')}
        </Button>
        {!trash && (
          <Button size="sm" kind="primary" icon="plus" onClick={() => openPanel('cycle-wizard', { measurements: [] })}>
            {t('cycles.new')}
          </Button>
        )}
      </div>
      <div class="panel" style={{ flex: 1 }}>
        <div class="panel-body" style={{ padding: 'var(--s4)' }}>
          {cycles.loading && !cycles.data ? (
            <div class="cards">
              {[1, 2, 3].map(() => (
                <Skeleton h={150} r="var(--r-lg)" />
              ))}
            </div>
          ) : cycles.error ? (
            <Empty icon="alert" title={t('err.title')} body={errText(cycles.error)} />
          ) : list.length === 0 ? (
            trash ? (
              <Empty icon="trash" title={t('cycles.trash.empty')} body={t('files.trash.body')} />
            ) : (
              <Empty icon="cycle" title={t('cycles.empty')} body={t('cycles.empty.d')}>
                <Button kind="primary" icon="plus" onClick={() => openPanel('cycle-wizard', { measurements: [] })}>
                  {t('cycles.new')}
                </Button>
              </Empty>
            )
          ) : (
            <div class="cards">
              {list.map((c) => (
                <CycleCard c={c} files={rel.data?.cycles[c.id!]?.files?.length || 0} graphs={rel.data?.cycles[c.id!]?.graphs?.length || 0} trash={trash} />
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function CycleCard(p: { c: CycleDoc; files: number; graphs: number; trash: boolean }) {
  const c = p.c;
  const n = Math.floor(c.config.duration / c.config.interval) + 1;
  const restore = async (e: Event) => {
    e.stopPropagation();
    await run(() => post(`/api/cycles/${c.id}/trash`, { trashed: false }));
  };
  return (
    <div class="gcard cycle-card" role="button" tabIndex={0} onClick={() => !p.trash && navigate('/ls/cycles/' + c.id)} onKeyDown={(e) => e.key === 'Enter' && !p.trash && navigate('/ls/cycles/' + c.id)}>
      <div class="bars" aria-hidden="true">
        {Array.from({ length: Math.min(n, 40) }, (_, i) => (
          <span style={{ height: `${30 + ((i * 37) % 70)}%` }} class={i >= Math.min(n, 40) ? 'miss' : ''} />
        ))}
      </div>
      <div class="body">
        <span class="title ellipsis">{c.config.name}</span>
        <div class="row gap1 wrap">
          <Badge kind="accent" icon="clock">
            {t('cycles.every', { n: c.config.interval, unit: t('unit.' + c.config.unit) })}
          </Badge>
          <Badge>{t('cycles.duration', { n: c.config.duration, unit: t('unit.' + c.config.unit) })}</Badge>
        </div>
        <span class="meta">
          {[c.config.sampleId, c.config.experiment].filter(Boolean).join(' · ') || t('cycles.points', { n })}
        </span>
        <span class="meta row gap2">
          <span class="row gap1">
            <Icon name="file" size="sm" />
            {t('graphs.n_files', { n: p.files })}
          </span>
          <span class="row gap1">
            <Icon name="chart" size="sm" />
            {t('cycles.n_graphs', { n: p.graphs })}
          </span>
          <span class="row gap1">
            <Icon name="clock" size="sm" />
            {fmtDate(c.updated)}
          </span>
        </span>
        {p.trash && (
          <Button size="sm" icon="undo" onClick={restore}>
            {t('ui.restore')}
          </Button>
        )}
      </div>
    </div>
  );
}

// ---- wizard ----

const defaultConfig = (): CycleConfig => ({ name: '', experiment: '', sampleId: '', start: new Date().toISOString(), startTzKnown: false, interval: 1, unit: 'days', duration: 7, replicates: 3, params: ['effective_diameter', 'polydispersity'] });

/** CycleWizard creates a cycle, or edits one (arg.edit). */
export function CycleWizard(p: { arg: { measurements: string[]; edit?: CycleDoc } }) {
  const edit = p.arg.edit;
  const files = useFiles();
  const byId = useMemo(() => allMeasurements(files.data || []), [files.data]);
  const [step, setStep] = useState(0);
  const [cfg, setCfg] = useState<CycleConfig>(edit ? { ...edit.config } : defaultConfig());
  const [ms, setMs] = useState<string[]>(edit ? edit.measurements : p.arg.measurements);
  const [startLocal, setStartLocal] = useState(edit ? toLocalInput(edit.config.start, !edit.config.startTzKnown) : '');
  const [preview, setPreview] = useState<CycleView | null>(null);
  const [perr, setPerr] = useState('');
  const [busy, setBusy] = useState(false);
  const [picking, setPicking] = useState(false);
  const firstUse = !seen('cycle') && !edit;
  const set = (patch: Partial<CycleConfig>) => setCfg({ ...cfg, ...patch });

  // Suggestions from the selected measurements: sample, experiment and the
  // earliest measurement time as the start.
  useEffect(() => {
    if (edit || !files.data) return;
    const sel = ms.map((id) => byId.get(id)).filter(Boolean) as { m: any; f: any }[];
    const samples = [...new Set(sel.map((x) => x.m.sampleId).filter(Boolean))];
    const exps = [...new Set(sel.map((x) => x.m.experiment || x.f.experiment).filter(Boolean))];
    const times = sel.map((x) => x.m.measuredAt).filter(Boolean).sort((a: any, b: any) => Date.parse(a.time) - Date.parse(b.time));
    const patch: Partial<CycleConfig> = {};
    if (!cfg.sampleId && samples.length === 1) patch.sampleId = samples[0];
    if (!cfg.experiment && exps.length === 1) patch.experiment = exps[0];
    if (!cfg.name) patch.name = samples.length === 1 ? t('cycles.default_name', { sample: samples[0] }) : '';
    if (!startLocal) {
      const first = times[0] as any;
      setStartLocal(first ? toLocalInput(first.time, !first.tzKnown) : toLocalInput(undefined));
      if (first) patch.startTzKnown = !!first.tzKnown;
    }
    if (Object.keys(patch).length) setCfg((c) => ({ ...c, ...patch }));
  }, [files.data, ms.join()]);

  const doc = (): CycleDoc => {
    const start = cfg.startTzKnown ? new Date(startLocal).toISOString() : startLocal + ':00Z';
    return { ...(edit || {}), config: { ...cfg, name: cfg.name.trim(), start }, measurements: ms } as CycleDoc;
  };
  const validCfg = cfg.name.trim() && startLocal && cfg.interval >= 1 && cfg.interval <= 99 && cfg.duration >= 1 && cfg.duration <= 99 && cfg.interval <= cfg.duration && cfg.params.length > 0;

  useEffect(() => {
    if (step !== 1) return;
    setPreview(null);
    setPerr('');
    run(() => post<CycleView>('/api/cycles/preview', doc()), setPerr).then((v) => v && setPreview(v));
  }, [step, ms.join()]);

  const save = async () => {
    setBusy(true);
    const c = await run(() => post<CycleDoc>('/api/cycles', doc()));
    setBusy(false);
    if (!c) return;
    markSeen('cycle');
    closePanel();
    navigate('/ls/cycles/' + c.id);
  };

  const nPoints = validCfg ? Math.floor(cfg.duration / cfg.interval) + 1 : 0;
  const unitHint = cfg.unit === 'months' || cfg.unit === 'years' ? t('cycles.calendar_hint') : '';
  return (
    <Modal
      size="wide"
      icon="cycle"
      title={edit ? t('cycles.edit') : t('cycles.new')}
      sub={step === 0 ? t('cycles.step.plan') : t('cycles.step.review')}
      onClose={closePanel}
      foot={
        <>
          {step === 1 && (
            <Button kind="ghost" icon="back" onClick={() => setStep(0)}>
              {t('ui.back')}
            </Button>
          )}
          <span class="spacer" />
          {step === 0 ? (
            <Button kind="primary" trail="next" disabled={!validCfg} onClick={() => setStep(1)}>
              {t('cycles.preview')}
            </Button>
          ) : (
            <Button kind="primary" icon="check" busy={busy} disabled={!preview} onClick={save}>
              {edit ? t('cycles.save_changes') : t('cycles.create')}
            </Button>
          )}
        </>
      }
    >
      {step === 0 ? (
        <div class="col gap4">
          {firstUse && (
            <Notice kind="info" icon="sparkles">
              <strong>{t('cycles.intro.title')}</strong>
              <div>{t('cycles.intro.body')}</div>
            </Notice>
          )}
          <div class="row gap3" style={{ alignItems: 'flex-start' }}>
            <Field label={t('cycles.name')} class="grow">
              <Input value={cfg.name} onValue={(name) => set({ name })} maxLength={120} autoFocus placeholder={t('cycles.name.ph')} />
            </Field>
            <Field label={t('col.sample_id')} class="grow">
              <Input value={cfg.sampleId || ''} onValue={(sampleId) => set({ sampleId })} maxLength={120} />
            </Field>
            <Field label={t('files.f.experiment')} class="grow">
              <Input value={cfg.experiment || ''} onValue={(experiment) => set({ experiment })} maxLength={120} />
            </Field>
          </div>
          <div class="row gap3" style={{ alignItems: 'flex-start' }}>
            <Field label={t('meta.cycle_start')} hint={cfg.startTzKnown ? t('cycles.start.local') : t('cycles.start.as_written')} class="grow">
              <input type="datetime-local" class="input" value={startLocal} onInput={(e) => setStartLocal((e.target as HTMLInputElement).value)} />
            </Field>
            <Field label={t('meta.cycle_interval')}>
              <Input value={String(cfg.interval)} onValue={(v) => set({ interval: Math.min(99, parseInt(v.replace(/\D/g, ''), 10) || 0) })} inputMode="numeric" style={{ width: 90 }} />
            </Field>
            <Field label={t('cycles.unit')}>
              <Select value={cfg.unit} onValue={(unit) => set({ unit })} options={UNITS.map((u) => ({ value: u, label: t('unit.' + u) }))} />
            </Field>
            <Field label={t('meta.cycle_duration')} error={cfg.interval > cfg.duration ? t('err.cycle.interval_exceeds_duration') : undefined}>
              <Input value={String(cfg.duration)} onValue={(v) => set({ duration: Math.min(99, parseInt(v.replace(/\D/g, ''), 10) || 0) })} inputMode="numeric" style={{ width: 90 }} />
            </Field>
            <Field label={t('cycles.replicates')} hint={t('cycles.replicates.hint')}>
              <Input value={String(cfg.replicates)} onValue={(v) => set({ replicates: Math.min(99, parseInt(v.replace(/\D/g, ''), 10) || 0) })} inputMode="numeric" style={{ width: 90 }} />
            </Field>
          </div>
          <div class="row gap2 small muted">
            <Icon name="info" size="sm" />
            {validCfg ? t('cycles.summary', { n: nPoints, every: cfg.interval, unit: t('unit.' + cfg.unit), total: cfg.duration }) : t('cycles.limits')}
            {unitHint && <span class="faint">· {unitHint}</span>}
          </div>
          <div class="field">
            <span class="label">{t('cycles.params')}</span>
            <div class="row wrap gap4">
              {PARAMS.map((k) => (
                <Check checked={cfg.params.includes(k)} onChange={(on) => set({ params: on ? [...cfg.params, k] : cfg.params.filter((x) => x !== k) })} label={t('param.' + k)} />
              ))}
            </div>
          </div>
          <div class="field">
            <div class="row">
              <span class="label grow">{t('cycles.measurements', { n: ms.length })}</span>
              <Button size="sm" icon="plus" onClick={() => setPicking(true)}>
                {t('cycles.choose_measurements')}
              </Button>
            </div>
            <div class="tags" style={{ maxHeight: 96, overflow: 'auto' }}>
              {ms.slice(0, 40).map((id) => {
                const x = byId.get(id);
                return <span class="tag">{x ? measurementName(x.m, x.f) : '…'}</span>;
              })}
              {ms.length > 40 && <span class="tag">+{ms.length - 40}</span>}
              {!ms.length && <span class="xs faint">{t('cycles.no_measurements_yet')}</span>}
            </div>
          </div>
        </div>
      ) : (
        <CyclePreview v={preview} err={perr} byId={byId} />
      )}
      {picking && (
        <MeasurementPicker
          title={t('cycles.choose_measurements')}
          initial={ms}
          onClose={() => setPicking(false)}
          onPick={(ids) => {
            setMs(ids);
            setPicking(false);
          }}
        />
      )}
    </Modal>
  );
}

function CyclePreview(p: { v: CycleView | null; err: string; byId: ReturnType<typeof allMeasurements> }) {
  if (p.err) return <Notice kind="danger">{errText(p.err)}</Notice>;
  if (!p.v) return <Spinner label={t('cycles.planning')} />;
  const v = p.v;
  const as = v.assignments || [];
  const unassigned = as.filter((a) => a.point < 0 || a.status === 'needs_confirmation');
  return (
    <div class="col gap3">
      <div class="row gap2 small muted">
        <Badge kind="accent">{t('cycles.points', { n: v.points?.length || 0 })}</Badge>
        <Badge kind="success">{t('cycles.auto_assigned', { n: as.filter((a) => a.status === 'auto').length })}</Badge>
        {unassigned.length > 0 && <Badge kind="warning">{t('cycles.need_review', { n: unassigned.length })}</Badge>}
      </div>
      <div class="points" style={{ maxHeight: '46vh', overflow: 'auto' }}>
        {(v.points || []).map((pt) => {
          const here = as.filter((a) => a.point === pt.index && a.status !== 'unassigned');
          return (
            <div class={`point ${here.length ? '' : 'miss'}`}>
              <div class="when">
                <b>{t('point.' + pt.unit, { n: pt.offset })}</b>
                <span>{fmtTS({ time: pt.target, raw: '', tzKnown: v.config.startTzKnown, source: 'file' })}</span>
              </div>
              <div class="row wrap gap1">
                {here.length ? (
                  here.map((a) => {
                    const x = p.byId.get(a.measurementId);
                    return (
                      <span class={`assign ${a.status}`} data-tip={a.reason ? t('reason.' + a.reason) : t('cycle.status.' + a.status)}>
                        <span class="ellipsis">{x ? measurementName(x.m, x.f) : '…'}</span>
                        {a.status === 'needs_confirmation' && <Icon name="warning" size="sm" />}
                      </span>
                    );
                  })
                ) : (
                  <span class="xs faint">{t('cycle.status.missing')}</span>
                )}
              </div>
              <span class="xs faint">{here.length ? t('cycles.n_meas', { n: here.length }) : ''}</span>
            </div>
          );
        })}
      </div>
      {unassigned.length > 0 && (
        <Notice kind="warning">
          <strong>{t('cycles.review_after')}</strong>
          <ul style={{ margin: '4px 0 0', paddingLeft: 18 }}>
            {unassigned.slice(0, 6).map((a) => {
              const x = p.byId.get(a.measurementId);
              return (
                <li>
                  {x ? measurementName(x.m, x.f) : '…'}: {a.reason ? t('reason.' + a.reason) : t('cycle.status.' + a.status)}
                </li>
              );
            })}
          </ul>
        </Notice>
      )}
    </div>
  );
}
