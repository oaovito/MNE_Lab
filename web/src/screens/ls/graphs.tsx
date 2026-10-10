// Graph Library: every saved graph with its preview, kind, sources, cycle
// and tags. Graphs are stored as reproducible definitions, never as images.
import { useState } from 'preact/hooks';
import { api, post } from '../../lib/api';
import { fmtDate } from '../../lib/format';
import { errText, t } from '../../lib/i18n';
import { useCycles, useGraphs, useRelations } from '../../lib/library';
import { navigate } from '../../lib/route';
import { openPanel, run, toast } from '../../lib/state';
import type { GraphDef } from '../../lib/types';
import { Thumb } from '../../ui/common';
import { Icon } from '../../ui/icons';
import { Badge, Button, Check, confirmDialog, Empty, Input, Seg, Skeleton } from '../../ui/kit';

export async function presentGraphs(ids: string[], index = 0) {
  await run(() => post('/api/present', { action: 'start', graphs: ids, index }));
}

export function GraphLibrary() {
  const [trash, setTrash] = useState(false);
  const graphs = useGraphs(trash);
  const cycles = useCycles();
  const rel = useRelations();
  const [q, setQ] = useState('');
  const [kind, setKind] = useState('all');
  const [sel, setSel] = useState<string[]>([]);
  const list = (graphs.data || []).filter((g) => (kind === 'all' || g.kind === kind) && (!q || [g.title, g.notes, ...(g.tags || [])].join(' ').toLowerCase().includes(q.toLowerCase())));
  const cycleName = (id?: string) => cycles.data?.find((c) => c.id === id)?.config.name;
  const toggle = (id: string) => setSel(sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id]);
  // Measurements of the selected graphs, for a new comparison or cycle.
  const selMeasurements = [...new Set((graphs.data || []).filter((g) => sel.includes(g.id!)).flatMap((g) => g.measurements || []))];

  const trashSel = async () => {
    for (const id of sel) await run(() => post(`/api/graphs/${id}/trash`, { trashed: true }));
    const ids = sel;
    setSel([]);
    toast('info', t('graphs.trashed', { n: ids.length }), { label: t('ui.undo'), run: () => ids.forEach((id) => run(() => post(`/api/graphs/${id}/trash`, { trashed: false }))) });
  };
  const restoreSel = async () => {
    for (const id of sel) await run(() => post(`/api/graphs/${id}/trash`, { trashed: false }));
    setSel([]);
  };
  const deleteSel = async () => {
    const ok = await confirmDialog({ title: t('graphs.delete.q', { n: sel.length }), body: t('graphs.delete.d'), confirm: t('files.delete.go'), danger: true });
    if (!ok) return;
    for (const id of sel) await run(() => api('DELETE', `/api/graphs/${id}`));
    setSel([]);
  };

  return (
    <>
      <div class="toolbar">
        <div style={{ width: 'min(300px, 36vw)' }}>
          <Input icon="search" size="sm" value={q} onValue={setQ} placeholder={t('graphs.search')} aria-label={t('graphs.search')} />
        </div>
        <Seg
          size="sm"
          value={kind}
          onValue={setKind}
          label={t('graphs.kind')}
          options={[
            { value: 'all', label: t('ui.all') },
            { value: 'dls_distribution', label: t('graph.kind.dls_distribution') },
            { value: 'parameter_time', label: t('graph.kind.parameter_time') },
            { value: 'dls_by_time', label: t('graph.kind.dls_by_time') },
            { value: 'statistical_groups', label: t('graph.kind.statistical_groups') },
          ]}
        />
        <span class="spacer" />
        {sel.length > 0 &&
          (trash ? (
            <>
              <span class="small muted">{t('files.selected', { n: sel.length })}</span>
              <Button size="sm" kind="danger" icon="trash" onClick={deleteSel}>
                {t('files.delete.go')}
              </Button>
              <Button size="sm" icon="undo" onClick={restoreSel}>
                {t('ui.restore')}
              </Button>
            </>
          ) : (
            <>
              <span class="small muted">{t('graphs.n_selected', { n: sel.length })}</span>
              <Button size="sm" kind="ghost" icon="trash" tip={t('ui.trash')} onClick={trashSel} />
              {selMeasurements.length > 0 && (
                <>
                  <Button size="sm" icon="layers" onClick={() => navigate('/ls/graphs/new?m=' + selMeasurements.join(','))}>
                    {t('files.compare')}
                  </Button>
                  <Button size="sm" icon="cycle" onClick={() => openPanel('cycle-wizard', { measurements: selMeasurements })}>
                    {t('files.cycle')}
                  </Button>
                </>
              )}
              <Button size="sm" icon="download" onClick={() => openPanel('export', { items: sel.map((id) => ({ kind: 'graph', id })) })}>
                {t('ui.export')}
              </Button>
              <Button size="sm" kind="primary" icon="present" onClick={() => presentGraphs(sel)}>
                {t('pres.start')}
              </Button>
            </>
          ))}
        <Seg
          size="sm"
          value={trash ? 'trash' : 'lib'}
          onValue={(v) => (setTrash(v === 'trash'), setSel([]))}
          options={[
            { value: 'lib', label: t('graphs.library'), icon: 'chart' },
            { value: 'trash', label: t('ui.trash_bin'), icon: 'trash' },
          ]}
        />
      </div>
      <div class="panel" style={{ flex: 1 }}>
        <div class="panel-body" style={{ padding: 'var(--s4)' }}>
          {graphs.loading && !graphs.data ? (
            <div class="cards">
              {[1, 2, 3, 4].map(() => (
                <div class="gcard">
                  <Skeleton h={140} r="0" />
                  <div class="body">
                    <Skeleton w="70%" />
                    <Skeleton w="40%" h={10} />
                  </div>
                </div>
              ))}
            </div>
          ) : graphs.error ? (
            <Empty icon="alert" title={t('err.title')} body={errText(graphs.error)}>
              <Button onClick={graphs.reload}>{t('ui.retry')}</Button>
            </Empty>
          ) : list.length === 0 ? (
            trash ? (
              <Empty icon="trash" title={t('graphs.trash.empty')} body={t('files.trash.body')} />
            ) : (graphs.data || []).length === 0 ? (
              <Empty icon="chart" title={t('graphs.empty')} body={t('graphs.empty.d')}>
                <Button kind="primary" icon="files" onClick={() => navigate('/ls/files')}>
                  {t('graphs.empty.go')}
                </Button>
              </Empty>
            ) : (
              <Empty icon="search" title={t('files.none_match')} />
            )
          ) : (
            <div class="cards">
              {list.map((g) => (
                <GraphCard g={g} on={sel.includes(g.id!)} onToggle={() => toggle(g.id!)} cycle={cycleName(g.cycleId)} files={rel.data?.graphs[g.id!]?.files?.length || 0} trash={trash} />
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function GraphCard(p: { g: GraphDef; on: boolean; onToggle: () => void; cycle?: string; files: number; trash: boolean }) {
  const g = p.g;
  return (
    <div class={`gcard ${p.on ? 'on' : ''}`} role="button" tabIndex={0} onClick={() => (p.trash ? p.onToggle() : navigate('/ls/graphs/' + g.id))} onKeyDown={(e) => e.key === 'Enter' && navigate('/ls/graphs/' + g.id)}>
      <div class="thumb">
        <Thumb id={g.id!} updated={g.updated} />
      </div>
      <span class="sel" onClick={(e) => e.stopPropagation()}>
        <Check checked={p.on} onChange={p.onToggle} aria={t('ui.select')} />
      </span>
      <div class="body">
        <span class="title ellipsis">{g.title || t('graph.kind.' + g.kind)}</span>
        <div class="row gap1 wrap">
          <Badge>{t('graph.kind.' + g.kind)}</Badge>
          {p.cycle && (
            <Badge kind="accent" icon="cycle">
              {p.cycle}
            </Badge>
          )}
        </div>
        <span class="meta row gap2">
          <span class="row gap1">
            <Icon name="calendar" size="sm" />
            {fmtDate(g.updated)}
          </span>
          <span class="row gap1">
            <Icon name="file" size="sm" />
            {t('graphs.n_files', { n: p.files })}
          </span>
        </span>
        {g.notes && <span class="xs muted clamp2">{g.notes}</span>}
        {!!g.tags?.length && (
          <div class="tags">
            {g.tags.slice(0, 3).map((x) => (
              <span class="tag">#{x}</span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
