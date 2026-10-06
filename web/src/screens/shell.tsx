// The application frame after a profile is open: header, page, footer.
import { useEffect, useRef, useState } from 'preact/hooks';
import { changeAccount, exitApp, switchProfile, syncNow, toggleTurbo } from '../lib/actions';
import { fmtRel } from '../lib/format';
import { t } from '../lib/i18n';
import { navigate, useRoute } from '../lib/route';
import { app, openPanel } from '../lib/state';
import { useStore } from '../lib/store';
import { activityLabel, StorageLabel, syncView } from '../ui/common';
import { Icon } from '../ui/icons';
import { Avatar, BrandMark, Button, Menu, type MenuEntry } from '../ui/kit';
import { Home } from './home';
import { LsModule } from './ls/module';
import { hasNews } from './help';

export function Shell() {
  const r = useRoute();
  const path = r.path;
  let page;
  if (path === '/' || path === '/settings') page = <Home />;
  else if (path.startsWith('/ls')) page = <LsModule />;
  else page = <Redirect to="/" />;
  return (
    <div class="shell screen">
      <Header />
      <main class="main" id="main">
        {page}
      </main>
      {path === '/' ? <Footer /> : <div />}
    </div>
  );
}

function Redirect(p: { to: string }) {
  useEffect(() => navigate(p.to, true), []);
  return null;
}

function Header() {
  const s = useStore(app, (x) => x.s)!;
  const { path } = useRoute();
  const turbo = s.settings.turbo;
  const nav = [
    { to: '/', icon: 'grid' as const, label: t('nav.home'), on: path === '/' },
    { to: '/ls/files', icon: 'flask' as const, label: 'LIGHTSCATTERING', on: path.startsWith('/ls') },
  ];
  return (
    <header class="header">
      <button class="brand" onClick={() => navigate('/')} aria-label={t('nav.home')}>
        <BrandMark size={26} />
        <span class="wordmark">
          MNE <b>Lab</b>
        </span>
      </button>
      <nav class="nav" aria-label={t('nav.main')}>
        {nav.map((n) => (
          <a
            href={n.to}
            class={n.on ? 'on' : ''}
            aria-current={n.on ? 'page' : undefined}
            onClick={(e) => {
              e.preventDefault();
              navigate(n.to);
            }}
          >
            <Icon name={n.icon} size="sm" />
            <span class="lbl">{n.label}</span>
          </a>
        ))}
      </nav>
      <span class="spacer" />
      <button class="search-btn" onClick={() => app.set({ palette: true })} aria-label={t('pal.open')}>
        <Icon name="search" size="sm" />
        <span class="lbl ellipsis">{t('pal.placeholder_short')}</span>
        <kbd>Ctrl K</kbd>
      </button>
      <StatusPill />
      <Button kind="ghost" icon="qr" class="hdr-mobile" tip={t('hdr.mobile')} label={t('hdr.mobile')} selected={s.mobile.active} onClick={() => openPanel('mobile')} />
      <Button kind="ghost" icon="gauge" class="hdr-turbo" tip={turbo ? t('hdr.turbo_on') : t('hdr.turbo_off')} label="Turbo" selected={turbo} onClick={toggleTurbo} />
      <span style={{ position: 'relative', display: 'inline-grid' }}>
        <Button kind="ghost" icon="help" class="hdr-help" tip={t('hdr.help')} label={t('hdr.help')} onClick={() => openPanel('help')} />
        {hasNews() && <span class="dot" style={{ position: 'absolute', top: 6, right: 6, color: 'var(--accent)', width: 7, height: 7 }} />}
      </span>
      <AvatarMenu />
    </header>
  );
}

function StatusPill() {
  const s = useStore(app, (x) => x.s)!;
  const ref = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const st = s.profile!.sync;
  const acts = s.activities || [];
  const busy = acts.some((a) => a.kind === 'sync');
  const v = syncView(st, busy);
  const work = acts.find((a) => a.kind !== 'sync');
  const label = work ? activityLabel(work.kind) : v.text;
  const tone = work ? 'busy' : v.tone;
  const upd = s.update;
  const items: MenuEntry[] = [
    { heading: t('status.title') },
    { label: v.text, icon: st.state === 'local' ? 'drive' : st.state === 'synced' ? 'cloudOk' : st.state === 'offline' ? 'cloudOff' : 'cloud', meta: v.detail, run: () => openPanel('settings', 'storage') },
    ...acts.map((a) => ({ label: activityLabel(a.kind), icon: 'activity' as const, meta: fmtRel(a.started), run: () => undefined })),
    ...(s.mobile.active ? [{ label: t('status.mobile_on', { n: s.mobile.devices?.length || 0 }), icon: 'phone' as const, run: () => openPanel('mobile') }] : []),
    {
      label: upd.locked ? t('status.locked', { v: upd.current }) : upd.staged ? t('status.update_ready') : t('status.build', { v: upd.current }),
      icon: upd.locked ? ('lock' as const) : ('package' as const),
      meta: upd.locked && upd.newer ? t('status.newer_available') : undefined,
      run: () => openPanel('settings', 'updates'),
    },
    { sep: true },
    ...(st.storageMode !== 'usb_only' ? [{ label: t('status.sync_now'), icon: 'refresh' as const, run: syncNow, disabled: busy }] : []),
    { label: t('status.open_storage'), icon: 'settings', run: () => openPanel('settings', 'storage') },
  ];
  return (
    <>
      <button ref={ref} class={`status-pill ${tone}`} onClick={() => setOpen(!open)} aria-expanded={open} data-tip={v.detail || undefined} aria-label={t('status.title') + ': ' + label}>
        {tone === 'busy' ? <span class="spin" style={{ width: 11, height: 11, borderWidth: 1.5 }} /> : <span class="dot" />}
        <span class="ellipsis">{label}</span>
      </button>
      {open && ref.current && <Menu anchor={ref.current} items={items} align="end" onClose={() => setOpen(false)} />}
    </>
  );
}

function AvatarMenu() {
  const s = useStore(app, (x) => x.s)!;
  const p = s.profile!;
  const ref = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const items: MenuEntry[] = [
    { label: t('menu.mystuff'), icon: 'archive', run: () => openPanel('mystuff') },
    { label: t('menu.settings'), icon: 'settings', run: () => openPanel('settings') },
    { label: t('menu.help'), icon: 'book', run: () => openPanel('help') },
    { label: t('menu.whatsnew'), icon: 'gift', badge: hasNews() ? t('ui.new') : undefined, run: () => openPanel('whatsnew') },
    { sep: true },
    { label: t('menu.switch_profile'), icon: 'users', run: switchProfile },
    { label: t('hdr.change_account'), icon: 'logout', run: changeAccount },
    { sep: true },
    { label: t('menu.exit'), icon: 'power', run: exitApp },
  ];
  return (
    <>
      <button ref={ref} class="avatar-btn" onClick={() => setOpen(!open)} aria-expanded={open} aria-haspopup="menu" aria-label={t('menu.profile', { name: p.username })} data-tip={p.username}>
        <Avatar id={p.id} name={p.username} color={p.color} photo={p.avatar} hash={p.avatarHash} size={32} />
      </button>
      {open && ref.current && (
        <Menu anchor={ref.current} items={items} align="end" onClose={() => setOpen(false)}>
          <div class="row gap3" style={{ padding: '8px 10px 10px' }}>
            <Avatar id={p.id} name={p.username} color={p.color} photo={p.avatar} hash={p.avatarHash} size={40} />
            <div class="col" style={{ gap: 0, minWidth: 0 }}>
              <b class="ellipsis">{p.username}</b>
              <span class="xs muted">
                <StorageLabel mode={p.storageMode} />
              </span>
            </div>
          </div>
          <div class="menu-sep" />
        </Menu>
      )}
    </>
  );
}

function Footer() {
  const s = useStore(app, (x) => x.s)!;
  return (
    <footer class="footer">
      <span>made by oaovito</span>
      <span class="sep" />
      <span class="num">MNE Lab {s.version}</span>
    </footer>
  );
}
