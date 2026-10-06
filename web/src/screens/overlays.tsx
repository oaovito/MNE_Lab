// Overlays opened from anywhere: Mobile Access, first-use explanations
// (Turbo, Save Clean & Exit), the onboarding tour, the LIGHTSCATTERING
// introduction, contextual tips, restart and exit screens.
import { useEffect, useState } from 'preact/hooks';
import { markSeen, seen, setTurbo, startExit } from '../lib/actions';
import { api, get, post } from '../lib/api';
import { fmtRel } from '../lib/format';
import { t } from '../lib/i18n';
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

const TOUR: Slide[] = [
  { icon: 'flask', key: 'tour.welcome', points: [{ icon: 'users', key: 'tour.p.profiles' }, { icon: 'shieldOk', key: 'tour.p.private' }, { icon: 'wifiOff', key: 'tour.p.offline' }, { icon: 'globe', key: 'tour.p.languages' }] },
  { icon: 'drive', key: 'tour.storage', points: [{ icon: 'usb', key: 'tour.p.usb_cloud' }, { icon: 'drive', key: 'tour.p.usb_only' }, { icon: 'cloud', key: 'tour.p.cloud_only' }, { icon: 'refresh', key: 'tour.p.sync' }] },
  { icon: 'usb', key: 'tour.modes', points: [{ icon: 'usb', key: 'tour.p.portable' }, { icon: 'laptop', key: 'tour.p.temporary' }, { icon: 'history', key: 'tour.p.backup' }, { icon: 'lifebuoy', key: 'tour.p.recovery' }] },
  { icon: 'package', key: 'tour.updates', points: [{ icon: 'package', key: 'tour.p.auto' }, { icon: 'lock', key: 'tour.p.locked' }] },
  { icon: 'phone', key: 'tour.mobile', points: [{ icon: 'qr', key: 'tour.p.qr' }, { icon: 'present', key: 'tour.p.controller' }, { icon: 'devices', key: 'tour.p.viewer' }] },
  { icon: 'microscope', key: 'tour.ls', points: [{ icon: 'fileUp', key: 'ls.flow.import' }, { icon: 'chart', key: 'ls.flow.graph' }, { icon: 'cycle', key: 'ls.flow.cycle' }, { icon: 'download', key: 'ls.flow.export' }] },
  { icon: 'archive', key: 'tour.everyday', points: [{ icon: 'archive', key: 'tour.p.mystuff' }, { icon: 'gauge', key: 'tour.p.turbo' }, { icon: 'search', key: 'tour.p.search' }, { icon: 'power', key: 'tour.p.exit' }] },
];

export function Tour() {
  const done = () => {
    closePanel();
    markSeen('tour');
  };
  return <Slides slides={TOUR} onDone={done} title={t('tour.title')} finish={t('tour.start')} />;
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
