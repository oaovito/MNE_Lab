// Measurement picker: choose measurements already in the File Library (no
// re-import), searchable by file, sample, date, experiment, group and tag.
import { useMemo, useState } from 'preact/hooks';
import { fmtQ, fmtTS } from '../../lib/format';
import { t } from '../../lib/i18n';
import { measurementName, useFiles } from '../../lib/library';
import { Button, Check, Empty, Input, Modal, Skeleton } from '../../ui/kit';

export function MeasurementPicker(p: { title: string; initial: string[]; onPick: (ids: string[]) => void; onClose: () => void; confirm?: string }) {
  const files = useFiles();
  const [q, setQ] = useState('');
  const [sel, setSel] = useState<string[]>(p.initial);
  const rows = useMemo(() => {
    const out: { id: string; name: string; file: string; when: string; ed: string; hay: string }[] = [];
    for (const f of files.data || [])
      for (const m of f.items || []) {
        const name = measurementName(m, f);
        out.push({
          id: m.id,
          name,
          file: f.name,
          when: m.measuredAt ? fmtTS(m.measuredAt) : '—',
          ed: m.params.effective_diameter ? fmtQ(m.params.effective_diameter) : '—',
          hay: [name, f.name, m.sampleId, m.experiment, f.experiment, m.group, f.group, ...(m.tags || []), ...(f.tags || []), m.measuredAt?.raw].filter(Boolean).join(' ').toLowerCase(),
        });
      }
    return out;
  }, [files.data]);
  const list = rows.filter((r) => !q || r.hay.includes(q.toLowerCase()));
  const toggle = (id: string, on: boolean) => setSel(on ? [...sel, id] : sel.filter((x) => x !== id));
  const allOn = list.length > 0 && list.every((r) => sel.includes(r.id));
  return (
    <Modal
      title={p.title}
      icon="listChecks"
      size="wide"
      onClose={p.onClose}
      foot={
        <>
          <span class="small muted">{t('files.selected', { n: sel.length })}</span>
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" disabled={!sel.length} onClick={() => p.onPick(sel)}>
            {p.confirm || t('ui.apply')}
          </Button>
        </>
      }
    >
      <div class="col gap3" style={{ minHeight: 360 }}>
        <Input icon="search" value={q} onValue={setQ} placeholder={t('files.search')} autoFocus />
        {files.loading && !files.data ? (
          <Skeleton h={200} />
        ) : !rows.length ? (
          <Empty icon="files" title={t('picker.empty')} />
        ) : (
          <div style={{ maxHeight: '52vh', overflow: 'auto', border: '1px solid var(--border)', borderRadius: 'var(--r)' }}>
            <table class="table">
              <thead>
                <tr>
                  <th style={{ width: 36 }}>
                    <Check checked={allOn} onChange={(on) => setSel(on ? [...new Set([...sel, ...list.map((r) => r.id)])] : sel.filter((id) => !list.some((r) => r.id === id)))} aria={t('files.select_all')} />
                  </th>
                  <th>{t('col.measurement')}</th>
                  <th>{t('col.source_file')}</th>
                  <th>{t('col.measured_at')}</th>
                  <th class="num">{t('param.short.effective_diameter')}</th>
                </tr>
              </thead>
              <tbody>
                {list.map((r) => (
                  <tr class={`clickable ${sel.includes(r.id) ? 'sel' : ''}`} onClick={() => toggle(r.id, !sel.includes(r.id))}>
                    <td onClick={(e) => e.stopPropagation()}>
                      <Check checked={sel.includes(r.id)} onChange={(v) => toggle(r.id, v)} aria={r.name} />
                    </td>
                    <td class="ellipsis" style={{ maxWidth: 220 }}>
                      {r.name}
                    </td>
                    <td class="ellipsis muted" style={{ maxWidth: 220 }}>
                      {r.file}
                    </td>
                    <td class="muted">{r.when}</td>
                    <td class="num">{r.ed}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Modal>
  );
}
