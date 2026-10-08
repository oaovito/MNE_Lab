// Homepage: the LIGHTSCATTERING module front and center, the state of the
// system, and the session actions (Mobile Access, Turbo, Save, Clean & Exit).
import { exitApp, toggleTurbo } from '../lib/actions';
import { fmtDate, fmtRel } from '../lib/format';
import { t } from '../lib/i18n';
import { useCycles, useFiles, useGraphs, useRev } from '../lib/library';
import { get } from '../lib/api';
import { navigate } from '../lib/route';
import { app, openPanel } from '../lib/state';
import { useStore } from '../lib/store';
import { activityLabel, ModeBadge, StorageLabel, syncView, Thumb } from '../ui/common';
import { Icon, type IconName } from '../ui/icons';
import { Badge, Button, Skeleton, Switch, useAsync } from '../ui/kit';
import { providerName } from '../ui/providers';
import { importFiles } from './ls/files';

function greeting() {
  const h = new Date().getHours();
  return h < 5 ? 'home.hello.night' : h < 12 ? 'home.hello.morning' : h < 18 ? 'home.hello.afternoon' : 'home.hello.evening';
}

export function Home() {
  const s = useStore(app, (x) => x.s)!;
  const p = s.profile!;
  const files = useFiles();
  const graphs = useGraphs();
  const cycles = useCycles();
  const nMeas = (files.data || []).reduce((n, f) => n + (f.items?.length || 0), 0);
  const loading = files.loading && !files.data;
  const flow: { icon: IconName; k: string; go: () => void }[] = [
    { icon: 'upload', k: 'import', go: () => importFiles() },
    { icon: 'listChecks', k: 'review', go: () => navigate('/ls/files') },
    { icon: 'chart', k: 'graph', go: () => navigate('/ls/graphs') },
    { icon: 'cycle', k: 'cycle', go: () => navigate('/ls/cycles') },
    { icon: 'present', k: 'present', go: () => navigate('/ls/graphs') },
    { icon: 'download', k: 'export', go: () => navigate('/ls/graphs') },
  ];
  const recent = (graphs.data || []).slice(0, 4);

  return (
    <div class="home">
      <div class="home-hero">
        <div class="grow">
          <h1>{t(greeting(), { name: p.username })}</h1>
          <div class="sub small">
            <ModeBadge mode={s.mode} />
            <span class="faint">·</span>
            <StorageLabel mode={p.storageMode} />
            {p.sync.provider && (
              <>
                <span class="faint">·</span>
                <span>{providerName(p.sync.provider)}</span>
              </>
            )}
          </div>
        </div>
        <Button kind="primary" icon="upload" class="hero-import" onClick={() => importFiles()}>
          {t('ls.import')}
        </Button>
      </div>

      <section class="module-card" aria-labelledby="ls-title">
        <div class="row">
          <span class="kicker">
            <Icon name="microscope" size="sm" />
            {t('home.module')}
          </span>
          <span class="spacer" />
          <Badge icon="book">{t('home.dls')}</Badge>
        </div>
        <div class="col gap2">
          <h2 id="ls-title">LIGHTSCATTERING</h2>
          <p class="desc">{t('home.ls.desc')}</p>
        </div>
        <div class="flow" role="list">
          {flow.map((f, i) => (
            <button class="flow-step" role="listitem" onClick={f.go}>
              <span class="row gap1">
                <Icon name={f.icon} size="sm" />
                <span class="faint num">{i + 1}</span>
              </span>
              <b>{t('home.flow.' + f.k)}</b>
              <span class="clamp2">{t('home.flow.' + f.k + '.d')}</span>
            </button>
          ))}
        </div>
        <div class="stats">
          {[
            ['files', files.data?.length],
            ['measurements', nMeas],
            ['graphs', graphs.data?.length],
            ['cycles', cycles.data?.length],
          ].map(([k, v]) => (
            <div class="stat">
              <div class="v">{loading ? <Skeleton w={36} h={24} /> : v ?? 0}</div>
              <div class="k">{t('home.stat.' + k)}</div>
            </div>
          ))}
        </div>
        <div class="row" style={{ marginTop: 'auto' }}>
          <span class="section-title">{t('home.recent')}</span>
          <span class="spacer" />
          <Button size="sm" kind="ghost" trail="right" onClick={() => navigate('/ls/files')}>
            {t('home.open_module')}
          </Button>
        </div>
        {recent.length ? (
          <div class="recent">
            {recent.map((g) => (
              <button class="gcard" onClick={() => navigate('/ls/graphs/' + g.id)}>
                <div class="thumb">
                  <Thumb id={g.id!} updated={g.updated} />
                </div>
                <div class="body">
                  <span class="title ellipsis small">{g.title || t('graph.kind.' + g.kind)}</span>
                  <span class="meta">{fmtDate(g.updated)}</span>
                </div>
              </button>
            ))}
          </div>
        ) : (
          <p class="small faint">{loading ? '' : t('home.recent.empty')}</p>
        )}
      </section>

      <aside class="side">
        <SystemCard />
        <SessionCard />
      </aside>
    </div>
  );
}

function SystemCard() {
  const s = useStore(app, (x) => x.s)!;
  const p = s.profile!;
  const acts = s.activities || [];
  const v = syncView(p.sync, acts.some((a) => a.kind === 'sync'));
  const u = s.update;
  const rev = useRev();
  const backups = useAsync(() => get<{ created: string }[] | null>('/api/backups'), [rev, acts.length]);
  const last = backups.data?.[0]?.created;
  const rows: { icon: IconName; k: string; v: any; go?: () => void }[] = [
    {
      icon: p.sync.state === 'local' ? 'drive' : p.sync.state === 'synced' ? 'cloudOk' : p.sync.state === 'offline' ? 'cloudOff' : 'cloud',
      k: t('home.sys.data'),
      v: (
        <span class={`row gap1 ${v.tone === 'err' ? '' : ''}`} style={{ justifyContent: 'flex-end' }}>
          <span class="dot" style={{ color: v.tone === 'ok' ? 'var(--success)' : v.tone === 'busy' ? 'var(--info)' : v.tone === 'warn' ? 'var(--warning)' : 'var(--danger)' }} />
          <span class="ellipsis">{v.text}</span>
        </span>
      ),
      go: () => openPanel('settings', 'storage'),
    },
    { icon: 'history', k: t('home.sys.backup'), v: <span class="muted">{last ? fmtRel(last) : backups.loading ? '' : t('home.sys.backup.none')}</span>, go: () => openPanel('settings', 'backups') },
    {
      icon: u.locked ? 'lock' : 'package',
      k: u.locked ? 'LockedBuild' : t('home.sys.updates'),
      v: (
        <span class="muted">
          {u.staged ? t('status.update_ready') : u.locked && u.newer ? t('status.newer_available') : u.configured ? (u.locked ? u.current : t('home.sys.auto_update')) : t('home.sys.update_offline_build')}
        </span>
      ),
      go: () => openPanel('settings', 'updates'),
    },
    {
      icon: 'phone',
      k: t('home.sys.mobile'),
      v: <span class="muted">{s.mobile.active ? t('status.mobile_on', { n: s.mobile.devices?.length || 0 }) : t('home.sys.off')}</span>,
      go: () => openPanel('mobile'),
    },
  ];
  return (
    <div class="card pad col gap2">
      <div class="row">
        <h4 class="grow">{t('home.sys.title')}</h4>
        {acts.length > 0 && (
          <span class="xs muted row gap1">
            <span class="spin" style={{ width: 12, height: 12, borderWidth: 1.5 }} />
            {activityLabel(acts[0].kind)}
          </span>
        )}
      </div>
      <div class="status-list">
        {rows.map((r) => (
          <div class="status-item" role="button" tabIndex={0} style={{ cursor: 'pointer' }} onClick={r.go} onKeyDown={(e) => e.key === 'Enter' && r.go?.()}>
            <Icon name={r.icon} size="sm" />
            <span class="k">{r.k}</span>
            <span class="v ellipsis">{r.v}</span>
          </div>
        ))}
      </div>
      {p.storageMode !== 'usb_only' && p.sync.lastSync && <span class="xs faint">{t('status.last_sync', { when: fmtRel(p.sync.lastSync) })}</span>}
    </div>
  );
}

function SessionCard() {
  const s = useStore(app, (x) => x.s)!;
  return (
    <div class="card pad col gap3">
      <h4>{t('home.session')}</h4>
      <div class="quick">
        <div class="quick-row">
          <span class="ic">
            <Icon name="qr" />
          </span>
          <div class="grow" style={{ minWidth: 0 }}>
            <b>{t('hdr.mobile')}</b>
            <p class="clamp2">{t('home.mobile.d')}</p>
          </div>
          <Button size="sm" onClick={() => openPanel('mobile')}>
            {s.mobile.active ? t('ui.open') : t('home.mobile.go')}
          </Button>
        </div>
        <div class="quick-row">
          <span class="ic">
            <Icon name="gauge" />
          </span>
          <div class="grow" style={{ minWidth: 0 }}>
            <b>Turbo</b>
            <p class="clamp2">{t('home.turbo.d')}</p>
          </div>
          <Switch checked={s.settings.turbo} onChange={toggleTurbo} tip={t('home.turbo.tip')} />
        </div>
        <div class="quick-row">
          <span class="ic">
            <Icon name="power" />
          </span>
          <div class="grow" style={{ minWidth: 0 }}>
            <b>{t('menu.exit')}</b>
            <p class="clamp2">{t(s.mode === 'temporary' ? 'home.exit.d.temporary' : 'home.exit.d.portable')}</p>
          </div>
          <Button size="sm" kind="primary" onClick={exitApp}>
            {t('home.exit.go')}
          </Button>
        </div>
      </div>
    </div>
  );
}
