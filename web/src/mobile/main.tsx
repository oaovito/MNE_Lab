// MNE Lab on the phone: Viewer (state, graphs, cycles, presentations) and
// Remote Controller (presentation control, synchronization). Designed for
// touch: bottom navigation, full-width cards, large controls.
import '../i18n/phone';
import { render } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import '../styles/tokens.css';
import '../styles/base.css';
import '../styles/mobile.css';
import { fmtDate, fmtNum, fmtRel, setDecimal } from '../lib/format';
import { errText, setLang, t } from '../lib/i18n';
import { createStore, useStore } from '../lib/store';
import type { CycleView, Presentation } from '../lib/types';
import { Icon, type IconName } from '../ui/icons';
import { Link, LinkError, pair, saved } from './link';
import mark from '../../../assets/brand/mnelab-mark.svg';
import icon from '../../../assets/brand/mnelab-icon.svg';

// Embedded so it still shows after the computer has closed the connection.
const iconURL = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(icon);

type MState = {
  version: string;
  mode: 'portable' | 'temporary';
  lang: string;
  decimal: string;
  profile: string;
  color: number;
  sync: { state: string; pending: number; lastSync?: string };
  activities: { id: string; kind: string; started: string }[] | null;
  present: Presentation;
  files: number;
  graphs: number;
  cycles: number;
  measurements: number;
};
type MGraph = { id: string; title: string; kind: string; updated: string };
type MCycle = { id: string; name: string; sample?: string; points: number; assigned: number; updated: string };
type Rendered = { id: string; title: string; kind: string; svg: string; series: { id: string; label: string; color?: string; hidden?: boolean }[] };

const st = createStore<{ phase: 'pairing' | 'ready' | 'ended' | 'error'; error: string; s: MState | null; live: boolean; tab: string; graph: string | null; cycle: string | null; rev: number }>({
  phase: 'pairing',
  error: '',
  s: null,
  live: false,
  tab: 'home',
  graph: null,
  cycle: null,
  rev: 0,
});
let link: Link | null = null;

const seriesLabel = (l: string) => {
  if (l.startsWith('series.')) return t(l);
  const p = l.split(':'); // point:<unit>:<offset>[:<replicate>]
  if (p[0] === 'point' && p.length >= 3) return t('point.' + p[1], { n: p[2] }) + (p[3] ? ' · R' + p[3] : '');
  return l;
};

async function refresh() {
  if (!link) return;
  try {
    const s = await link.call<MState>('state');
    setLang(s.lang);
    setDecimal(s.decimal);
    st.set({ s, phase: 'ready' });
  } catch (e) {
    fail(e);
  }
}

function fail(e: unknown) {
  const code = e instanceof LinkError ? e.code : 'app.internal_error';
  if (code === 'mobile.session_ended' || link?.ended) st.set({ phase: 'ended', error: code });
  else if (code !== 'mobile.replayed') toast(errText(code));
}

let tt = 0;
const toastStore = createStore<{ text: string }>({ text: '' });
function toast(text: string) {
  toastStore.set({ text });
  clearTimeout(tt);
  tt = window.setTimeout(() => toastStore.set({ text: '' }), 3500);
}

async function boot() {
  const lang = navigator.language.toLowerCase().startsWith('pt') ? 'pt-BR' : navigator.language.toLowerCase().startsWith('es') ? 'es' : 'en';
  setLang(lang);
  const frag = new URLSearchParams(location.hash.slice(1));
  // The pairing secret leaves the address bar at once.
  if (location.hash) history.replaceState(null, '', location.pathname);
  let session = saved();
  try {
    if (frag.get('p') && frag.get('k')) session = await pair(frag.get('p')!, frag.get('k')!, deviceLabel());
  } catch (e) {
    st.set({ phase: 'error', error: e instanceof LinkError ? e.code : 'mobile.pairing_invalid' });
    return;
  }
  if (!session) return st.set({ phase: 'error', error: 'mobile.not_paired' });
  link = new Link(session);
  let timer = 0;
  link.events(
    (type, data) => {
      if (type === 'ended') {
        link?.end();
        st.set({ phase: 'ended', error: 'mobile.session_ended' });
        return;
      }
      if (type === 'present' && data) st.set((x) => ({ s: x.s ? { ...x.s, present: data } : x.s }));
      if (type === 'library') st.set((x) => ({ rev: x.rev + 1 }));
      clearTimeout(timer);
      timer = window.setTimeout(refresh, 150);
    },
    (live) => {
      st.set({ live });
      if (live) refresh();
      else if (!link?.ended) refresh();
    },
  );
  await refresh();
}

function deviceLabel() {
  const ua = navigator.userAgent;
  if (/iPhone/.test(ua)) return 'iPhone';
  if (/iPad/.test(ua)) return 'iPad';
  if (/Android/.test(ua)) return 'Android';
  return 'Smartphone';
}

// ---- screens ----

function App() {
  const phase = useStore(st, (x) => x.phase);
  const error = useStore(st, (x) => x.error);
  if (phase === 'pairing') return <Center icon="phone" title={t('m.pairing')} busy />;
  if (phase === 'error' || phase === 'ended') return <Center icon={phase === 'ended' ? 'logout' : 'qr'} title={phase === 'ended' ? t('m.ended') : t('m.cannot_pair')} body={phase === 'ended' ? t('m.ended.d') : errText(error) + ' ' + t('m.scan_again')} />;
  return <Main />;
}

function Center(p: { icon: IconName; title: string; body?: string; busy?: boolean }) {
  return (
    <div class="m-center">
      <img src={iconURL} alt="" width={56} height={56} />
      <h2>{p.title}</h2>
      {p.body && <p>{p.body}</p>}
      {p.busy && <span class="spin" />}
    </div>
  );
}

function Main() {
  const s = useStore(st, (x) => x.s)!;
  const tab = useStore(st, (x) => x.tab);
  const graph = useStore(st, (x) => x.graph);
  const cycle = useStore(st, (x) => x.cycle);
  const live = useStore(st, (x) => x.live);
  const text = useStore(toastStore, (x) => x.text);
  const presenting = s.present?.active;
  const tabs: { id: string; icon: IconName; label: string }[] = [
    { id: 'home', icon: 'grid', label: t('nav.home') },
    { id: 'graphs', icon: 'chart', label: t('ls.tab.graphs') },
    { id: 'cycles', icon: 'cycle', label: t('ls.tab.cycles') },
    { id: 'present', icon: 'present', label: t('m.control') },
  ];
  let body;
  if (tab === 'graphs' && graph) body = <GraphScreen id={graph} />;
  else if (tab === 'cycles' && cycle) body = <CycleScreen id={cycle} />;
  else if (tab === 'graphs') body = <Graphs />;
  else if (tab === 'cycles') body = <Cycles />;
  else if (tab === 'present') body = <Controller />;
  else body = <Home s={s} />;
  return (
    <div class="m-app">
      <header class="m-head">
        <img src={iconURL} alt="" width={24} height={24} />
        <b>
          MNE <span>Lab</span>
        </b>
        <span class="spacer" />
        <span class={`m-live ${live ? 'on' : ''}`} aria-label={live ? t('m.live') : t('m.reconnecting')}>
          <span class="dot" />
          {live ? t('m.live') : t('m.reconnecting')}
        </span>
      </header>
      <main class="m-main">{body}</main>
      {text && <div class="m-toast">{text}</div>}
      <nav class="m-nav" aria-label={t('nav.main')}>
        {tabs.map((x) => (
          <button type="button" class={tab === x.id ? 'on' : ''} aria-current={tab === x.id ? 'page' : undefined} onClick={() => st.set({ tab: x.id, graph: null, cycle: null })}>
            <span class="ic">
              <Icon name={x.icon} />
              {x.id === 'present' && presenting && <span class="badge-dot" />}
            </span>
            <span>{x.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}

function Home(p: { s: MState }) {
  const s = p.s;
  const acts = s.activities || [];
  const syncTone = s.sync.state === 'synced' || s.sync.state === 'local' ? 'ok' : s.sync.state === 'error' || s.sync.state === 'reconnect' ? 'err' : 'warn';
  return (
    <div class="m-stack">
      <div class="m-card m-hello">
        {/* The official generic avatar: the MNE Lab mark on the profile's color. */}
        <span class="m-avatar" style={{ background: `var(--pc${s.color % 8})` }} aria-hidden="true" dangerouslySetInnerHTML={{ __html: mark }} />
        <div>
          <b>{s.profile}</b>
          <span>{t('mode.' + s.mode)} · MNE Lab {s.version}</span>
        </div>
      </div>
      <div class="m-card">
        <div class="m-row">
          <Icon name={s.sync.state === 'local' ? 'drive' : s.sync.state === 'offline' ? 'cloudOff' : 'cloud'} />
          <div class="grow">
            <b>{t('sync.' + s.sync.state)}</b>
            <span>{s.sync.pending ? t('stor.pending.n', { n: s.sync.pending }) : s.sync.lastSync ? t('status.last_sync', { when: fmtRel(s.sync.lastSync) }) : ''}</span>
          </div>
          <span class={`m-tone ${syncTone}`} />
        </div>
        {s.sync.state !== 'local' && (
          <button type="button" class="m-btn" onClick={() => link?.call('sync').then(refresh, fail)}>
            <Icon name="refresh" size="sm" />
            {t('status.sync_now')}
          </button>
        )}
      </div>
      {acts.length > 0 && (
        <div class="m-card">
          {acts.map((a) => (
            <div class="m-row">
              <span class="spin" />
              <div class="grow">
                <b>{t('activity.' + a.kind)}</b>
                <span>{fmtRel(a.started)}</span>
              </div>
            </div>
          ))}
        </div>
      )}
      <div class="m-grid">
        <Stat icon="files" n={s.files} k={t('ls.tab.files')} />
        <Stat icon="table" n={s.measurements} k={t('my.measurements')} />
        <Stat icon="chart" n={s.graphs} k={t('ls.tab.graphs')} go={() => st.set({ tab: 'graphs' })} />
        <Stat icon="cycle" n={s.cycles} k={t('ls.tab.cycles')} go={() => st.set({ tab: 'cycles' })} />
      </div>
      {s.present?.active && (
        <button type="button" class="m-card m-cta" onClick={() => st.set({ tab: 'present' })}>
          <Icon name="present" />
          <div class="grow">
            <b>{t('m.presenting')}</b>
            <span>{t('m.slide', { n: s.present.index + 1, total: s.present.graphs?.length || 0 })}</span>
          </div>
          <Icon name="right" />
        </button>
      )}
      <p class="m-foot">{t('m.private')}</p>
    </div>
  );
}

function Stat(p: { icon: IconName; n: number; k: string; go?: () => void }) {
  return (
    <button type="button" class="m-stat" onClick={p.go} disabled={!p.go}>
      <Icon name={p.icon} size="sm" />
      <b>{p.n}</b>
      <span>{p.k}</span>
    </button>
  );
}

function useCall<T>(op: string, args: unknown, deps: unknown[]) {
  const rev = useStore(st, (x) => x.rev);
  const [v, setV] = useState<{ data?: T; error?: string }>({});
  useEffect(() => {
    if (!op) return;
    let alive = true;
    link
      ?.call<T>(op, args)
      .then((data) => alive && setV({ data }))
      .catch((e) => {
        if (!alive) return;
        setV({ error: e.code || 'app.internal_error' });
        fail(e);
      });
    return () => {
      alive = false;
    };
  }, [...deps, rev]);
  return v;
}

function Graphs() {
  const g = useCall<MGraph[]>('graphs', undefined, []);
  const [sel, setSel] = useState<string[]>([]);
  if (!g.data) return <Loading error={g.error} />;
  if (!g.data.length) return <Center icon="chart" title={t('graphs.empty')} body={t('m.graphs.empty')} />;
  const toggle = (id: string) => setSel(sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id]);
  return (
    <div class="m-stack">
      <h2 class="m-title">{t('ls.tab.graphs')}</h2>
      {g.data.map((x) => (
        <div class={`m-card m-item ${sel.includes(x.id) ? 'on' : ''}`}>
          <button type="button" class="m-check" aria-pressed={sel.includes(x.id)} aria-label={t('ui.select')} onClick={() => toggle(x.id)}>
            {sel.includes(x.id) && <Icon name="check" size="sm" />}
          </button>
          <button type="button" class="grow m-item-main" onClick={() => st.set({ graph: x.id })}>
            <b>{x.title || t('graph.kind.' + x.kind)}</b>
            <span>
              {t('graph.kind.' + x.kind)} · {fmtDate(x.updated)}
            </span>
          </button>
          <Icon name="right" size="sm" />
        </div>
      ))}
      {sel.length > 0 && (
        <div class="m-sticky">
          <button
            type="button"
            class="m-btn primary"
            onClick={() =>
              link?.call('present', { action: 'start', graphs: sel, index: 0 }).then(() => {
                setSel([]);
                st.set({ tab: 'present' });
              }, fail)
            }
          >
            <Icon name="present" size="sm" />
            {t('m.present_n', { n: sel.length })}
          </button>
        </div>
      )}
    </div>
  );
}

function useBox<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });
  useEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(([e]) => setSize({ w: Math.round(e.contentRect.width), h: Math.round(e.contentRect.height) }));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return [ref, size] as const;
}

function GraphView(p: { id: string; extra?: unknown; onData?: (r: Rendered) => void }) {
  const [ref, size] = useBox<HTMLDivElement>();
  const w = Math.max(240, size.w);
  const h = Math.max(180, Math.round(w * 0.72));
  // Rendered once the width is known, at the phone's own size.
  const r = useCall<Rendered>(size.w > 0 ? 'graph' : '', { id: p.id, width: w, height: h }, [p.id, Math.round(size.w / 40), JSON.stringify(p.extra || null)]);
  useEffect(() => {
    if (r.data) p.onData?.(r.data);
  }, [r.data]);
  return (
    <div class="m-paper" ref={ref} style={{ minHeight: h }}>
      {r.data ? <div class="m-svg" dangerouslySetInnerHTML={{ __html: r.data.svg }} /> : <Loading error={r.error} />}
    </div>
  );
}

function GraphScreen(p: { id: string }) {
  const [data, setData] = useState<Rendered | null>(null);
  const g = { data };
  return (
    <div class="m-stack">
      <button type="button" class="m-back" onClick={() => st.set({ graph: null })}>
        <Icon name="back" size="sm" />
        {t('ls.tab.graphs')}
      </button>
      <h2 class="m-title">{g.data?.title || (g.data ? t('graph.kind.' + g.data.kind) : '')}</h2>
      <GraphView id={p.id} onData={setData} />
      {g.data && (
        <div class="m-card">
          {g.data.series
            .filter((x) => !x.hidden)
            .map((x) => (
              <div class="m-row">
                <span class="m-swatch" style={{ background: x.color || 'var(--accent)' }} />
                <span class="grow ellipsis">{seriesLabel(x.label)}</span>
              </div>
            ))}
        </div>
      )}
      <button type="button" class="m-btn primary" onClick={() => link?.call('present', { action: 'start', graphs: [p.id], index: 0 }).then(() => st.set({ tab: 'present', graph: null }), fail)}>
        <Icon name="present" size="sm" />
        {t('m.present_this')}
      </button>
    </div>
  );
}

function Cycles() {
  const c = useCall<MCycle[]>('cycles', undefined, []);
  if (!c.data) return <Loading error={c.error} />;
  if (!c.data.length) return <Center icon="cycle" title={t('cycles.empty')} body={t('m.cycles.empty')} />;
  return (
    <div class="m-stack">
      <h2 class="m-title">{t('ls.tab.cycles')}</h2>
      {c.data.map((x) => (
        <button type="button" class="m-card m-item" onClick={() => st.set({ cycle: x.id })}>
          <Icon name="cycle" />
          <div class="grow m-item-main">
            <b>{x.name}</b>
            <span>{[x.sample, t('cycles.points', { n: x.points }), t('m.assigned', { n: x.assigned })].filter(Boolean).join(' · ')}</span>
          </div>
          <Icon name="right" size="sm" />
        </button>
      ))}
    </div>
  );
}

function CycleScreen(p: { id: string }) {
  const c = useCall<CycleView>('cycle', { id: p.id }, [p.id]);
  const [param, setParam] = useState('');
  if (!c.data) return <Loading error={c.error} />;
  const v = c.data;
  const k = param || v.config.params[0];
  const rows = v.series?.[k] || [];
  const unit = (v.items || []).find((m) => m.params[k]?.unit)?.params[k]?.unit || '';
  return (
    <div class="m-stack">
      <button type="button" class="m-back" onClick={() => st.set({ cycle: null })}>
        <Icon name="back" size="sm" />
        {t('ls.tab.cycles')}
      </button>
      <h2 class="m-title">{v.config.name}</h2>
      <span class="m-sub">
        {t('cycles.every', { n: v.config.interval, unit: t('unit.' + v.config.unit) })} · {t('cycles.duration', { n: v.config.duration, unit: t('unit.' + v.config.unit) })}
      </span>
      <div class="m-seg" role="tablist">
        {v.config.params.map((x) => (
          <button type="button" role="tab" aria-selected={x === k} class={x === k ? 'on' : ''} onClick={() => setParam(x)}>
            {t('param.short.' + x)}
          </button>
        ))}
      </div>
      <div class="m-card">
        {rows.map((r) => (
          <div class="m-row">
            <span class="m-pt">{t('point.' + v.config.unit, { n: r.offset })}</span>
            <span class="grow num" style={{ textAlign: 'right' }}>
              {r.missing || r.summary.mean === undefined ? <span class="faint">{t('cycle.status.missing')}</span> : `${fmtNum(r.summary.mean)}${r.summary.sd !== undefined ? ' ± ' + fmtNum(r.summary.sd) : ''} ${unit}`}
            </span>
            <span class="faint num" style={{ width: 36, textAlign: 'right' }}>
              n={r.summary.n}
            </span>
          </div>
        ))}
      </div>
      <p class="m-foot">{t('stats.definition')}</p>
    </div>
  );
}

function Controller() {
  const s = useStore(st, (x) => x.s)!;
  const pr = s.present;
  const [data, setData] = useState<Rendered | null>(null);
  const g = { data: data && data.id === pr?.graphs?.[pr.index] ? data : null };
  if (!pr?.active || !pr.graphs?.length) return <Center icon="present" title={t('m.no_presentation')} body={t('m.no_presentation.d')} />;
  const send = (action: string, extra: Record<string, unknown> = {}) => {
    navigator.vibrate?.(12);
    link?.call<Presentation>('present', { action, ...extra }).then((p) => st.set((x) => ({ s: x.s ? { ...x.s, present: p } : x.s })), fail);
  };
  return (
    <div class="m-stack">
      <div class="m-counter">
        <b>{t('m.slide', { n: pr.index + 1, total: pr.graphs.length })}</b>
        <span class="ellipsis">{g.data?.title || ''}</span>
      </div>
      <GraphView id={pr.graphs[pr.index]} extra={[pr.noLegend, pr.hidden]} onData={setData} />
      <div class="m-pad">
        <button type="button" class="m-big" disabled={pr.index <= 0} onClick={() => send('prev')} aria-label={t('pres.prev')}>
          <Icon name="left" />
        </button>
        <button type="button" class="m-big primary" disabled={pr.index >= pr.graphs.length - 1} onClick={() => send('next')} aria-label={t('pres.next')}>
          <Icon name="right" />
        </button>
      </div>
      <div class="m-card">
        <div class="m-row">
          <Icon name="list" size="sm" />
          <span class="grow">{t('pres.legend')}</span>
          <button type="button" class={`m-switch ${pr.noLegend ? '' : 'on'}`} role="switch" aria-checked={!pr.noLegend} onClick={() => send('legend')} />
        </div>
        {(g.data?.series || [])
          .filter((x) => !x.hidden || pr.hidden?.includes(x.id))
          .map((x) => (
            <div class="m-row">
              <span class="m-swatch" style={{ background: x.color || 'var(--accent)' }} />
              <span class="grow ellipsis">{seriesLabel(x.label)}</span>
              <button type="button" class={`m-switch ${pr.hidden?.includes(x.id) ? '' : 'on'}`} role="switch" aria-checked={!pr.hidden?.includes(x.id)} onClick={() => send('series', { series: x.id })} />
            </div>
          ))}
      </div>
      <button type="button" class="m-btn danger" onClick={() => send('stop')}>
        <Icon name="x" size="sm" />
        {t('pres.exit')}
      </button>
    </div>
  );
}

function Loading(p: { error?: string }) {
  return <div class="m-loading">{p.error ? <span class="faint">{errText(p.error)}</span> : <span class="spin" />}</div>;
}

const dark = matchMedia('(prefers-color-scheme: dark)');
const applyTheme = () => (document.documentElement.dataset.theme = dark.matches ? 'dark' : 'light');
dark.addEventListener('change', applyTheme);
applyTheme();
render(<App />, document.getElementById('app')!);
boot();
