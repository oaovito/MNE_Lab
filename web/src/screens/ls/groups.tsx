// Groups and saved selections (addendum B §23-24): named references to
// measurements or series, reused for graphs, cycles and analyses. Deleting
// a group never touches its measurements.
import { useState } from 'preact/hooks';
import { fmtDate } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { deleteGroup, saveGroup, useDesigns, useGroups } from '../../lib/groups';
import { useRelations } from '../../lib/library';
import { navigate } from '../../lib/route';
import { openPanel, run, toast } from '../../lib/state';
import { useSeriesCatalog } from '../../lib/series';
import type { DesignView, Group, SeriesEntry } from '../../lib/types';
import { Icon } from '../../ui/icons';
import { Badge, Button, confirmDialog, Empty, Field, Input, Modal, Skeleton } from '../../ui/kit';

/** The head design that places members of the group, if any. */
const designFor = (g: Group, heads: DesignView[]) => heads.find((v) => v.design.assignments.some((a) => g.measurements.includes(a.measurementId)));

/** Where a graph of the group opens: its design by unit, its series, or its measurements. */
export function groupGraphRoute(g: Group, heads: DesignView[], catalog: SeriesEntry[]): string {
  const d = designFor(g, heads);
  if (d) return `/ls/graphs/new?kind=design_time&design=${d.design.id}&groups=${g.id}&title=${encodeURIComponent(g.name)}`;
  if (g.seriesIds?.length) {
    const first = catalog.find((s) => s.id === g.seriesIds![0]);
    const kind = first && first.kind !== 'curve' ? 'series_values' : 'dls_distribution';
    const v = first?.y.key ? (kind === 'series_values' ? '&param=' : '&weighting=') + encodeURIComponent(first.y.key) : '';
    return `/ls/graphs/new?kind=${kind}${v}&group=${g.id}&title=${encodeURIComponent(g.name)}`;
  }
  return `/ls/graphs/new?m=${g.measurements.join(',')}&title=${encodeURIComponent(g.name)}`;
}

export function groupAnalyzeRoute(g: Group, heads: DesignView[]): string {
  const d = designFor(g, heads);
  return `/ls/statistics/new?group=${g.id}` + (d ? '&design=' + d.design.id : '');
}

export function GroupLibrary() {
  const groups = useGroups();
  const rel = useRelations();
  const designs = useDesigns();
  const catalog = useSeriesCatalog();
  const heads = (designs.data || []).filter((v) => v.head);
  const [q, setQ] = useState('');
  const list = (groups.data || []).filter((g) => !q || [g.name, g.notes, ...(g.tags || [])].join(' ').toLowerCase().includes(q.toLowerCase()));
  const remove = async (g: Group) => {
    if (!(await confirmDialog({ title: t('grp.delete.q', { name: g.name }), body: t('grp.delete.d'), confirm: t('files.delete.go'), danger: true }))) return;
    if ((await run(() => deleteGroup(g.id!))) !== undefined) toast('info', t('grp.deleted'));
  };
  return (
    <>
      <div class="toolbar">
        <div style={{ width: 'min(300px, 36vw)' }}>
          <Input icon="search" size="sm" value={q} onValue={setQ} placeholder={t('grp.search')} aria-label={t('grp.search')} />
        </div>
        <span class="xs muted grow ellipsis">{t('grp.d')}</span>
      </div>
      <div class="panel groups-lib" style={{ flex: 1 }}>
        <div class="panel-body" style={{ padding: 'var(--s4)' }}>
          {groups.loading && !groups.data ? (
            <div class="col gap2">
              {[1, 2, 3].map(() => (
                <Skeleton h={56} />
              ))}
            </div>
          ) : groups.error ? (
            <Empty icon="alert" title={t('err.title')} body={errText(groups.error)} />
          ) : !list.length ? (
            (groups.data || []).length ? (
              <Empty icon="search" title={t('files.none_match')} />
            ) : (
              <Empty icon="layers" title={t('grp.empty')} body={t('grp.empty.d')}>
                <Button kind="primary" icon="files" onClick={() => navigate('/ls/files')}>
                  {t('graphs.empty.go')}
                </Button>
              </Empty>
            )
          ) : (
            <div class="col gap2">
              {list.map((g) => {
                const r = rel.data?.groups?.[g.id!];
                return (
                  <div class="card grp-card" key={g.id}>
                    <div class="row gap3 wrap">
                      <Icon name={g.kind === 'selection' ? 'listChecks' : 'layers'} />
                      <div class="col grow" style={{ gap: 2, minWidth: 180 }}>
                        <b class="ellipsis">{g.name}</b>
                        <span class="xs muted row gap2 wrap">
                          <Badge>{t('grp.kind.' + (g.kind || 'group'))}</Badge>
                          <span>{g.seriesIds?.length ? t('grp.series', { n: g.seriesIds.length }) + ' · ' : ''}{t('grp.members', { n: g.measurements.length })}</span>
                          {!!r?.graphs?.length && <span>{t('grp.used', { n: r.graphs.length })}</span>}
                          <span class="row gap1">
                            <Icon name="calendar" size="sm" />
                            {fmtDate(g.updated)}
                          </span>
                        </span>
                      </div>
                      <div class="row gap1 wrap grp-actions">
                        <Button size="sm" icon="chart" class="grp-graph" onClick={() => navigate(groupGraphRoute(g, heads, catalog.data?.series || []))}>
                          {t('grp.graph')}
                        </Button>
                        <Button size="sm" icon="cycle" onClick={() => openPanel('cycle-wizard', { measurements: g.measurements, groupId: g.id })}>
                          {t('grp.cycle')}
                        </Button>
                        <Button size="sm" icon="sigma" class="grp-analyze" onClick={() => navigate(groupAnalyzeRoute(g, heads))}>
                          {t('grp.analyze')}
                        </Button>
                        <Button size="sm" icon="download" onClick={() => openPanel('export', { items: [{ kind: 'group', id: g.id }] })}>
                          {t('ui.export')}
                        </Button>
                        <Button size="sm" kind="ghost" icon="trash" tip={t('files.delete.go')} onClick={() => remove(g)} />
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

/** SaveGroupDialog names a set of measurements; only their IDs are stored. */
export function SaveGroupDialog(p: { measurements: string[]; onClose: () => void }) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    const g = await run(() => saveGroup({ name: name.trim(), measurements: p.measurements }));
    setBusy(false);
    if (!g) return;
    toast('success', t('grp.saved', { name: g.name }));
    p.onClose();
  };
  return (
    <Modal
      title={t('grp.save')}
      sub={t('grp.members', { n: p.measurements.length })}
      icon="layers"
      size="narrow"
      onClose={p.onClose}
      foot={
        <>
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" icon="save" class="grp-save" busy={busy} disabled={!name.trim()} onClick={save}>
            {t('ui.save')}
          </Button>
        </>
      }
    >
      <div class="col gap2">
        <Field label={t('grp.name')}>
          <Input value={name} onValue={setName} placeholder={t('grp.name.ph')} maxLength={120} autoFocus onKeyDown={(e: KeyboardEvent) => e.key === 'Enter' && name.trim() && save()} />
        </Field>
        <span class="xs faint">{t('grp.d')}</span>
      </div>
    </Modal>
  );
}
