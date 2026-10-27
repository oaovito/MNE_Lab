// Combine: a new graph built from the data of saved graphs (never from
// their images). The core checks every combination: different variables,
// units or X domains are refused, and a display conversion is offered only
// when every graph converts with an exact factor. The originals are only
// read; the new graph records Derived From each of them.
import { useEffect, useState } from 'preact/hooks';
import { errText, t } from '../../lib/i18n';
import { COMBINABLE, checkCombine, combineGraphs, combineKey } from '../../lib/groups';
import { useGraphs } from '../../lib/library';
import { variableLabel } from '../../lib/series';
import { navigate } from '../../lib/route';
import { toast } from '../../lib/state';
import type { CombineCheck, GraphDef } from '../../lib/types';
import { Badge, Button, Check, Field, Input, Modal, Notice, Select, Spinner } from '../../ui/kit';

const variableOf = (g: GraphDef) => (g.kind === 'dls_distribution' ? t('axis.' + (g.weighting || 'intensity')) : variableLabel(g.param || 'effective_diameter'));

export function CombineDialog(p: { ids: string[]; onClose: () => void }) {
  const graphs = useGraphs();
  const candidates = (graphs.data || []).filter((g) => COMBINABLE.includes(g.kind) && !g.trashed);
  const [sel, setSel] = useState<string[]>(p.ids);
  const [unit, setUnit] = useState('');
  const [title, setTitle] = useState('');
  const [titleTouched, setTitleTouched] = useState(false);
  const [ck, setCk] = useState<CombineCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const byId = (id: string) => candidates.find((g) => g.id === id);

  useEffect(() => {
    if (titleTouched) return;
    const names = sel.map((id) => byId(id)?.title || '').filter(Boolean);
    setTitle(names.length === 2 ? t('gc.default_title', { a: names[0], b: names[1] }) : names.join(' · '));
  }, [sel.join(), graphs.data]);

  useEffect(() => {
    let live = true;
    setErr('');
    if (sel.length < 2) {
      setCk(null);
      return;
    }
    setChecking(true);
    checkCombine(sel, unit)
      .then((c) => live && setCk(c))
      .catch((e) => live && setErr(errText((e as { code?: string }).code || 'graph.combine_incompatible')))
      .finally(() => live && setChecking(false));
    return () => {
      live = false;
    };
  }, [sel.join(), unit]);

  const toggle = (id: string) => {
    setSel(sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id]);
    setUnit('');
  };
  const create = async () => {
    setBusy(true);
    try {
      const r = await combineGraphs(sel, title.trim(), ck?.displayUnit || unit);
      toast('success', t('gc.created'));
      p.onClose();
      navigate('/ls/graphs/' + r.graph.id);
    } catch (e) {
      setErr(errText((e as { code?: string }).code || 'graph.combine_incompatible'));
    } finally {
      setBusy(false);
    }
  };

  const units = ck?.units.map((u) => u || t('gb.y_unit.none')).join(', ') || '';
  const offer = !!ck && ck.units.length > 1 && !!ck.conversions?.length;
  return (
    <Modal
      title={t('gc.title')}
      sub={t('gc.d')}
      icon="layers"
      size="wide"
      onClose={p.onClose}
      bodyClass="combine-dialog"
      foot={
        <>
          <span class="spacer" />
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" icon="layers" class="gc-create" busy={busy} disabled={!ck?.compatible || checking || !title.trim()} onClick={create}>
            {t('gc.create')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <div class="col gap1">
          <span class="section-title">{t('gc.pick', { n: sel.length })}</span>
          <span class="xs faint">{t('gc.pick.d')}</span>
        </div>
        <div class="gc-list" role="group" aria-label={t('gc.pick', { n: sel.length })}>
          {candidates.length < 2 && <span class="small faint">{t('gc.none')}</span>}
          {candidates.map((g) => (
            <label class={`gc-row ${sel.includes(g.id!) ? 'on' : ''}`} key={g.id}>
              <Check checked={sel.includes(g.id!)} onChange={() => toggle(g.id!)} aria={g.title || t('graph.kind.' + g.kind)} />
              <span class="grow ellipsis small">{g.title || t('graph.kind.' + g.kind)}</span>
              <Badge>{t('graph.kind.' + g.kind)}</Badge>
              <span class="xs muted ellipsis gc-var">{variableOf(g)}</span>
            </label>
          ))}
        </div>
        {checking && <Spinner label={t('gc.checking')} />}
        {err && <Notice kind="danger">{err}</Notice>}
        {ck && !checking && (
          <div class="col gap2 gc-verdict" data-compatible={ck.compatible ? 'yes' : 'no'}>
            {ck.compatible ? (
              <Notice kind="success">{t('gc.ok', { kind: t('graph.kind.' + ck.kind), variable: ck.variable ? (ck.kind === 'dls_distribution' ? t('axis.' + ck.variable) : variableLabel(ck.variable)) : '—', unit: ck.displayUnit || units || '—' })}</Notice>
            ) : (
              <Notice kind="danger">
                <div class="col gap1">
                  <b>{t('gc.refused')}</b>
                  {ck.problems.map((c) => (
                    <span class="small gc-problem" data-code={c} key={c}>
                      {t(combineKey(c), { units })}
                    </span>
                  ))}
                </div>
              </Notice>
            )}
            {offer && (
              <Field label={t('gc.convert')} hint={t('gc.convert.d')}>
                <Select label={t('gc.convert')} value={unit} onValue={setUnit} options={[{ value: '', label: t('gc.convert.none') }, ...ck.conversions!.map((u) => ({ value: u, label: u }))]} />
              </Field>
            )}
            {ck.notes.map((c) => (
              <span class="xs muted gc-note" key={c}>
                {t(combineKey(c), { unit: ck.displayUnit || unit })}
              </span>
            ))}
          </div>
        )}
        <Field label={t('gc.name')}>
          <Input
            value={title}
            onValue={(v) => {
              setTitle(v);
              setTitleTouched(true);
            }}
            maxLength={160}
          />
        </Field>
      </div>
    </Modal>
  );
}
