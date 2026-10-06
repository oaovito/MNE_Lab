// Overlays opened from anywhere: Mobile Access, first-use explanations
// (Turbo, Save Clean & Exit), the onboarding tour, the LIGHTSCATTERING
// introduction, contextual tips, restart and exit screens.
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { markSeen, seen, setTurbo, startExit } from '../lib/actions';
import { api, get, post } from '../lib/api';
import { fmtRel } from '../lib/format';
import { t } from '../lib/i18n';
import { navigate } from '../lib/route';
import { app, closePanel, openPanel, refresh, run } from '../lib/state';
import { useStore } from '../lib/store';
import type { MobileInfo } from '../lib/types';
import { Icon, type IconName } from '../ui/icons';
import { BrandMark, Button, confirmDialog, Modal, Notice, Spinner } from '../ui/kit';

// ---- Mobile Access ----

export function MobilePanel() {
  const s = useStore(app, (x) => x.s)!;
  const [info, setInfo] = useState<MobileInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const first = !seen('mobile');
  const load = () => get<MobileInfo>('/api/mobile').then(setInfo).catch(() => undefined);
  useEffect(() => {
    load();
  }, [s.mobile.active, s.mobile.devices?.length]);
  const start = async () => {
    setBusy(true);
    setErr('');
    const r = await run(() => post<MobileInfo>('/api/mobile/start'), setErr);
    setBusy(false);
    if (r) (setInfo(r), markSeen('mobile'));
  };
  const stop = async () => {
    await run(() => post('/api/mobile/stop'));
    load();
    refresh();
  };
  const revoke = async (id: string) => {
    if (await confirmDialog({ title: t('mob.revoke.q'), body: t('mob.revoke.d'), confirm: t('mob.revoke'), danger: true })) {
      const r = await run(() => api<MobileInfo>('DELETE', '/api/mobile/devices/' + id));
      if (r) setInfo(r);
    }
  };
  const m = info || s.mobile;
  return (
    <Modal size="wide" icon="qr" title={t('hdr.mobile')} sub={t('mob.sub')} onClose={closePanel}>
      <div class="qr-box">
        <div class="col gap2" style={{ alignItems: 'center' }}>
          <div class="qr" aria-label={t('mob.qr')}>
            {m.active && m.qr ? (
              <span dangerouslySetInnerHTML={{ __html: m.qr }} style={{ width: '100%', height: '100%', display: 'block' }} />
            ) : busy ? (
              <Spinner />
            ) : (
              <Icon name="qr" size="lg" class="faint" />
            )}
          </div>
          {m.active && m.qr && m.expires && <span class="xs faint">{t('mob.expires', { when: fmtRel(m.expires) })}</span>}
          {m.active && !m.qr && (
            <Button size="sm" icon="refresh" busy={busy} onClick={start}>
              {t('mob.new_code')}
            </Button>
          )}
        </div>
        <div class="col gap4">
          {(first || !m.active) && (
            <div class="col gap3">
              <Point icon="present" title={t('mob.controller')} body={t('mob.controller.d')} />
              <Point icon="devices" title={t('mob.viewer')} body={t('mob.viewer.d')} />
              <Point icon="shieldOk" title={t('mob.secure')} body={t('mob.secure.d')} />
            </div>
          )}
          {m.active && (
            <Notice kind="info" icon="wifi">
              {t('mob.scan', { addr: m.address || '' })}
            </Notice>
          )}
          {err && <Notice kind="danger">{t('err.' + err) !== 'err.' + err ? t('err.' + err) : t('err.mobile')}</Notice>}
          {s.mode === 'temporary' && <span class="xs faint">{t('mob.temporary')}</span>}
          <div class="col gap2">
            <span class="section-title">{t('mob.devices')}</span>
            {!m.devices?.length ? (
              <span class="small muted">{t('mob.devices.none')}</span>
            ) : (
              m.devices.map((d) => (
                <div class="row gap2 small">
                  <Icon name="phone" size="sm" />
                  <span class="grow ellipsis">{d.label || t('mob.device')}</span>
                  <span class="xs faint">{fmtRel(d.lastSeen)}</span>
                  <Button size="sm" kind="ghost" onClick={() => revoke(d.id)}>
                    {t('mob.revoke')}
                  </Button>
                </div>
              ))
            )}
          </div>
          <div class="row gap2">
            {m.active ? (
              <Button kind="danger" icon="power" onClick={stop}>
                {t('mob.stop')}
              </Button>
            ) : (
              <Button kind="primary" icon="qr" busy={busy} onClick={start}>
                {t('mob.start')}
              </Button>
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
}

function Point(p: { icon: IconName; title: string; body: string }) {
  return (
    <div class="row gap3" style={{ alignItems: 'flex-start' }}>
      <span class="modal-icon" style={{ width: 34, height: 34 }}>
        <Icon name={p.icon} size="sm" />
      </span>
      <div class="col" style={{ gap: 2 }}>
        <b>{p.title}</b>
        <span class="small muted">{p.body}</span>
      </div>
    </div>
  );
}

// ---- first use: Turbo, Save Clean & Exit ----

export function TurboPanel() {
  const s = useStore(app, (x) => x.s)!;
  const on = !s.settings.turbo;
  return (
    <Modal
      size="narrow"
      icon="gauge"
      title={on ? t('turbo.on.q') : t('turbo.off.q')}
      onClose={closePanel}
      foot={
        <>
          <span class="spacer" />
          <Button onClick={closePanel}>{t('ui.cancel')}</Button>
          <Button
            kind="primary"
            onClick={async () => {
              await markSeen('turbo');
              setTurbo(on);
            }}
          >
            {on ? t('turbo.on.go') : t('turbo.off.go')}
          </Button>
        </>
      }
    >
      <div class="col gap2 muted">
        <span>{t('turbo.d1')}</span>
        <span>{t('turbo.d2')}</span>
        <span class="small">{t('turbo.d3')}</span>
      </div>
    </Modal>
  );
}

export function ExitConfirm() {
  const s = useStore(app, (x) => x.s)!;
  const temp = s.mode === 'temporary';
  // The same steps the exit runs, in its order, for this profile and session.
  const cloud = !!s.profile && s.profile.storageMode !== 'usb_only';
  const steps = ['save', ...(cloud ? ['sync', 'verify'] : []), ...(s.mobile.active ? ['mobile'] : []), ...(temp ? ['clean'] : [])];
  return (
    <Modal
      size="narrow"
      icon="power"
      title={t('menu.exit')}
      onClose={closePanel}
      foot={
        <>
          <span class="spacer" />
          <Button onClick={closePanel}>{t('ui.cancel')}</Button>
          <Button
            kind="primary"
            icon="power"
            onClick={async () => {
              await markSeen('exit');
              closePanel();
              startExit();
            }}
          >
            {t('home.exit.go')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <span class="muted">{t(temp ? 'exitq.temporary' : 'exitq.portable')}</span>
        <div class="exit-steps">
          {steps.map((k) => (
            <div class="exit-step">
              <Icon name="right" size="sm" />
              {t('exit.step.' + k)}
            </div>
          ))}
        </div>
      </div>
    </Modal>
  );
}

/** Busy covers a restart (Turbo) or an account change. */
export function Busy(p: { what: string; arg?: string }) {
  return (
    <div class="overlay" style={{ zIndex: 95 }}>
      <div class="col gap3" style={{ alignItems: 'center', color: 'var(--text-2)' }}>
        <BrandMark size={44} />
        <Spinner label={p.what === 'switching' ? t('busy.switching') : p.arg === 'turbo_on' ? t('busy.turbo_on') : p.arg === 'turbo_off' ? t('busy.turbo_off') : t('busy.restarting')} />
      </div>
    </div>
  );
}

// ---- exit screen ----

const ORDER = ['save', 'sync', 'verify', 'mobile', 'recovery', 'clean', 'done'];

export function ExitScreen() {
  const steps = useStore(app, (x) => x.exitSteps);
  const s = useStore(app, (x) => x.s);
  const online = useStore(app, (x) => x.online);
  const temp = s?.mode === 'temporary';
  const last = new Map<string, { state: string; detail?: string }>();
  for (const x of steps) last.set(x.step, x);
  const done = last.get('done')?.state === 'done' || (!online && steps.length > 0);
  const recovery = last.has('recovery');
  const show = ORDER.filter((k) => last.has(k) && k !== 'done');
  return (
    <div class="exit screen">
      <div class="card pad-lg col gap4">
        <div class="row gap3">
          <BrandMark size={34} />
          <div class="col" style={{ gap: 0 }}>
            <h2>{done ? t('exit.done') : t('exit.running')}</h2>
            <span class="small muted">{t(temp ? 'exit.sub.temporary' : 'exit.sub.portable')}</span>
          </div>
        </div>
        <div class="exit-steps" aria-live="polite">
          {!show.length && (
            <div class="exit-step running">
              <span class="spin" style={{ width: 14, height: 14 }} />
              {t('exit.step.save')}
            </div>
          )}
          {show.map((k) => {
            const st = last.get(k)!;
            return (
              <div class={`exit-step ${st.state}`}>
                {st.state === 'running' && !done ? <span class="spin" style={{ width: 14, height: 14 }} /> : <Icon name={st.state === 'warning' ? 'warning' : 'check'} size="sm" />}
                <span class="grow">{t('exit.step.' + k)}</span>
                {st.state === 'warning' && <span class="xs">{t('exit.warn.' + k)}</span>}
              </div>
            );
          })}
        </div>
        {recovery && <Notice kind="warning" icon="lifebuoy">{t('exit.recovery')}</Notice>}
        {done && <span class="small muted">{t('exit.close')}</span>}
      </div>
    </div>
  );
}

// ---- onboarding ----

type Slide = { icon: IconName; key: string; points?: { icon: IconName; key: string }[] };

function Slides(p: { slides: Slide[]; onDone: () => void; title: string; finish: string }) {
  const [i, setI] = useState(0);
  const sl = p.slides[i];
  const last = i === p.slides.length - 1;
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight' && !last) setI(i + 1);
      if (e.key === 'ArrowLeft' && i > 0) setI(i - 1);
    };
    document.addEventListener('keydown', k);
    return () => document.removeEventListener('keydown', k);
  }, [i]);
  return (
    <Modal
      icon="sparkles"
      title={p.title}
      sub={t('tour.step', { n: i + 1, total: p.slides.length })}
      onClose={p.onDone}
      foot={
        <>
          <Button kind="ghost" onClick={p.onDone}>
            {t('tour.skip')}
          </Button>
          <span class="spacer" />
          <span class="row gap1" aria-hidden="true">
            {p.slides.map((_, k) => (
              <span class="dot" style={{ color: k === i ? 'var(--accent)' : 'var(--border-2)' }} />
            ))}
          </span>
          <span class="spacer" />
          {i > 0 && (
            <Button icon="back" onClick={() => setI(i - 1)}>
              {t('ui.back')}
            </Button>
          )}
          <Button kind="primary" trail={last ? undefined : 'next'} onClick={() => (last ? p.onDone() : setI(i + 1))}>
            {last ? p.finish : t('ui.next')}
          </Button>
        </>
      }
    >
      <div class="tour" key={i}>
        <div class="tour-art">
          <Icon name={sl.icon} />
        </div>
        <div class="col gap2">
          <h3>{t(sl.key)}</h3>
          <p class="muted">{t(sl.key + '.d')}</p>
        </div>
        {sl.points && (
          <div class="tour-points">
            {sl.points.map((x) => (
              <div>
                <Icon name={x.icon} size="sm" />
                <span>{t(x.key)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}

// The first-run tutorial points at the real interface, one stop at a time,
// so it teaches where things are instead of listing features.
type Stop = { key: string; icon: IconName; target?: string[]; points?: { icon: IconName; key: string }[] };

const TOUR: Stop[] = [
  { key: 'tour.welcome', icon: 'sparkles' },
  { key: 'tour.import', icon: 'upload', target: ['.hero-import'], points: [{ icon: 'fileUp', key: 'tour.p.drop' }] },
  {
    key: 'tour.ls',
    icon: 'flask',
    target: ['.nav a[href="/ls/files"]'],
    points: [
      { icon: 'listChecks', key: 'tour.p.files' },
      { icon: 'chart', key: 'tour.p.graphs' },
      { icon: 'cycle', key: 'tour.p.cycles' },
      { icon: 'present', key: 'tour.p.share' },
    ],
  },
  { key: 'tour.status', icon: 'cloudOk', target: ['.status-pill'] },
  { key: 'tour.search', icon: 'search', target: ['.search-btn'] },
  {
    key: 'tour.tools',
    icon: 'qr',
    target: ['.hdr-mobile', '.hdr-turbo', '.hdr-help'],
    points: [
      { icon: 'phone', key: 'tour.p.phone' },
      { icon: 'gauge', key: 'tour.p.turbo' },
      { icon: 'help', key: 'tour.p.help' },
    ],
  },
  {
    key: 'tour.profile',
    icon: 'user',
    target: ['.avatar-btn'],
    points: [
      { icon: 'archive', key: 'tour.p.mystuff' },
      { icon: 'settings', key: 'tour.p.settings' },
      { icon: 'power', key: 'tour.p.exit' },
    ],
  },
];

const FLOW: { icon: IconName; k: string }[] = [
  { icon: 'upload', k: 'import' },
  { icon: 'listChecks', k: 'review' },
  { icon: 'chart', k: 'graph' },
  { icon: 'cycle', k: 'cycle' },
  { icon: 'present', k: 'present' },
  { icon: 'download', k: 'export' },
];

type Box = { x: number; y: number; w: number; h: number };

/** The union of the visible target elements, or null to center the card. */
function measure(sel?: string[]): Box | null {
  if (!sel) return null;
  const rs = sel.flatMap((q) => [...document.querySelectorAll(q)].map((e) => e.getBoundingClientRect())).filter((r) => r.width > 0 && r.height > 0);
  if (!rs.length) return null;
  const x = Math.min(...rs.map((r) => r.left)),
    y = Math.min(...rs.map((r) => r.top));
  const w = Math.max(...rs.map((r) => r.right)) - x,
    h = Math.max(...rs.map((r) => r.bottom)) - y;
  return { x: x - 6, y: y - 6, w: w + 12, h: h + 12 };
}

export function Tour() {
  const [i, setI] = useState(0);
  const [box, setBox] = useState<Box | null>(null);
  const [vw, setVw] = useState(innerWidth);
  const cardRef = useRef<HTMLDivElement>(null);
  const stop = TOUR[i];
  const last = i === TOUR.length - 1;
  const done = () => {
    closePanel();
    markSeen('tour');
  };
  const go = (n: number) => n >= 0 && n < TOUR.length && setI(n);

  // The stops live on the home screen.
  useEffect(() => navigate('/'), []);
  useLayoutEffect(() => {
    let prev = '';
    const update = () => {
      const b = measure(stop.target);
      const k = JSON.stringify(b) + innerWidth;
      if (k === prev) return;
      prev = k;
      setBox(b);
      setVw(innerWidth);
    };
    update();
    const raf = requestAnimationFrame(update);
    const iv = setInterval(update, 400);
    addEventListener('resize', update);
    return () => {
      cancelAnimationFrame(raf);
      clearInterval(iv);
      removeEventListener('resize', update);
    };
  }, [i]);
  useEffect(() => {
    cardRef.current?.querySelector<HTMLElement>('button.primary')?.focus();
  }, [i]);
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        done();
      } else if (e.key === 'ArrowRight') go(i + 1);
      else if (e.key === 'ArrowLeft') go(i - 1);
      else if (e.key === 'Tab' && cardRef.current) {
        const f = cardRef.current.querySelectorAll<HTMLElement>('button:not([disabled])');
        const a = f[0],
          b = f[f.length - 1];
        if (e.shiftKey && document.activeElement === a) (e.preventDefault(), b.focus());
        else if (!e.shiftKey && document.activeElement === b) (e.preventDefault(), a.focus());
      }
    };
    document.addEventListener('keydown', k);
    return () => document.removeEventListener('keydown', k);
  }, [i]);

  // The card sits under its target (every stop is near the top), aligned to
  // it and kept inside the window; a stop without a target is centered.
  const W = Math.min(box ? 360 : 440, vw - 32);
  let place: Record<string, string | number> = {};
  let caret: number | undefined;
  if (box) {
    const left = Math.max(16, Math.min(box.x + box.w / 2 - W / 2, vw - 16 - W));
    place = { left, top: box.y + box.h + 14, width: W };
    caret = Math.max(22, Math.min(box.x + box.w / 2 - left, W - 22));
  }

  return (
    <div class="coach" role="presentation">
      {box ? <div class="coach-hole" style={{ left: box.x, top: box.y, width: box.w, height: box.h }} /> : <div class="coach-scrim" />}
      <div ref={cardRef} class={`coach-card ${box ? '' : 'coach-center'}`} style={box ? place : { width: W }} role="dialog" aria-modal="true" aria-labelledby="coach-title" aria-describedby="coach-desc">
        {caret !== undefined && <span class="coach-caret" style={{ left: caret }} />}
        <div class="row gap3">
          <span class="coach-icon">
            <Icon name={stop.icon} size="sm" />
          </span>
          <span class="xs muted grow">{t('tour.kicker')}</span>
          <span class="xs faint num">{t('tour.step', { n: i + 1, total: TOUR.length })}</span>
        </div>
        <div class="col gap2" key={i}>
          <h3 id="coach-title">{t(stop.key)}</h3>
          <p id="coach-desc" class="muted small">
            {t(stop.key + '.d')}
          </p>
        </div>
        {i === 0 && (
          <ol class="coach-flow">
            {FLOW.map((f) => (
              <li>
                <Icon name={f.icon} size="sm" />
                <span>{t('home.flow.' + f.k)}</span>
              </li>
            ))}
          </ol>
        )}
        {stop.points && (
          <ul class="coach-points">
            {stop.points.map((x) => (
              <li>
                <Icon name={x.icon} size="sm" />
                <span>{t(x.key)}</span>
              </li>
            ))}
          </ul>
        )}
        <div class="row gap2 coach-foot">
          <Button kind="ghost" size="sm" onClick={done}>
            {t('tour.skip')}
          </Button>
          <span class="spacer" />
          <span class="row gap1" aria-hidden="true">
            {TOUR.map((_, k) => (
              <span class="dot" style={{ color: k === i ? 'var(--accent)' : 'var(--border-2)' }} />
            ))}
          </span>
          <span class="spacer" />
          {i > 0 && <Button size="sm" icon="back" label={t('ui.back')} tip={t('ui.back')} onClick={() => go(i - 1)} />}
          <Button kind="primary" size="sm" trail={last ? undefined : 'next'} onClick={() => (last ? done() : go(i + 1))}>
            {last ? t('tour.start') : t('ui.next')}
          </Button>
        </div>
      </div>
    </div>
  );
}

const LS_INTRO: Slide[] = [
  { icon: 'microscope', key: 'lsi.what' },
  { icon: 'fileUp', key: 'lsi.import' },
  { icon: 'chart', key: 'lsi.graph' },
  { icon: 'layers', key: 'lsi.compare' },
  { icon: 'cycle', key: 'lsi.cycle' },
  { icon: 'present', key: 'lsi.present' },
  { icon: 'download', key: 'lsi.export' },
];

export function LsIntro() {
  const done = () => {
    closePanel();
    markSeen('ls');
  };
  return <Slides slides={LS_INTRO} onDone={done} title="LIGHTSCATTERING" finish={t('lsi.go')} />;
}

// ---- contextual tips (one at a time, never repeated once dismissed) ----

type Tip = { key: string; icon: IconName; when: (s: NonNullable<ReturnType<typeof app.get>['s']>) => boolean; action?: { label: string; run: () => void } };

const TIPS: Tip[] = [
  { key: 'tip.portable', icon: 'usb', when: (s) => s.mode === 'portable' },
  { key: 'tip.temporary', icon: 'laptop', when: (s) => s.mode === 'temporary' },
  { key: 'tip.cloud', icon: 'cloud', when: (s) => !!s.profile && s.profile.storageMode !== 'usb_only' },
  { key: 'tip.locked_new', icon: 'lock', when: (s) => s.update.locked && s.update.newer, action: { label: 'tip.locked_new.go', run: () => openPanel('settings', 'updates') } },
];

export function ContextTips() {
  const s = useStore(app, (x) => x.s);
  const panel = useStore(app, (x) => x.panel);
  const [hidden, setHidden] = useState<string[]>([]);
  if (!s?.profile || panel) return null;
  if (!seen('tour')) return null;
  const tip = TIPS.find((x) => !hidden.includes(x.key) && !seenTip(x.key, s) && x.when(s));
  if (!tip) return null;
  const close = () => {
    setHidden([...hidden, tip.key]);
    markSeen(tipId(tip.key, s));
  };
  return (
    <div class="tip" role="status" style={{ right: 20, bottom: 56 }}>
      <h4>
        <Icon name={tip.icon} size="sm" />
        {t(tip.key)}
      </h4>
      <p>{t(tip.key + '.d')}</p>
      <div class="row gap2" style={{ justifyContent: 'flex-end' }}>
        {tip.action && (
          <Button
            size="sm"
            onClick={() => {
              close();
              tip.action!.run();
            }}
          >
            {t(tip.action.label)}
          </Button>
        )}
        <Button size="sm" kind="primary" onClick={close}>
          {t('tip.ok')}
        </Button>
      </div>
    </div>
  );
}

// The LockedBuild tip is tied to the newest build, so it returns once for
// each new Stable build and never again for the same one.
const tipId = (key: string, s: any) => (key === 'tip.locked_new' ? key + ':' + (s.update.latest || '') : key);
const seenTip = (key: string, s: any) => seen(tipId(key, s));
