// File Library: import NanoBrook exports, review what was detected, find
// files by any attribute and select measurements for graphs and cycles.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { api, get, patch, post } from '../../lib/api';
import { markSeen, seen } from '../../lib/actions';
import { fmtBytes, fmtDate, fmtQ, fmtTS, toLocalInput } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { measurementName, PARAMS, useFiles, useRelations, warnText } from '../../lib/library';
import { navigate } from '../../lib/route';
import { app, openPanel, run, toast } from '../../lib/state';
import { createStore, useStore } from '../../lib/store';
import type { FileView, ImportResult, Measurement, MeasurementSummary, SourceFile } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Badge, Button, Check, confirmDialog, Empty, Field, Input, Menu as MenuLazy, Modal, Notice, Seg, Skeleton, Spinner, Tabs, useAsync } from '../../ui/kit';

/** The measurements selected for a graph, a cycle or an export. */
export const selection = createStore<{ ids: string[] }>({ ids: [] });
const setSel = (ids: string[]) => selection.set({ ids: [...new Set(ids)] });
const toggleSel = (ids: string[], on: boolean) => {
  const cur = new Set(selection.get().ids);
  ids.forEach((id) => (on ? cur.add(id) : cur.delete(id)));
  selection.set({ ids: [...cur] });
};

// ---- import ----

type ImportRow = { name: string; size: number; state: 'waiting' | 'running' | 'done'; result?: ImportResult; file: File };
export const imports = createStore<{ rows: ImportRow[]; open: boolean }>({ rows: [], open: false });

async function uploadOne(row: ImportRow, force = false) {
  const update = (r: Partial<ImportRow>) => imports.set((s) => ({ rows: s.rows.map((x) => (x === row ? Object.assign(row, r) : x)) }));
  update({ state: 'running' });
  try {
    const res = await api<ImportResult>('POST', '/api/files' + (force ? '?force=1' : ''), undefined, { raw: row.file, headers: { 'X-File-Name': encodeURIComponent(row.name), 'Content-Type': 'application/octet-stream' } });
    update({ state: 'done', result: res });
  } catch (e: any) {
    update({ state: 'done', result: { name: row.name, status: 'failed', measurements: 0, error: e?.code || 'app.internal_error' } });
  }
}

export async function importFileList(list: File[]) {
  if (!list.length) return;
  const rows: ImportRow[] = list.map((f) => ({ name: f.name, size: f.size, state: 'waiting', file: f }));
  imports.set((s) => ({ rows: [...s.rows.filter((r) => r.state !== 'done'), ...rows], open: true }));
  for (const r of rows) await uploadOne(r);
}

/** importFiles opens the system file chooser and imports what was picked. */
export function importFiles() {
  const input = document.createElement('input');
  input.type = 'file';
  input.multiple = true;
  input.accept = '.txt,.csv,.tsv,.dat,.asc,.dls,text/plain,text/csv,text/tab-separated-values';
  input.onchange = () => importFileList([...(input.files || [])]);
  input.click();
}

export function ImportDialog() {
  const { rows, open } = useStore(imports, (s) => s);
  if (!open) return null;
  const running = rows.some((r) => r.state !== 'done');
  const ok = rows.filter((r) => r.result && (r.result.status === 'parsed' || r.result.status === 'partial'));
  const close = () => imports.set({ open: false, rows: running ? rows : [] });
  return (
    <Modal
      title={t('imp.title')}
      sub={running ? t('imp.running') : t('imp.summary', { ok: ok.length, n: rows.length })}
      icon="upload"
      size="wide"
      onClose={close}
      foot={
        <>
          <Button kind="ghost" icon="plus" onClick={importFiles} disabled={running}>
            {t('imp.more')}
          </Button>
          <span class="spacer" />
          {ok.length > 0 && !running && (
            <Button
              icon="chart"
              onClick={() => {
                const ids = ok.map((r) => r.result!.fileId!).filter(Boolean);
                close();
                navigate('/ls/files?file=' + ids[0]);
              }}
            >
              {t('imp.review')}
            </Button>
          )}
          <Button kind="primary" onClick={close} disabled={running}>
            {t('ui.done')}
          </Button>
        </>
      }
    >
      <div class="col gap2">
        {rows.map((r) => (
          <ImportItem row={r} />
        ))}
      </div>
      <p class="xs faint" style={{ marginTop: 'var(--s4)' }}>
        {t('imp.preserved')}
      </p>
    </Modal>
  );
}

function ImportItem(p: { row: ImportRow }) {
  const r = p.row;
  const res = r.result;
  const icon = !res ? null : res.status === 'parsed' ? 'ok' : res.status === 'partial' ? 'warning' : res.status === 'duplicate' ? 'copy' : 'alert';
  const tone = !res ? '' : res.status === 'parsed' ? 'success' : res.status === 'failed' ? 'danger' : 'warning';
  return (
    <div class="card" style={{ padding: 'var(--s3) var(--s4)' }}>
      <div class="row gap3">
        {r.state === 'done' && icon ? <Icon name={icon} class="" /> : <span class="spin" />}
        <div class="grow col" style={{ gap: 0, minWidth: 0 }}>
          <b class="ellipsis small">{r.name}</b>
          <span class="xs faint">
            {fmtBytes(r.size)}
            {res && res.status !== 'failed' && res.status !== 'duplicate' && ' · ' + t('imp.measurements', { n: res.measurements })}
            {res?.recognized?.length ? ' · ' + res.recognized.map((k) => t('param.short.' + k) !== 'param.short.' + k ? t('param.short.' + k) : t('param.' + k)).join(', ') : ''}
          </span>
        </div>
        {res && <Badge kind={tone as any}>{t('imp.status.' + res.status)}</Badge>}
        {res?.status === 'duplicate' && (
          <>
            <Button size="sm" kind="ghost" onClick={() => res.existing && navigate('/ls/files?file=' + res.existing)}>
              {t('imp.open_existing')}
            </Button>
            <Button size="sm" onClick={() => uploadOne(r, true)}>
              {t('imp.import_anyway')}
            </Button>
          </>
        )}
      </div>
      {res?.error && <div class="xs" style={{ color: 'var(--danger)', marginTop: 6, marginLeft: 30 }}>{errText(res.error)}</div>}
      {res?.needsDate && (
        <div class="xs" style={{ color: 'var(--warning)', marginTop: 6, marginLeft: 30 }}>
          {t('imp.needs_date')}
        </div>
      )}
      {!!res?.warnings?.length && (
        <ul class="xs muted" style={{ margin: '6px 0 0 30px', paddingLeft: 16 }}>
          {res.warnings.slice(0, 4).map((w) => (
            <li>{warnText(w)}</li>
          ))}
          {res.warnings.length > 4 && <li>{t('imp.more_warnings', { n: res.warnings.length - 4 })}</li>}
        </ul>
      )}
    </div>
  );
}

// ---- library ----

type Filters = { q: string; status: string; experiment: string; group: string; tag: string; from: string; to: string };
const noFilters: Filters = { q: '', status: '', experiment: '', group: '', tag: '', from: '', to: '' };

function matches(f: FileView, fl: Filters) {
  const ms = f.items || [];
  if (fl.status && f.status !== fl.status) return false;
  if (fl.experiment && f.experiment !== fl.experiment && !ms.some((m) => m.experiment === fl.experiment)) return false;
  if (fl.group && f.group !== fl.group && !ms.some((m) => m.group === fl.group)) return false;
  if (fl.tag && !(f.tags || []).includes(fl.tag) && !ms.some((m) => (m.tags || []).includes(fl.tag))) return false;
  if (fl.from || fl.to) {
    const lo = fl.from ? Date.parse(fl.from + 'T00:00:00Z') : -Infinity;
    const hi = fl.to ? Date.parse(fl.to + 'T23:59:59Z') : Infinity;
    if (!ms.some((m) => m.measuredAt && Date.parse(m.measuredAt.time) >= lo && Date.parse(m.measuredAt.time) <= hi)) return false;
  }
  if (fl.q) {
    const q = fl.q.toLowerCase();
    const hay = [f.name, f.experiment, f.group, f.notes, ...(f.tags || []), ...ms.flatMap((m) => [m.sampleId, m.label, m.condition, m.notes, m.experiment, m.group, ...(m.tags || []), m.measuredAt?.raw])].filter(Boolean).join(' ').toLowerCase();
    if (!hay.includes(q)) return false;
  }
  return true;
}

export function FileLibrary() {
  const [trash, setTrash] = useState(false);
  const files = useFiles(trash);
  const rel = useRelations();
  const sel = useStore(selection, (s) => s.ids);
  const [fl, setFl] = useState<Filters>(noFilters);
  const [showFilters, setShowFilters] = useState(false);
  const [open, setOpen] = useState<string | null>(new URLSearchParams(location.search).get('file'));
  const [drag, setDrag] = useState(false);
  const list = (files.data || []).filter((f) => matches(f, fl));
  const facets = useMemo(() => {
    const ex = new Set<string>(),
      gr = new Set<string>(),
      tg = new Set<string>();
    for (const f of files.data || []) {
      f.experiment && ex.add(f.experiment);
      f.group && gr.add(f.group);
      (f.tags || []).forEach((x) => tg.add(x));
      for (const m of f.items || []) {
        m.experiment && ex.add(m.experiment);
        m.group && gr.add(m.group);
        (m.tags || []).forEach((x) => tg.add(x));
      }
    }
    return { ex: [...ex].sort(), gr: [...gr].sort(), tg: [...tg].sort() };
  }, [files.data]);
  const current = (files.data || []).find((f) => f.id === open) || null;
  const nFilters = Object.entries(fl).filter(([k, v]) => k !== 'q' && v).length;

  useEffect(() => {
    if (!seen('ls_files') && files.data?.length) markSeen('ls_files');
  }, [files.data?.length]);

  // Files dropped anywhere on the library are imported.
  useEffect(() => {
    if (trash) return;
    let depth = 0;
    const enter = (e: DragEvent) => {
      if (!e.dataTransfer?.types.includes('Files')) return;
      depth++;
      setDrag(true);
    };
    const leave = () => {
      depth = Math.max(0, depth - 1);
      if (!depth) setDrag(false);
    };
    const over = (e: DragEvent) => e.dataTransfer?.types.includes('Files') && e.preventDefault();
    const drop = (e: DragEvent) => {
      e.preventDefault();
      depth = 0;
      setDrag(false);
      importFileList([...(e.dataTransfer?.files || [])]);
    };
    document.addEventListener('dragenter', enter);
    document.addEventListener('dragleave', leave);
    document.addEventListener('dragover', over);
    document.addEventListener('drop', drop);
    return () => {
      document.removeEventListener('dragenter', enter);
      document.removeEventListener('dragleave', leave);
      document.removeEventListener('dragover', over);
      document.removeEventListener('drop', drop);
    };
  }, [trash]);

  const allIds = list.flatMap((f) => (f.items || []).map((m) => m.id));
  const allOn = allIds.length > 0 && allIds.every((id) => sel.includes(id));
  const someOn = allIds.some((id) => sel.includes(id));

  if (!trash && files.data && files.data.length === 0) return <LsEmpty />;

  return (
    <>
      <div class="toolbar">
        <div style={{ width: 'min(340px, 40vw)' }}>
          <Input icon="search" size="sm" value={fl.q} onValue={(q) => setFl({ ...fl, q })} placeholder={t('files.search')} aria-label={t('files.search')} />
        </div>
        <Button size="sm" icon="filter" selected={showFilters || nFilters > 0} onClick={() => setShowFilters(!showFilters)}>
          {t('files.filters')}
          {nFilters > 0 && <Badge kind="accent">{nFilters}</Badge>}
        </Button>
        <SetsMenu />
        <span class="spacer" />
        <Seg
          size="sm"
          value={trash ? 'trash' : 'lib'}
          onValue={(v) => (setTrash(v === 'trash'), setOpen(null))}
          options={[
            { value: 'lib', label: t('files.library'), icon: 'files' },
            { value: 'trash', label: t('ui.trash_bin'), icon: 'trash' },
          ]}
        />
        {!trash && (
          <Button size="sm" kind="primary" icon="upload" onClick={importFiles}>
            {t('ls.import')}
          </Button>
        )}
      </div>
      {showFilters && (
        <div class="card row wrap gap3" style={{ padding: 'var(--s3) var(--s4)', animation: 'rise-in var(--t2) var(--ease-out)' }}>
          <FilterSelect label={t('files.f.status')} value={fl.status} onValue={(status) => setFl({ ...fl, status })} options={['parsed', 'partial', 'failed'].map((v) => ({ value: v, label: t('file.status.' + v) }))} />
          <FilterSelect label={t('files.f.experiment')} value={fl.experiment} onValue={(experiment) => setFl({ ...fl, experiment })} options={facets.ex.map((v) => ({ value: v, label: v }))} />
          <FilterSelect label={t('files.f.group')} value={fl.group} onValue={(group) => setFl({ ...fl, group })} options={facets.gr.map((v) => ({ value: v, label: v }))} />
          <FilterSelect label={t('files.f.tag')} value={fl.tag} onValue={(tag) => setFl({ ...fl, tag })} options={facets.tg.map((v) => ({ value: v, label: v }))} />
          <label class="row gap1 small muted">
            {t('files.f.from')}
            <input type="date" class="input sm" style={{ width: 150 }} value={fl.from} onInput={(e) => setFl({ ...fl, from: (e.target as HTMLInputElement).value })} />
          </label>
          <label class="row gap1 small muted">
            {t('files.f.to')}
            <input type="date" class="input sm" style={{ width: 150 }} value={fl.to} onInput={(e) => setFl({ ...fl, to: (e.target as HTMLInputElement).value })} />
          </label>
          <span class="spacer" />
          <Button size="sm" kind="ghost" icon="reset" disabled={!nFilters} onClick={() => setFl({ ...noFilters, q: fl.q })}>
            {t('files.f.clear')}
          </Button>
        </div>
      )}
      <div class="split files">
        <div class="panel">
          <div class="panel-body">
            {files.loading && !files.data ? (
              <div class="col gap3" style={{ padding: 'var(--s4)' }}>
                {[1, 2, 3, 4, 5].map(() => (
                  <Skeleton h={22} />
                ))}
              </div>
            ) : files.error ? (
              <Empty icon="alert" title={t('err.title')} body={errText(files.error)}>
                <Button onClick={files.reload}>{t('ui.retry')}</Button>
              </Empty>
            ) : list.length === 0 ? (
              <Empty icon={trash ? 'trash' : 'search'} title={trash ? t('files.trash.empty') : t('files.none_match')} body={trash ? t('files.trash.body') : t('files.none_match.body')} />
            ) : (
              <table class="table">
                <thead>
                  <tr>
                    <th style={{ width: 36 }}>{!trash && <Check checked={allOn} indeterminate={!allOn && someOn} onChange={(on) => toggleSel(allIds, on)} aria={t('files.select_all')} />}</th>
                    <th>{t('files.col.name')}</th>
                    <th>{t('col.sample_id')}</th>
                    <th>{t('col.measured_at')}</th>
                    <th class="num">{t('files.col.runs')}</th>
                    <th>{t('files.col.org')}</th>
                  </tr>
                </thead>
                <tbody>
                  {list.map((f) => {
                    const ids = (f.items || []).map((m) => m.id);
                    const on = ids.length > 0 && ids.every((id) => sel.includes(id));
                    const some = ids.some((id) => sel.includes(id));
                    const samples = [...new Set((f.items || []).map((m) => m.sampleId).filter(Boolean))];
                    const dates = (f.items || []).map((m) => m.measuredAt).filter(Boolean);
                    const needs = (f.items || []).some((m) => !m.measuredAt || (m.measuredAt.ambiguous && !m.measuredAt.confirmed));
                    return (
                      <tr class={`clickable ${open === f.id ? 'sel' : ''}`} onClick={() => setOpen(f.id)}>
                        <td onClick={(e) => e.stopPropagation()}>{!trash && <Check checked={on} indeterminate={!on && some} onChange={(v) => toggleSel(ids, v)} aria={f.name} />}</td>
                        <td style={{ maxWidth: 280 }}>
                          <div class="row gap2">
                            <FileStatus status={f.status} />
                            <span class="ellipsis" title={f.name}>
                              {f.name}
                            </span>
                          </div>
                        </td>
                        <td class="ellipsis" style={{ maxWidth: 160 }}>
                          {samples.join(', ') || <span class="faint">—</span>}
                        </td>
                        <td>
                          <span class="row gap1">
                            {dates[0] ? fmtTS(dates[0], false) : <span class="faint">—</span>}
                            {needs && <Icon name="calendarClock" size="sm" class="" title={t('files.needs_date')} />}
                          </span>
                        </td>
                        <td class="num">{f.items?.length || 0}</td>
                        <td>
                          <div class="tags">
                            {f.experiment && <span class="tag">{f.experiment}</span>}
                            {f.group && <span class="tag">{f.group}</span>}
                            {(f.tags || []).slice(0, 2).map((x) => (
                              <span class="tag">#{x}</span>
                            ))}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </div>
          {sel.length > 0 && !trash && <SelectionBar />}
        </div>
        <div class="panel">{current ? <FileDetail key={current.id} file={current} trash={trash} rel={rel.data} onClose={() => setOpen(null)} /> : <DetailHint count={files.data?.length || 0} />}</div>
      </div>
      {drag && (
        <div class="dropzone">
          <div class="box">
            <Icon name="fileUp" />
            <h3>{t('files.drop')}</h3>
            <p class="small muted">{t('files.drop.d')}</p>
          </div>
        </div>
      )}
    </>
  );
}

function FilterSelect(p: { label: string; value: string; onValue: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <label class="row gap1 small muted">
      {p.label}
      <select class="select sm" style={{ width: 150 }} value={p.value} onChange={(e) => p.onValue((e.target as HTMLSelectElement).value)} disabled={!p.options.length}>
        <option value="">{t('files.f.any')}</option>
        {p.options.map((o) => (
          <option value={o.value}>{o.label}</option>
        ))}
      </select>
    </label>
  );
}

export function FileStatus(p: { status: SourceFile['status'] }) {
  const icon = p.status === 'parsed' ? 'ok' : p.status === 'partial' ? 'warning' : 'alert';
  return (
    <span class={`status-ic ${p.status}`} data-tip={t('file.status.' + p.status)}>
      <Icon name={icon} size="sm" />
    </span>
  );
}

/** Measurement sets used before (by saved graphs and cycles). */
function SetsMenu() {
  const graphs = useAsync(() => get<{ id: string; title: string; kind: string; measurements: string[] }[] | null>('/api/graphs'), []);
  const cycles = useAsync(() => get<{ id: string; config: { name: string }; measurements: string[] }[] | null>('/api/cycles'), []);
  const sets = [
    ...(graphs.data || []).filter((g) => g.measurements?.length).map((g) => ({ label: g.title || t('graph.kind.' + g.kind), icon: 'chart' as const, ids: g.measurements })),
    ...(cycles.data || []).filter((c) => c.measurements?.length).map((c) => ({ label: c.config.name, icon: 'cycle' as const, ids: c.measurements })),
  ];
  const [open, setOpen] = useState(false);
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  if (!sets.length) return null;
  return (
    <>
      <Button size="sm" icon="layers" btnRef={(el: HTMLButtonElement | null) => setAnchor(el)} onClick={() => setOpen(!open)} tip={t('files.sets.tip')}>
        {t('files.sets')}
      </Button>
      {open && anchor && (
        <MenuLazy
          anchor={anchor}
          onClose={() => setOpen(false)}
          items={[{ heading: t('files.sets.title') }, ...sets.slice(0, 20).map((s) => ({ label: s.label, icon: s.icon, meta: t('files.n_meas', { n: s.ids.length }), run: () => setSel(s.ids) }))]}
        />
      )}
    </>
  );
}


function SelectionBar() {
  const sel = useStore(selection, (s) => s.ids);
  return (
    <div style={{ padding: 'var(--s3)', borderTop: '1px solid var(--border)' }}>
      <div class="selbar">
        <Icon name="listChecks" size="sm" />
        <b>{t('files.selected', { n: sel.length })}</b>
        <span class="spacer" />
        <Button size="sm" kind="primary" icon="chart" onClick={() => navigate('/ls/graphs/new?m=' + sel.join(','))}>
          {sel.length > 1 ? t('files.compare') : t('files.graph')}
        </Button>
        <Button size="sm" icon="cycle" onClick={() => openPanel('cycle-wizard', { measurements: sel })}>
          {t('files.cycle')}
        </Button>
        <Button size="sm" icon="download" onClick={() => openPanel('export', { items: [{ kind: 'dataset', ids: sel }] })}>
          {t('ui.export')}
        </Button>
        <Button size="sm" kind="ghost" icon="x" tip={t('files.clear_sel')} onClick={() => setSel([])} />
      </div>
    </div>
  );
}

function DetailHint(p: { count: number }) {
  return (
    <div class="panel-body center" style={{ padding: 'var(--s6)' }}>
      <div class="col gap3" style={{ alignItems: 'center', textAlign: 'center', maxWidth: 280 }}>
        <span class="modal-icon">
          <Icon name="file" />
        </span>
        <b>{t('files.detail.hint')}</b>
        <p class="small muted">{t('files.detail.hint.d', { n: p.count })}</p>
      </div>
    </div>
  );
}

function LsEmpty() {
  return (
    <div class="panel" style={{ flex: 1 }}>
      <div class="panel-body center">
        <Empty icon="microscope" title={t('ls.empty.title')} body={t('ls.empty.body')}>
          <Button kind="primary" icon="upload" size="lg" onClick={importFiles}>
            {t('ls.import')}
          </Button>
          <Button kind="ghost" icon="book" onClick={() => openPanel('help', 'ls')}>
            {t('ls.empty.learn')}
          </Button>
        </Empty>
        <div class="row gap6 small faint" style={{ justifyContent: 'center', marginTop: 'calc(-1 * var(--s4))', paddingBottom: 'var(--s8)' }}>
          <span class="row gap1">
            <Icon name="file" size="sm" />
            {t('ls.empty.what')}
          </span>
          <span class="row gap1">
            <Icon name="flask" size="sm" />
            {t('ls.empty.instrument')}
          </span>
          <span class="row gap1">
            <Icon name="chart" size="sm" />
            {t('ls.empty.produces')}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---- file detail ----

function FileDetail(p: { file: FileView; trash: boolean; rel?: { files: Record<string, { graphs: string[] | null; cycles: string[] | null }> } | null; onClose: () => void }) {
  const f = p.file;
  const [tab, setTab] = useState<'overview' | 'measurements' | 'original'>('overview');
  const rev = useStore(app, (s) => s.libraryRev);
  const full = useAsync(() => get<{ file: SourceFile; measurements: Measurement[] | null }>('/api/files/' + f.id), [f.id, rev]);
  const r = p.rel?.files[f.id];
  const used = (r?.graphs?.length || 0) + (r?.cycles?.length || 0);
  const sel = useStore(selection, (s) => s.ids);

  const trash = async () => {
    if (used) {
      const ok = await confirmDialog({ title: t('files.trash.q', { name: f.name }), body: t('files.trash.used', { g: r?.graphs?.length || 0, c: r?.cycles?.length || 0 }), confirm: t('files.trash.go'), danger: true, icon: 'trash' });
      if (!ok) return;
    }
    const done = await run(() => post(`/api/files/${f.id}/trash`, { trashed: true }));
    if (done !== undefined) {
      toggleSel((f.items || []).map((m) => m.id), false);
      p.onClose();
      toast('info', t('files.trashed', { name: f.name }), { label: t('ui.undo'), run: () => run(() => post(`/api/files/${f.id}/trash`, { trashed: false })) });
    }
  };
  const restore = async () => {
    const done = await run(() => post(`/api/files/${f.id}/trash`, { trashed: false }));
    if (done !== undefined) {
      p.onClose();
      toast('success', t('files.restored'));
    }
  };
  const destroy = async () => {
    const ok = await confirmDialog({ title: t('files.delete.q'), body: used ? t('files.delete.used', { g: r?.graphs?.length || 0, c: r?.cycles?.length || 0 }) : t('files.delete.body'), confirm: t('files.delete.go'), danger: true });
    if (!ok) return;
    const done = await run(() => api('DELETE', `/api/files/${f.id}`));
    if (done !== undefined) p.onClose();
  };

  return (
    <>
      <div class="panel-head">
        <FileStatus status={f.status} />
        <h3 class="grow ellipsis small" title={f.name} style={{ fontSize: 'var(--fs-md)' }}>
          {f.name}
        </h3>
        <Button kind="ghost" size="sm" icon="x" tip={t('ui.close')} onClick={p.onClose} />
      </div>
      <Tabs
        value={tab}
        onValue={setTab}
        class=""
        tabs={[
          { value: 'overview', label: t('files.t.overview') },
          { value: 'measurements', label: t('files.t.measurements'), count: f.items?.length || 0 },
          { value: 'original', label: t('files.t.original') },
        ]}
      />
      <div class="panel-body" style={{ padding: 'var(--s4)' }}>
        {tab === 'overview' && (
          <div class="col gap4">
            {f.status !== 'parsed' && (
              <Notice kind={f.status === 'failed' ? 'danger' : 'warning'}>
                <strong>{t('file.status.' + f.status)}</strong>
                <div>{f.error ? errText(f.error) : t('files.partial.d')}</div>
              </Notice>
            )}
            {!!f.warnings?.length && (
              <div class="col gap1">
                <span class="section-title">{t('files.warnings')}</span>
                <ul class="small muted" style={{ margin: 0, paddingLeft: 18 }}>
                  {f.warnings.map((w) => (
                    <li>{warnText(w)}</li>
                  ))}
                </ul>
              </div>
            )}
            <OrgEditor file={f} disabled={p.trash} />
            <div class="col gap2">
              <span class="section-title">{t('files.provenance')}</span>
              <dl class="kv">
                <dt>{t('files.k.imported')}</dt>
                <dd>{fmtDate(f.importedAt, true)}</dd>
                <dt>{t('files.k.size')}</dt>
                <dd>{fmtBytes(f.size)}</dd>
                <dt>{t('files.k.format')}</dt>
                <dd>{[t('files.format.' + f.format) !== 'files.format.' + f.format ? t('files.format.' + f.format) : f.format, f.encoding, f.delimiter && t('files.delim.' + f.delimiter), f.decimal && t('files.decimal', { d: f.decimal })].filter(Boolean).join(' · ')}</dd>
                <dt>{t('col.parser')}</dt>
                <dd class="mono xs">{f.parser}</dd>
                <dt>{t('col.spec')}</dt>
                <dd class="mono xs">{f.spec}</dd>
                <dt>SHA-256</dt>
                <dd class="mono xs" title={f.sha256}>
                  {f.sha256.slice(0, 16)}…
                </dd>
              </dl>
            </div>
            {r && used > 0 && (
              <div class="col gap2">
                <span class="section-title">{t('files.used_by')}</span>
                <UsedBy graphs={r.graphs || []} cycles={r.cycles || []} />
              </div>
            )}
          </div>
        )}
        {tab === 'measurements' && (
          <div class="col gap2">
            {full.loading && !full.data && <Skeleton h={120} />}
            {(f.items || []).map((m) => (
              <MeasurementCard m={m} file={f} full={full.data?.measurements?.find((x) => x.id === m.id)} selected={sel.includes(m.id)} disabled={p.trash} />
            ))}
            {!f.items?.length && <p class="small muted">{t('files.no_measurements')}</p>}
          </div>
        )}
        {tab === 'original' && <OriginalViewer file={f} />}
      </div>
      <div class="modal-foot" style={{ padding: 'var(--s3) var(--s4)', flexWrap: 'wrap' }}>
        {p.trash ? (
          <>
            <Button size="sm" kind="danger" icon="trash" onClick={destroy}>
              {t('files.delete.go')}
            </Button>
            <span class="spacer" />
            <Button size="sm" kind="primary" icon="undo" onClick={restore}>
              {t('ui.restore')}
            </Button>
          </>
        ) : (
          <>
            <Button size="sm" kind="ghost" icon="trash" tip={t('files.trash.go')} onClick={trash} />
            <Button size="sm" kind="ghost" icon="download" tip={t('ui.export')} onClick={() => openPanel('export', { items: [{ kind: 'file', id: f.id }] })} />
            <span class="spacer" />
            {(f.items || []).length > 0 && (
              <>
                <Button size="sm" icon="cycle" onClick={() => openPanel('cycle-wizard', { measurements: (f.items || []).map((m) => m.id) })}>
                  {t('files.cycle')}
                </Button>
                <Button size="sm" kind="primary" icon="chart" onClick={() => navigate('/ls/graphs/new?m=' + (f.items || []).map((m) => m.id).join(','))}>
                  {t('files.graph')}
                </Button>
              </>
            )}
          </>
        )}
      </div>
    </>
  );
}


export function UsedBy(p: { graphs: string[]; cycles: string[] }) {
  const graphs = useAsync(() => get<{ id: string; title: string; kind: string }[] | null>('/api/graphs'), [p.graphs.join()]);
  const cycles = useAsync(() => get<{ id: string; config: { name: string } }[] | null>('/api/cycles'), [p.cycles.join()]);
  return (
    <div class="col gap1">
      {p.graphs.map((id) => {
        const g = graphs.data?.find((x) => x.id === id);
        return (
          <Button size="sm" kind="ghost" icon="chart" class="row-btn" onClick={() => navigate('/ls/graphs/' + id)}>
            <span class="ellipsis">{g ? g.title || t('graph.kind.' + g.kind) : '…'}</span>
          </Button>
        );
      })}
      {p.cycles.map((id) => {
        const c = cycles.data?.find((x) => x.id === id);
        return (
          <Button size="sm" kind="ghost" icon="cycle" class="row-btn" onClick={() => navigate('/ls/cycles/' + id)}>
            <span class="ellipsis">{c ? c.config.name : '…'}</span>
          </Button>
        );
      })}
    </div>
  );
}

function OrgEditor(p: { file: FileView; disabled?: boolean }) {
  const f = p.file;
  const [ex, setEx] = useState(f.experiment || '');
  const [gr, setGr] = useState(f.group || '');
  const [tags, setTags] = useState<string[]>(f.tags || []);
  const [tag, setTag] = useState('');
  const [notes, setNotes] = useState(f.notes || '');
  const [busy, setBusy] = useState(false);
  const dirty = ex !== (f.experiment || '') || gr !== (f.group || '') || notes !== (f.notes || '') || tags.join('\u0000') !== (f.tags || []).join('\u0000');
  const save = async () => {
    setBusy(true);
    const r = await run(() => patch(`/api/files/${f.id}`, { experiment: ex, group: gr, tags, notes }));
    setBusy(false);
    if (r !== undefined) toast('success', t('ui.saved'));
  };
  const addTag = () => {
    const v = tag.trim().replace(/^#/, '');
    if (v && !tags.includes(v)) setTags([...tags, v]);
    setTag('');
  };
  return (
    <div class="col gap3">
      <span class="section-title">{t('files.organize')}</span>
      <div class="row gap2" style={{ alignItems: 'flex-start' }}>
        <Field label={t('files.f.experiment')} class="grow">
          <Input size="sm" value={ex} onValue={setEx} maxLength={120} disabled={p.disabled} />
        </Field>
        <Field label={t('files.f.group')} class="grow">
          <Input size="sm" value={gr} onValue={setGr} maxLength={120} disabled={p.disabled} />
        </Field>
      </div>
      <Field label={t('files.f.tags')}>
        <div class="col gap2">
          {tags.length > 0 && (
            <div class="tags">
              {tags.map((x) => (
                <span class="tag">
                  #{x}
                  {!p.disabled && (
                    <button aria-label={t('ui.remove')} onClick={() => setTags(tags.filter((y) => y !== x))}>
                      <Icon name="x" />
                    </button>
                  )}
                </span>
              ))}
            </div>
          )}
          {!p.disabled && <Input size="sm" icon="tag" value={tag} onValue={setTag} placeholder={t('files.tag.add')} onKeyDown={(e: KeyboardEvent) => (e.key === 'Enter' || e.key === ',') && (e.preventDefault(), addTag())} onBlur={addTag} />}
        </div>
      </Field>
      <Field label={t('files.f.notes')}>
        <textarea class="textarea" rows={2} value={notes} maxLength={4000} disabled={p.disabled} onInput={(e) => setNotes((e.target as HTMLTextAreaElement).value)} />
      </Field>
      {dirty && (
        <div class="row">
          <span class="spacer" />
          <Button size="sm" kind="ghost" onClick={() => (setEx(f.experiment || ''), setGr(f.group || ''), setTags(f.tags || []), setNotes(f.notes || ''))}>
            {t('ui.cancel')}
          </Button>
          <Button size="sm" kind="primary" busy={busy} onClick={save}>
            {t('ui.save')}
          </Button>
        </div>
      )}
    </div>
  );
}

function MeasurementCard(p: { m: MeasurementSummary; file: FileView; full?: Measurement; selected: boolean; disabled?: boolean }) {
  const m = p.m;
  const [edit, setEdit] = useState(false);
  const [dateOpen, setDateOpen] = useState(false);
  const needs = !m.measuredAt || (m.measuredAt.ambiguous && !m.measuredAt.confirmed);
  return (
    <div class={`meas ${p.selected ? 'on' : ''}`}>
      <div class="row gap2">
        {!p.disabled && <Check checked={p.selected} onChange={(v) => toggleSel([m.id], v)} aria={measurementName(m, p.file)} />}
        <b class="grow ellipsis small">{measurementName(m, p.file)}</b>
        {m.replicate ? <Badge>{t('ls.replicate_n', { n: m.replicate })}</Badge> : null}
        {!p.disabled && <Button size="sm" kind="ghost" icon="pencil" tip={t('ui.edit')} onClick={() => setEdit(true)} />}
      </div>
      <div class="row gap2 xs muted">
        <Icon name="calendar" size="sm" />
        {m.measuredAt ? (
          <span data-tip={m.measuredAt.tzKnown ? undefined : t('ls.tz_unknown')}>
            {fmtTS(m.measuredAt)}
            {m.measuredAt.source === 'user' && ' · ' + t('ls.date_by_user')}
          </span>
        ) : (
          <span>{t('ls.no_date')}</span>
        )}
        {needs && !p.disabled && (
          <Button size="sm" kind="ghost" class="" icon="calendarClock" onClick={() => setDateOpen(true)}>
            {t('ls.confirm_date')}
          </Button>
        )}
      </div>
      <div class="params">
        {PARAMS.map((k) => (
          <div class="param">
            <div class="k">{t('param.' + k)}</div>
            <div class="v">{m.params[k] ? fmtQ(m.params[k]) : <span class="faint">—</span>}</div>
          </div>
        ))}
      </div>
      <div class="row gap1 xs faint wrap">
        {m.bins > 0 ? t('ls.bins', { n: m.bins }) : t('ls.no_distribution')}
        {m.weightings?.length ? ' · ' + m.weightings.map((w) => t('axis.' + w)).join(', ') : ''}
        {m.condition && ' · ' + m.condition}
        {p.full?.distribution && ' · ' + t('ls.lines', { a: p.full.distribution.firstLine, b: p.full.distribution.lastLine })}
      </div>
      {edit && <MeasurementEdit m={m} onClose={() => setEdit(false)} />}
      {dateOpen && <DateConfirm m={m} onClose={() => setDateOpen(false)} />}
    </div>
  );
}

function MeasurementEdit(p: { m: MeasurementSummary; onClose: () => void }) {
  const m = p.m;
  const [label, setLabel] = useState(m.label || '');
  const [sample, setSample] = useState(m.sampleId || '');
  const [rep, setRep] = useState(m.replicate ? String(m.replicate) : '');
  const [cond, setCond] = useState(m.condition || '');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    const r = await run(() => patch(`/api/measurements/${m.id}`, { label, sampleId: sample, replicate: rep ? Math.max(0, Math.min(999, parseInt(rep, 10) || 0)) : 0, condition: cond }));
    setBusy(false);
    if (r !== undefined) p.onClose();
  };
  return (
    <Modal
      title={t('meas.edit')}
      sub={t('meas.edit.d')}
      icon="pencil"
      size="narrow"
      onClose={p.onClose}
      foot={
        <>
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" busy={busy} onClick={save}>
            {t('ui.save')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <Field label={t('meas.label')} hint={t('meas.label.hint')}>
          <Input value={label} onValue={setLabel} maxLength={120} autoFocus />
        </Field>
        <Field label={t('col.sample_id')}>
          <Input value={sample} onValue={setSample} maxLength={120} />
        </Field>
        <div class="row gap3">
          <Field label={t('col.replicate')} class="grow">
            <Input value={rep} onValue={(v) => setRep(v.replace(/\D/g, ''))} inputMode="numeric" maxLength={3} />
          </Field>
          <Field label={t('col.condition')} class="grow">
            <Input value={cond} onValue={setCond} maxLength={120} />
          </Field>
        </div>
      </div>
    </Modal>
  );
}

function DateConfirm(p: { m: MeasurementSummary; onClose: () => void }) {
  const ts = p.m.measuredAt;
  const [v, setV] = useState(ts ? toLocalInput(ts.time, !ts.tzKnown) : '');
  const [busy, setBusy] = useState(false);
  const save = async (asIs: boolean) => {
    setBusy(true);
    const body = asIs ? { confirmDate: true } : { measuredAt: v + ':00Z' };
    const r = await run(() => patch(`/api/measurements/${p.m.id}`, body));
    setBusy(false);
    if (r !== undefined) p.onClose();
  };
  return (
    <Modal
      title={t('date.title')}
      icon="calendarClock"
      size="narrow"
      onClose={p.onClose}
      foot={
        <>
          {ts && (
            <Button kind="ghost" busy={busy} onClick={() => save(true)}>
              {t('date.keep')}
            </Button>
          )}
          <span class="spacer" />
          <Button kind="primary" busy={busy} disabled={!v} onClick={() => save(false)}>
            {t('date.set')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <p class="small muted">{ts ? (ts.ambiguous ? t('date.ambiguous', { raw: ts.raw }) : t('date.check', { raw: ts.raw })) : t('date.missing')}</p>
        <Field label={t('date.when')} hint={t('date.hint')}>
          <input type="datetime-local" class="input" value={v} onInput={(e) => setV((e.target as HTMLInputElement).value)} autoFocus />
        </Field>
        <Notice kind="info">{t('date.not_import')}</Notice>
      </div>
    </Modal>
  );
}

function OriginalViewer(p: { file: SourceFile }) {
  const text = useAsync(() => get<string>(`/api/files/${p.file.id}/original`, { text: true }), [p.file.id]);
  const lines = useMemo(() => (text.data || '').split(/\r?\n/), [text.data]);
  return (
    <div class="col gap3" style={{ height: '100%' }}>
      <div class="row">
        <span class="xs faint grow">{t('orig.d')}</span>
        <a class="btn sm" href={`/api/files/${p.file.id}/original?download=1`} download={p.file.name}>
          <Icon name="download" size="sm" />
          {t('orig.download')}
        </a>
      </div>
      {text.loading ? (
        <Spinner label={t('ui.loading')} />
      ) : text.error ? (
        <Notice kind="danger">{errText(text.error)}</Notice>
      ) : (
        <div class="original" style={{ maxHeight: '100%' }}>
          {lines.slice(0, 5000).map((l) => (
            <div>{l || ' '}</div>
          ))}
          {lines.length > 5000 && <div class="faint">{t('orig.truncated', { n: lines.length - 5000 })}</div>}
        </div>
      )}
    </div>
  );
}
