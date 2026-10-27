// Settings → Manage Units: system scientific units (never deleted, can be
// hidden) and custom units (deleted when unused; a used one is archived or
// replaced, never deleted destructively). Units of measure, not the
// experimental units of the statistics module.
import { useState } from 'preact/hooks';
import { api, get, post } from '../lib/api';
import { t } from '../lib/i18n';
import { useRev } from '../lib/library';
import { run, toast } from '../lib/state';
import { Icon } from '../ui/icons';
import { Badge, Button, confirmDialog, Field, Input, Modal, Notice, Select, Skeleton, Switch, useAsync } from '../ui/kit';
import { codePoints, listUnits, unitHeaders, type MeasurementUnit } from '../ui/unit-select';

type Use = { kind: string; id: string; label?: string; fields?: string[]; origin: string; replaceable: boolean };
type Usage = { unit: MeasurementUnit; uses: Use[]; counts: Record<string, number>; inUse: boolean; replaceable: boolean };
type DeleteResult = { deleted: boolean; usage?: Usage; options?: string[] };
type Replacement = { changes: unknown[] };

const path = (u: MeasurementUnit, action = '') => '/api/measurement-units/' + encodeURIComponent(u.id) + action;

export function UnitsTab() {
  const rev = useRev();
  const list = useAsync(() => listUnits({ archived: '1', usage: '1' }), [rev]);
  const dims = useAsync(() => get<string[]>('/api/measurement-units/dimensions', { headers: unitHeaders() }), []);
  const [q, setQ] = useState('');
  const [dim, setDim] = useState('');
  const [archived, setArchived] = useState(false);
  const [creating, setCreating] = useState(false);
  const [inUse, setInUse] = useState<Usage | null>(null);
  const all = list.data || [];
  const needle = q.trim().toLowerCase();
  const shown = all.filter((u) => (archived || !u.archived) && (!dim || u.dimension === dim) && (!needle || u.symbol.toLowerCase().includes(needle) || (u.name || '').toLowerCase().includes(needle)));
  const act = async (u: MeasurementUnit, action: string, body: unknown, done?: string) => {
    if ((await run(() => post(path(u, '/' + action), body, { headers: unitHeaders() }))) === undefined) return;
    if (done) toast('success', t(done, { symbol: u.symbol }));
    list.reload();
  };
  const remove = async (u: MeasurementUnit) => {
    const usage = await run(() => get<Usage>(path(u, '/usage'), { headers: unitHeaders() }));
    if (!usage) return;
    if (!usage.inUse) {
      if (!(await confirmDialog({ title: t('units.delete.q', { symbol: u.symbol }), body: t('units.delete.d'), confirm: t('units.delete'), danger: true }))) return;
    }
    const r = await run(() => api<DeleteResult>('DELETE', path(u), undefined, { headers: unitHeaders() }));
    if (!r) return;
    if (r.deleted) {
      toast('success', t('units.deleted.toast', { symbol: u.symbol }));
      list.reload();
    } else if (r.usage) setInUse(r.usage);
  };
  const row = (u: MeasurementUnit) => (
    <div class="setting" data-unit={u.symbol}>
      <Icon name="ruler" />
      <div class="txt">
        <b>
          <bdi style={{ fontFamily: 'var(--mono)' }}>{u.symbol}</bdi>
          {u.name ? ' · ' + u.name : ''}
        </b>
        <span>
          {t('units.dim.' + u.dimension)}{codePoints(u.symbol) ? ' · ' + codePoints(u.symbol) : ''} · {u.uses ? t('units.uses', { n: u.uses }) : t('units.unused')}
        </span>
      </div>
      <div class="row gap1 wrap" style={{ justifyContent: 'flex-end' }}>
        <Badge kind={u.system ? 'info' : 'accent'}>{t(u.system ? 'units.badge.system' : 'units.badge.custom')}</Badge>
        {u.archived && <Badge kind="warning">{t(u.system ? 'units.badge.hidden' : 'units.badge.archived')}</Badge>}
        <Button size="sm" kind="ghost" icon="star" selected={u.favorite} label={t(u.favorite ? 'units.unfavorite' : 'units.favorite') + ' ' + u.symbol} tip={t(u.favorite ? 'units.unfavorite' : 'units.favorite')} onClick={() => act(u, 'favorite', { favorite: !u.favorite })} />
        {u.archived ? (
          <Button size="sm" icon="reset" onClick={() => act(u, 'restore', {}, 'units.restored.toast')}>
            {t('units.restore')}
          </Button>
        ) : (
          <Button size="sm" icon={u.system ? 'eyeOff' : 'archive'} onClick={() => act(u, 'archive', {}, u.system ? undefined : 'units.archived.toast')}>
            {t(u.system ? 'units.hide' : 'units.archive')}
          </Button>
        )}
        {!u.system && (
          <Button size="sm" kind="ghost" icon="trash" onClick={() => remove(u)}>
            {t('units.delete')}
          </Button>
        )}
      </div>
    </div>
  );
  const system = shown.filter((u) => u.system);
  const custom = shown.filter((u) => !u.system);
  return (
    <>
      <section class="col" style={{ gap: 'var(--s2)' }}>
        <h3 style={{ marginBottom: 2 }}>{t('units.title')}</h3>
        <span class="small muted">{t('units.d')}</span>
        <Notice icon="info">{t('units.conversion.d')}</Notice>
        <div class="row gap2 wrap" style={{ alignItems: 'center' }}>
          <div class="grow" style={{ minWidth: 200 }}>
            <Input icon="search" value={q} onValue={setQ} placeholder={t('units.search')} aria-label={t('units.search')} />
          </div>
          <div style={{ width: 230, maxWidth: '100%' }}>
            <Select value={dim} onValue={setDim} label={t('units.dimension')} options={[{ value: '', label: t('units.dim.all') }, ...(dims.data || []).map((d) => ({ value: d, label: t('units.dim.' + d) }))]} />
          </div>
          <Switch checked={archived} onChange={setArchived} label={t('units.show_archived')} />
          <Button size="sm" kind="primary" icon="plus" onClick={() => setCreating(true)}>
            {t('units.create')}
          </Button>
        </div>
      </section>
      <section class="col" style={{ gap: 0 }}>
        <h3 style={{ marginBottom: 2 }}>{t('units.custom')}</h3>
        <span class="small muted" style={{ marginBottom: 'var(--s2)' }}>{t('units.custom.d')}</span>
        {list.loading && !list.data ? <Skeleton h={48} /> : custom.length ? custom.map(row) : <span class="small muted">{t('units.custom.none')}</span>}
      </section>
      <section class="col" style={{ gap: 0 }}>
        <h3 style={{ marginBottom: 2 }}>{t('units.system')}</h3>
        <span class="small muted" style={{ marginBottom: 'var(--s2)' }}>{t('units.system.d')}</span>
        {list.loading && !list.data ? <Skeleton h={48} /> : system.map(row)}
      </section>
      {creating && <CreateUnit dims={dims.data || []} onClose={() => setCreating(false)} onDone={() => (setCreating(false), list.reload())} />}
      {inUse && <InUseDialog usage={inUse} units={all} onClose={() => setInUse(null)} onDone={() => (setInUse(null), list.reload())} />}
    </>
  );
}

function CreateUnit(p: { dims: string[]; onClose: () => void; onDone: () => void }) {
  const [symbol, setSymbol] = useState('');
  const [name, setName] = useState('');
  const [dim, setDim] = useState('length');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    const u = await run(() => post<MeasurementUnit>('/api/measurement-units', { symbol, name: name.trim(), dimension: dim }, { headers: unitHeaders() }));
    setBusy(false);
    if (!u) return;
    toast('success', t('units.created', { symbol: u.symbol }));
    p.onDone();
  };
  return (
    <Modal
      size="narrow"
      icon="ruler"
      title={t('units.create')}
      onClose={p.onClose}
      foot={
        <>
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" busy={busy} disabled={!symbol} onClick={save}>
            {t('units.create')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <Field label={t('units.symbol')} hint={t('units.symbol.d')} id="unit-symbol">
          <Input id="unit-symbol" class="mono" value={symbol} onValue={setSymbol} maxLength={64} />
        </Field>
        <Field label={t('units.name')} id="unit-name">
          <Input id="unit-name" value={name} onValue={setName} maxLength={120} />
        </Field>
        <Field label={t('units.dimension')} id="unit-dimension">
          <Select id="unit-dimension" value={dim} onValue={setDim} options={p.dims.map((d) => ({ value: d, label: t('units.dim.' + d) }))} />
        </Field>
      </div>
    </Modal>
  );
}

// The three choices for a used custom unit: Archive, Replace Usage Then
// Delete (only user-assigned uses; refused while saved analyses or source
// files use it) and Cancel.
function InUseDialog(p: { usage: Usage; units: MeasurementUnit[]; onClose: () => void; onDone: () => void }) {
  const u = p.usage.unit;
  const targets = p.units.filter((x) => x.id !== u.id && !x.archived && x.dimension === u.dimension && x.dimension !== 'unclassified');
  const [target, setTarget] = useState('');
  const [busy, setBusy] = useState('');
  const archive = async () => {
    setBusy('archive');
    const r = await run(() => post(path(u, '/archive'), {}, { headers: unitHeaders() }));
    setBusy('');
    if (r === undefined) return;
    toast('success', t('units.archived.toast', { symbol: u.symbol }));
    p.onDone();
  };
  const replace = async () => {
    const to = targets.find((x) => x.id === target);
    if (!to) return;
    setBusy('replace');
    const r = await run(() => post<Replacement>(path(u, '/replace'), { replacement: to.id }, { headers: unitHeaders() }));
    setBusy('');
    if (!r) return;
    toast('success', t('units.replaced.toast', { from: u.symbol, to: to.symbol, n: r.changes.length }));
    p.onDone();
  };
  return (
    <Modal size="wide" icon="warning" title={t('units.inuse.q', { symbol: u.symbol })} onClose={p.onClose}>
      <div class="col gap3">
        <span class="muted">{t('units.inuse.d')}</span>
        <details>
          <summary>
            {t('units.usage')} · {t('units.uses', { n: p.usage.uses.length })}
          </summary>
          <ul class="small" style={{ margin: '6px 0 0', paddingLeft: 18, maxHeight: 180, overflow: 'auto' }}>
            {p.usage.uses.map((x) => (
              <li>
                {t('units.use.' + x.kind)} · {x.label || x.id} · {t('units.origin.' + x.origin)}
              </li>
            ))}
          </ul>
        </details>
        <div class="setting">
          <Icon name="archive" />
          <div class="txt">
            <b>{t('units.archive')}</b>
            <span>{t('units.opt.archive.d')}</span>
          </div>
          <Button kind="primary" busy={busy === 'archive'} onClick={archive}>
            {t('units.archive')}
          </Button>
        </div>
        <div class="setting" style={{ flexWrap: 'wrap' }}>
          <Icon name="workflow" />
          <div class="txt">
            <b>{t('units.opt.replace')}</b>
            <span>{t('units.opt.replace.d')}</span>
            {!p.usage.replaceable && (
              <div style={{ marginTop: 6 }}>
                <Notice kind="warning">{t('units.replace.blocked')}</Notice>
              </div>
            )}
          </div>
          {p.usage.replaceable && (
            <div class="row gap2">
              <Select value={target} onValue={setTarget} label={t('units.replace.with')} options={[{ value: '', label: t('units.replace.choose') }, ...targets.map((x) => ({ value: x.id, label: x.symbol + (x.name ? ' · ' + x.name : '') }))]} />
              <Button kind="danger" busy={busy === 'replace'} disabled={!target} onClick={replace}>
                {t('units.opt.replace')}
              </Button>
            </div>
          )}
        </div>
        <div class="row">
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
        </div>
      </div>
    </Modal>
  );
}
