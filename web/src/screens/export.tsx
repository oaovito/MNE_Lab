// Smart Export: recognizes what is being exported, recommends formats
// without hiding the other valid ones, previews the result, names it
// usefully and never overwrites a file silently.
import { useEffect, useRef, useState } from 'preact/hooks';
import { get, post } from '../lib/api';
import { fmtBytes, fmtNum } from '../lib/format';
import { errText, t } from '../lib/i18n';
import { closePanel, run, toast } from '../lib/state';
import type { ExportPrefs, Figure } from '../lib/types';
import { FigureView } from '../plot/Figure';
import { DirPicker } from '../ui/dirpicker';
import { Icon, type IconName } from '../ui/icons';
import { providerName } from '../ui/providers';
import { Button, Check, Field, Input, Modal, Notice, Seg, Select, Skeleton, Switch, useAsync } from '../ui/kit';

type Item = { kind: string; id?: string; ids?: string[] };
type Format = { id: string; ext: string; group: string; vector?: boolean; lossy?: boolean; dpi?: boolean; alpha?: boolean; hint?: string; warning?: string };
type Choice = { preset: string; formats: string[] };
type Spec = { width: number; height: number; unit: string; dpi: number; background: string; backColor?: string; title: boolean; legend: boolean; metadata: boolean; grid: boolean; points: boolean; lineWidth: number; fontSize: number; margin: number; scale: number };
type Options = {
  options: { kind: string; sections: { id: string; formats: string[] }[]; presets: string[]; default: Choice };
  defaults: Record<string, Choice>;
  formats: Record<string, Format>;
  destinations: { id: string; path: string; create?: boolean }[] | null;
  prefs: ExportPrefs;
  presets: Record<string, Spec>;
};
type Estimate = { files: number; bytes: number; pixels?: [number, number]; dpi?: number; sizeMm?: [number, number]; name: string; zip: boolean; destination: string; exists?: boolean; nameAdjusted?: boolean; warnings?: string[] | null };
type Result = { saved?: { path: string; name: string; size: number }[]; conflict?: string; suggest?: string };

const presetIcon: Record<string, IconName> = { quick: 'rocket', presentation: 'present', publication: 'book', raw: 'table', package: 'package', custom: 'sliders' };
const figurePreset = (preset: string) => (preset === 'publication' ? 'publication' : preset === 'presentation' ? 'presentation' : 'screen');

export function ExportDialog(p: { arg: { items: Item[] } }) {
  const items = p.arg.items || [];
  const kind = items.length > 1 ? 'batch' : items[0]?.kind || 'graph';
  const o = useAsync(() => get<Options>('/api/export/options?kind=' + kind), [kind]);
  if (o.error)
    return (
      <Modal title={t('export.title')} icon="download" onClose={closePanel} size="narrow">
        <Notice kind="danger">{errText(o.error)}</Notice>
      </Modal>
    );
  if (!o.data)
    return (
      <Modal title={t('export.title')} icon="download" onClose={closePanel} size="xwide">
        <Skeleton h="100%" />
      </Modal>
    );
  return <Dialog items={items} kind={o.data.options.kind} o={o.data} />;
}

function Dialog(p: { items: Item[]; kind: string; o: Options }) {
  const o = p.o;
  const prefs = o.prefs || {};
  const remembered = prefs.formats?.[p.kind]?.split(',').filter((f) => compatible(o, f));
  const [preset, setPreset] = useState(prefs.preset && o.options.presets.includes(prefs.preset) ? prefs.preset : o.options.default.preset);
  const [formats, setFormats] = useState<string[]>(remembered?.length ? remembered : o.options.default.formats);
  const [custom, setCustom] = useState<Spec | null>(null);
  const [decimal, setDecimal] = useState(prefs.csvDecimal || '');
  const dests = o.destinations || [];
  const [dest, setDest] = useState(prefs.destination || dests[0]?.path || '');
  const [name, setName] = useState('');
  const [all, setAll] = useState(false);
  const [picking, setPicking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [conflict, setConflict] = useState<Result | null>(null);
  const [prev, setPrev] = useState<{ figure?: Figure; estimate: Estimate } | null>(null);
  const [perr, setPerr] = useState('');
  const seq = useRef(0);

  const hasFigure = formats.some((f) => o.formats[f]?.group === 'figure');
  // A research package holds many files; its summary names it instead of counting them.
  const onlyPackage = formats.length === 1 && formats[0] === 'package';
  const hasData = formats.some((f) => o.formats[f]?.group === 'data') || formats.includes('package');
  const baseSpec = o.presets[figurePreset(preset)];
  const req = (extra: Record<string, unknown> = {}) => ({ items: p.items, formats, preset, figure: custom || {}, data: { decimal }, destination: dest, name: name.trim(), ...extra });

  const choosePreset = (ps: string) => {
    setPreset(ps);
    if (ps !== 'custom') {
      setFormats((o.defaults[ps] || o.options.default).formats.filter((f) => compatible(o, f)));
      setCustom(null);
    }
  };
  const toggle = (f: string) => {
    const next = formats.includes(f) ? formats.filter((x) => x !== f) : [...formats, f];
    setFormats(next);
    const d = o.defaults[preset];
    if (!d || d.formats.join() !== next.join()) setPreset('custom');
  };

  // Preview and estimate follow every choice.
  const key = JSON.stringify(req());
  useEffect(() => {
    if (!formats.length) return setPrev(null);
    const n = ++seq.current;
    const timer = setTimeout(() => {
      post<{ figure?: Figure; estimate: Estimate }>('/api/export/preview', req())
        .then((v) => n === seq.current && (setPrev(v), setPerr('')))
        .catch((e) => n === seq.current && setPerr(e.code || 'app.internal_error'));
    }, 160);
    return () => clearTimeout(timer);
  }, [key]);

  const doExport = async (collision = '', rename?: string) => {
    setBusy(true);
    const r = await run(() => post<Result>('/api/export', req({ collision, ...(rename !== undefined ? { name: rename } : {}) })));
    setBusy(false);
    if (!r) return;
    if (r.conflict) return setConflict(r);
    setConflict(null);
    const s = r.saved?.[0];
    if (!s) return;
    closePanel();
    toast('success', t('export.done', { name: s.name, size: fmtBytes(s.size) }), { label: t('export.show_folder'), run: () => run(() => post('/api/export/reveal', { path: s.path })) });
  };

  const est = prev?.estimate;
  const warnings = (est?.warnings || []).filter((w, i, a) => a.indexOf(w) === i);
  const blocking = warnings.some((w) => !w.startsWith('export.warn.'));
  const recommended = o.options.sections.filter((s) => s.id !== 'all');
  const allSection = o.options.sections.find((s) => s.id === 'all');
  const destLabel = (d: { id: string; path: string }) => (d.id.startsWith('cloud.') ? t('dest.cloud', { name: providerName(d.id.slice(6)) }) : t('dest.' + d.id));
  const knownDest = dests.find((d) => d.path === dest);

  return (
    <Modal
      size="xwide"
      icon="download"
      title={t('export.title')}
      sub={t('export.kind.' + p.kind, { n: p.items.length })}
      onClose={closePanel}
      bodyClass="flush"
      foot={
        <>
          <span class="small muted ellipsis" style={{ maxWidth: '50%' }}>
            {est ? (onlyPackage ? t('export.one_package') : est.zip ? t('export.one_zip', { n: est.files }) : t('export.one_file')) : ''}
          </span>
          <span class="spacer" />
          <Button onClick={closePanel}>{t('ui.cancel')}</Button>
          <Button kind="primary" icon="download" busy={busy} disabled={!formats.length || blocking || !!perr} onClick={() => doExport()}>
            {t('ui.export')}
          </Button>
        </>
      }
    >
      <div class="export">
        <div class="col-l">
          <span class="section-title">{t('export.intent')}</span>
          <div class="intents">
            {o.options.presets.map((ps) => (
              <button type="button" class={`intent ${preset === ps ? 'on' : ''}`} onClick={() => choosePreset(ps)} aria-pressed={preset === ps}>
                <Icon name={presetIcon[ps] || 'sliders'} />
                <div class="col" style={{ gap: 0 }}>
                  <b>{t('export.preset.' + ps)}</b>
                  <p>{t('export.preset.' + ps + '.d')}</p>
                </div>
              </button>
            ))}
          </div>
        </div>

        <div class="col-c">
          {recommended.map((s) => (
            <FormatGroup title={t('export.sec.' + s.id)} ids={s.formats} o={o} selected={formats} onToggle={toggle} />
          ))}
          {allSection && (
            <>
              <Button size="sm" kind="ghost" trail={all ? 'down' : 'right'} onClick={() => setAll(!all)}>
                {t('export.sec.all')}
              </Button>
              {all && <FormatGroup ids={allSection.formats} o={o} selected={formats} onToggle={toggle} />}
            </>
          )}
          <div class="preview">
            {prev?.figure && hasFigure ? (
              <div class={`sheet ${custom?.background === 'transparent' ? 'transparent' : ''}`} style={{ aspectRatio: `${prev.figure.width} / ${prev.figure.height}`, width: `min(94cqw, calc(94cqh * ${prev.figure.width / prev.figure.height}))` }}>
                <FigureView figure={prev.figure} />
              </div>
            ) : (
              <div class="col gap2" style={{ alignItems: 'center', color: 'var(--text-3)', padding: 'var(--s6)', textAlign: 'center' }}>
                <Icon name={formats.includes('package') ? 'package' : hasData ? 'table' : 'file'} size="lg" />
                <span class="small">{formats.length ? formats.map((f) => t('fmt.' + f)).join(' · ') : t('export.pick_format')}</span>
                {formats.includes('package') && <span class="xs" style={{ maxWidth: 360 }}>{t('export.package.contents')}</span>}
              </div>
            )}
          </div>
          {formats.map((f) => o.formats[f]?.warning).filter(Boolean).length > 0 && (
            <Notice kind="warning" icon="info">
              {formats
                .map((f) => o.formats[f]?.warning)
                .filter(Boolean)
                .map((w) => (
                  <div>{t(w!)}</div>
                ))}
            </Notice>
          )}
        </div>

        <div class="col-r">
          <Field label={t('export.name')} hint={est?.exists ? t('export.name.exists') : est?.nameAdjusted ? t('export.name.adjusted') : undefined}>
            <Input value={name} onValue={setName} placeholder={est?.name || ''} maxLength={160} icon="file" />
          </Field>
          <div class="field">
            <span class="label">{t('export.destination')}</span>
            <div class="dir-list">
              {dests.map((d) => (
                <button type="button" onClick={() => setDest(d.path)} aria-pressed={dest === d.path} style={dest === d.path ? { background: 'var(--accent-soft)' } : undefined}>
                  <Icon name={d.id.startsWith('cloud.') ? 'cloud' : d.id === 'drive' ? 'usb' : 'folder'} size="sm" />
                  <span class="col grow" style={{ gap: 0, minWidth: 0 }}>
                    <span class="ellipsis">{destLabel(d)}</span>
                    <span class="xs faint ellipsis">{d.path}</span>
                  </span>
                  {dest === d.path && <Icon name="check" size="sm" class="on" />}
                </button>
              ))}
              {!knownDest && dest && (
                <button type="button" style={{ background: 'var(--accent-soft)' }}>
                  <Icon name="folderOpen" size="sm" />
                  <span class="xs ellipsis grow">{dest}</span>
                  <Icon name="check" size="sm" class="on" />
                </button>
              )}
            </div>
            <Button size="sm" icon="folderOpen" onClick={() => setPicking(true)}>
              {t('export.choose_folder')}
            </Button>
          </div>

          {hasData && (
            <Field label={t('export.decimal')} hint={t('export.decimal.hint')}>
              <Seg
                size="sm"
                value={decimal || '.'}
                onValue={setDecimal}
                options={[
                  { value: '.', label: t('export.decimal.point') },
                  { value: ',', label: t('export.decimal.comma') },
                ]}
              />
            </Field>
          )}

          {hasFigure && (
            <div class="group">
              <Switch checked={!!custom} onChange={(on) => (setCustom(on ? { ...baseSpec } : null), on && preset !== 'custom' && setPreset('custom'))} label={t('export.adjust_figure')} />
              {custom ? <FigureSettings s={custom} set={setCustom} /> : <span class="xs faint">{t('export.figure_auto')}</span>}
            </div>
          )}

          <div class="card pad col gap2" style={{ background: 'var(--surface-2)' }}>
            <span class="section-title">{t('export.summary')}</span>
            {perr ? (
              <span class="small" style={{ color: 'var(--danger)' }}>
                {errText(perr)}
              </span>
            ) : !est ? (
              <Skeleton h={60} />
            ) : (
              <>
                <Row k={t('export.s.files')} v={onlyPackage ? t('export.one_package') : est.zip ? t('export.s.zip', { n: est.files }) : String(est.files)} />
                <Row k={t('export.s.size')} v={'≈ ' + fmtBytes(est.bytes)} />
                {hasFigure && est.pixels && <Row k={t('export.s.pixels')} v={`${est.pixels[0]} × ${est.pixels[1]} px`} />}
                {hasFigure && est.sizeMm && <Row k={t('export.s.physical')} v={`${fmtNum(est.sizeMm[0], 0)} × ${fmtNum(est.sizeMm[1], 0)} mm · ${fmtNum(est.dpi || 0, 0)} dpi`} />}
                <Row k={t('export.s.name')} v={est.name} />
                {warnings
                  .filter((w) => !w.startsWith('export.warn.'))
                  .map((w) => (
                    <span class="xs" style={{ color: 'var(--danger)' }}>
                      {errText(w)}
                    </span>
                  ))}
                <span class="xs faint">{t('export.local_only')}</span>
              </>
            )}
          </div>
        </div>
      </div>
      {picking && (
        <DirPicker
          title={t('export.choose_folder')}
          start={dest}
          confirm={t('export.use_folder')}
          onClose={() => setPicking(false)}
          onPick={(d) => {
            setDest(d);
            setPicking(false);
          }}
        />
      )}
      {conflict && <Collision r={conflict} busy={busy} onClose={() => setConflict(null)} onChoice={(c, rename) => doExport(c, rename)} />}
    </Modal>
  );
}

function compatible(o: Options, f: string) {
  return o.options.sections.some((s) => s.formats.includes(f));
}

function Row(p: { k: string; v: string }) {
  return (
    <div class="row gap2 small">
      <span class="muted grow">{p.k}</span>
      <span class="ellipsis num" style={{ maxWidth: 190, textAlign: 'right' }} data-tip={p.v}>
        {p.v}
      </span>
    </div>
  );
}

function FormatGroup(p: { title?: string; ids: string[]; o: Options; selected: string[]; onToggle: (f: string) => void }) {
  return (
    <div class="col gap2">
      {p.title && <span class="section-title">{p.title}</span>}
      <div class="formats">
        {p.ids.map((id) => {
          const f = p.o.formats[id];
          const on = p.selected.includes(id);
          return (
            <button type="button" class={`fmt ${on ? 'on' : ''}`} onClick={() => p.onToggle(id)} aria-pressed={on} data-tip={f?.hint ? t(f.hint) : undefined}>
              <b>{t('fmt.' + id)}</b>
              <span>{t('fmt.' + id + '.d')}</span>
              {on && <Icon name="check" size="sm" class="check-mark" />}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function FigureSettings(p: { s: Spec; set: (s: Spec) => void }) {
  const s = p.s;
  const set = (patch: Partial<Spec>) => p.set({ ...s, ...patch });
  const num = (v: string, max: number) => Math.min(max, Math.max(0, parseFloat(v.replace(',', '.')) || 0));
  return (
    <div class="col gap3">
      <div class="row gap2">
        <Field label={t('export.f.width')}>
          <Input size="sm" value={String(s.width)} onValue={(v) => set({ width: num(v, 20000) })} inputMode="decimal" style={{ width: 80 }} />
        </Field>
        <Field label={t('export.f.height')}>
          <Input size="sm" value={String(s.height)} onValue={(v) => set({ height: num(v, 20000) })} inputMode="decimal" style={{ width: 80 }} />
        </Field>
        <Field label={t('export.f.unit')}>
          <Select size="sm" value={s.unit} onValue={(unit) => set({ unit })} options={['px', 'mm', 'cm', 'in'].map((u) => ({ value: u, label: u }))} />
        </Field>
      </div>
      <div class="row gap2">
        <Field label={t('export.f.dpi')}>
          <Select size="sm" value={String(s.dpi)} onValue={(v) => set({ dpi: +v })} options={['72', '96', '144', '150', '300', '600', '1200'].map((d) => ({ value: d, label: d }))} />
        </Field>
        <Field label={t('export.f.font')}>
          <Input size="sm" value={String(s.fontSize)} onValue={(v) => set({ fontSize: num(v, 48) })} inputMode="decimal" style={{ width: 64 }} />
        </Field>
        <Field label={t('export.f.line')}>
          <Input size="sm" value={String(s.lineWidth)} onValue={(v) => set({ lineWidth: num(v, 12) })} inputMode="decimal" style={{ width: 64 }} />
        </Field>
      </div>
      <Seg
        size="sm"
        value={s.background}
        onValue={(background) => set({ background })}
        label={t('export.f.background')}
        options={[
          { value: 'solid', label: t('export.f.white') },
          { value: 'transparent', label: t('export.f.transparent') },
        ]}
      />
      <div class="col gap2">
        <Check checked={s.title} onChange={(title) => set({ title })} label={t('export.f.title')} />
        <Check checked={s.legend} onChange={(legend) => set({ legend })} label={t('graph.legend')} />
        <Check checked={s.grid} onChange={(grid) => set({ grid })} label={t('graph.grid')} />
        <Check checked={s.metadata} onChange={(metadata) => set({ metadata })} label={t('graph.metadata')} />
      </div>
    </div>
  );
}

function Collision(p: { r: Result; busy: boolean; onClose: () => void; onChoice: (c: string, rename?: string) => void }) {
  const [renaming, setRenaming] = useState(false);
  const [n, setN] = useState(p.r.suggest || '');
  return (
    <Modal
      size="narrow"
      icon="warning"
      title={t('export.exists.q')}
      onClose={p.onClose}
      foot={
        renaming ? (
          <>
            <span class="spacer" />
            <Button onClick={() => setRenaming(false)}>{t('ui.back')}</Button>
            <Button kind="primary" disabled={!n.trim()} busy={p.busy} onClick={() => p.onChoice('', n.trim())}>
              {t('ui.export')}
            </Button>
          </>
        ) : (
          <>
            <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
            <span class="spacer" />
            <Button onClick={() => setRenaming(true)}>{t('export.exists.rename')}</Button>
            <Button kind="danger" busy={p.busy} onClick={() => p.onChoice('replace')}>
              {t('export.exists.replace')}
            </Button>
            <Button kind="primary" busy={p.busy} onClick={() => p.onChoice('keep_both')}>
              {t('export.exists.keep_both')}
            </Button>
          </>
        )
      }
    >
      {renaming ? (
        <Field label={t('export.name')}>
          <Input value={n} onValue={setN} autoFocus maxLength={160} />
        </Field>
      ) : (
        <div class="col gap2 muted">
          <span>{t('export.exists.d', { name: p.r.conflict || '' })}</span>
          {p.r.suggest && <span class="small">{t('export.exists.keep_hint', { name: p.r.suggest })}</span>}
        </div>
      )}
    </Modal>
  );
}
