// MNE Lab desktop interface.
import './i18n/desktop';
import { render } from 'preact';
import { useEffect } from 'preact/hooks';
import './styles/tokens.css';
import './styles/base.css';
import './styles/app.css';
import { saveNow, seen } from './lib/actions';
import { on } from './lib/api';
import { openHelp } from './lib/features';
import { t } from './lib/i18n';
import { navigate, useRoute } from './lib/route';
import { app, openPanel, refresh, start, toast } from './lib/state';
import { useStore } from './lib/store';
import type { UpdateStatus } from './lib/types';
import { BrandMark, Button, DialogHost, Spinner, Toasts, TooltipHost } from './ui/kit';
import { ImportDialog, importFiles } from './screens/ls/files';
import { ContextTips, ExitScreen } from './screens/overlays';
import { Palette } from './screens/palette';
import { Panels } from './screens/panels';
import { Presentation } from './screens/present';
import { Profiles } from './screens/profiles';
import { Shell } from './screens/shell';
import { Start, startHold } from './screens/start';

function Root() {
  const s = useStore(app, (x) => x.s);
  const online = useStore(app, (x) => x.online);
  const hold = useStore(startHold, (x) => x.recoveryKey);
  const { path } = useRoute();
  const exitSteps = useStore(app, (x) => x.exitSteps.length);

  // Routes opened by the tray or a second launch.
  useEffect(() => {
    const open = (route: string) => {
      const u = new URL(route, location.origin);
      const panel = u.searchParams.get('panel');
      if (u.pathname === '/settings') {
        navigate('/', true);
        openPanel('settings');
      } else if (panel) {
        if (u.pathname !== location.pathname) navigate(u.pathname, true);
        openPanel(panel);
      } else navigate(u.pathname + u.search);
    };
    if (location.pathname === '/settings' || new URLSearchParams(location.search).has('panel')) open(location.pathname + location.search);
    const offs = [
      on('navigate', (r: string) => typeof r === 'string' && open(r)),
      on('focus', () => window.focus()),
      on('notice', (n: { key: string; label?: string }) => n?.key && toast('info', t('notice.' + n.key, { label: n.label || '' }))),
      on('update', (u: UpdateStatus) => {
        if (u?.staged && u.staged !== app.get().s?.update.staged) toast('success', t('upd.staged_toast', { v: u.staged }));
      }),
    ];
    return () => offs.forEach((f) => f());
  }, []);

  // The first time a profile opens, a short tour (skippable, replayable).
  useEffect(() => {
    if (s?.profile && !seen('tour') && !app.get().panel && path !== '/exit') openPanel('tour');
  }, [s?.profile?.id]);

  if (path === '/exit' || s?.exiting || exitSteps > 0) return <ExitScreen />;
  if (!s) return <Splash offline={!online} />;
  return (
    <>
      {!s.account || hold ? <Start /> : !s.profile ? <Profiles /> : <Shell />}
      {!online && <Reconnecting />}
    </>
  );
}

function Splash(p: { offline: boolean }) {
  return (
    <div class="screen splash">
      <div class="col gap4" style={{ alignItems: 'center' }}>
        <BrandMark size={64} />
        {p.offline ? (
          <>
            <span class="muted">{t('err.app.unreachable')}</span>
            <Button icon="refresh" onClick={() => refresh()}>
              {t('ui.retry')}
            </Button>
          </>
        ) : (
          <Spinner />
        )}
      </div>
    </div>
  );
}

function Reconnecting() {
  return (
    <div class="toasts" style={{ top: 12, bottom: 'auto' }} role="status">
      <div class="toast warning">
        <span class="spin" style={{ width: 14, height: 14 }} />
        <span class="grow">{t('app.reconnecting')}</span>
      </div>
    </div>
  );
}

function Global() {
  const prof = useStore(app, (x) => !!x.s?.profile);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      const st = app.get();
      if (mod && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        app.set({ palette: !st.palette });
      } else if (e.key === 'F1') {
        e.preventDefault();
        openHelp();
      } else if (mod && e.key.toLowerCase() === 's' && prof && !st.panel) {
        e.preventDefault();
        if (!location.pathname.startsWith('/ls/graphs/')) saveNow();
      } else if (mod && e.key.toLowerCase() === 'i' && prof && !st.panel) {
        e.preventDefault();
        if (!location.pathname.startsWith('/ls/files')) navigate('/ls/files');
        importFiles();
      }
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [prof]);
  return null;
}

function App() {
  const prof = useStore(app, (x) => !!x.s?.profile);
  return (
    <>
      <Root />
      <Global />
      {prof && <ImportDialog />}
      {prof && <Presentation />}
      {prof && <ContextTips />}
      <Panels />
      <Palette />
      <DialogHost />
      <Toasts />
      <TooltipHost />
    </>
  );
}

start();
render(<App />, document.getElementById('app')!);
