// My Stuff: everything that belongs to the open profile in one place:
// scientific data, saved work, exports, backups, connections, sessions and
// preferences.
import { get, post } from '../lib/api';
import { fmtDate, fmtRel } from '../lib/format';
import { LANG_NAMES, t, type Lang } from '../lib/i18n';
import { useCycles, useFiles, useGraphs } from '../lib/library';
import { navigate } from '../lib/route';
import { app, closePanel, openPanel, run } from '../lib/state';
import { useStore } from '../lib/store';
import { StorageLabel } from '../ui/common';
import { Icon, type IconName } from '../ui/icons';
import { Avatar, Button, Modal, Skeleton, useAsync } from '../ui/kit';
import { providerName } from '../ui/providers';

type ExportRec = { id: string; at: string; formats: string[]; preset: string; names: string[] | null; folder: string };

export function MyStuff() {
  const s = useStore(app, (x) => x.s)!;
  const p = s.profile!;
  const files = useFiles();
  const graphs = useGraphs();
  const cycles = useCycles();
  const tf = useFiles(true);
  const tg = useGraphs(true);
  const tc = useCycles(true);
  const history = useAsync(() => get<ExportRec[] | null>('/api/export/history').then((x) => x || []), []);
  const backups = useAsync(() => get<{ created: string }[] | null>('/api/backups').then((x) => x || []), []);
  const nMeas = (files.data || []).reduce((n, f) => n + (f.items?.length || 0), 0);
  const go = (fn: () => void) => () => (closePanel(), fn());
  const trashN = (tf.data?.length || 0) + (tg.data?.length || 0) + (tc.data?.length || 0);

  return (
    <Modal size="wide" icon="archive" title={t('menu.mystuff')} sub={t('my.sub')} onClose={closePanel}>
      <div class="col gap4">
        <div class="row gap3">
          <Avatar id={p.id} name={p.username} color={p.color} photo={p.avatar} hash={p.avatarHash} size={48} />
          <div class="col grow" style={{ gap: 2 }}>
            <b style={{ fontSize: 'var(--fs-lg)' }}>{p.username}</b>
            <span class="small muted row gap2">
              <StorageLabel mode={p.storageMode} />
              {p.provider && <span>· {providerName(p.provider)}</span>}
              <span>· {t('my.since', { when: fmtDate(p.created) })}</span>
            </span>
          </div>
          <Button size="sm" icon="settings" onClick={() => openPanel('settings', 'profile')}>
            {t('menu.settings')}
          </Button>
        </div>

        <div class="stats">
          <Stat icon="files" v={files.data?.length} k={t('ls.tab.files')} go={go(() => navigate('/ls/files'))} />
          <Stat icon="table" v={files.data ? nMeas : undefined} k={t('my.measurements')} go={go(() => navigate('/ls/files'))} />
          <Stat icon="chart" v={graphs.data?.length} k={t('ls.tab.graphs')} go={go(() => navigate('/ls/graphs'))} />
          <Stat icon="cycle" v={cycles.data?.length} k={t('ls.tab.cycles')} go={go(() => navigate('/ls/cycles'))} />
        </div>

        <div class="row gap4" style={{ alignItems: 'stretch', flexWrap: 'wrap' }}>
          <Card title={t('my.exports')} icon="download" class="grow">
            {history.loading && !history.data ? (
              <Skeleton h={80} />
            ) : !history.data?.length ? (
              <span class="small muted">{t('my.exports.none')}</span>
            ) : (
              history.data.slice(0, 6).map((h) => (
                <div class="row gap2 small">
                  <Icon name="file" size="sm" />
                  <span class="ellipsis grow" data-tip={h.folder}>
                    {(h.names || []).join(', ')}
                  </span>
                  <span class="xs faint">{fmtRel(h.at)}</span>
                  <Button size="sm" kind="ghost" icon="folderOpen" tip={t('export.show_folder')} onClick={() => run(() => post('/api/export/reveal', { path: h.folder }))} />
                </div>
              ))
            )}
          </Card>
          <Card title={t('my.safety')} icon="history" class="grow">
            <Line icon="history" k={t('home.sys.backup')} v={backups.data?.[0] ? fmtRel(backups.data[0].created) : t('home.sys.backup.none')} go={go(() => openPanel('settings', 'backups'))} />
            <Line icon="cloud" k={t('stor.cloud')} v={p.storageMode === 'usb_only' ? t('my.cloud_unused') : p.provider ? providerName(p.provider) : t('my.no_cloud')} go={go(() => openPanel('settings', 'storage'))} />
            <Line icon="trash" k={t('ui.trash_bin')} v={t('my.trash_n', { n: trashN })} go={go(() => navigate('/ls/files'))} />
          </Card>
        </div>

        <div class="row gap4" style={{ alignItems: 'stretch', flexWrap: 'wrap' }}>
          <Card title={t('my.sessions')} icon="devices" class="grow">
            <Line icon={s.mode === 'portable' ? 'usb' : 'laptop'} k={t('my.this_session')} v={t('mode.' + s.mode)} />
            <Line icon="phone" k={t('hdr.mobile')} v={s.mobile.active ? t('status.mobile_on', { n: s.mobile.devices?.length || 0 }) : t('home.sys.off')} go={go(() => openPanel('settings', 'mobile'))} />
          </Card>
          <Card title={t('my.prefs')} icon="sliders" class="grow">
            <Line icon="globe" k={t('set.language')} v={LANG_NAMES[s.lang as Lang] || s.lang} go={go(() => openPanel('settings', 'general'))} />
            <Line icon="palette" k={t('set.theme')} v={t('set.theme.' + (p.settings.theme || 'system'))} go={go(() => openPanel('settings', 'general'))} />
            <Line icon="gauge" k="Turbo" v={s.settings.turbo ? t('ui.on') : t('ui.off')} go={go(() => openPanel('settings', 'general'))} />
          </Card>
        </div>
      </div>
    </Modal>
  );
}

function Stat(p: { icon: IconName; v?: number; k: string; go: () => void }) {
  return (
    <button type="button" class="stat" onClick={p.go} style={{ cursor: 'pointer', textAlign: 'left' }}>
      <span class="v">{p.v === undefined ? '…' : p.v}</span>
      <span class="k row gap1">
        <Icon name={p.icon} size="sm" />
        {p.k}
      </span>
    </button>
  );
}

function Card(p: { title: string; icon: IconName; children: any; class?: string }) {
  return (
    <div class={`card pad col gap2 ${p.class || ''}`} style={{ minWidth: 260, flexBasis: 0 }}>
      <div class="row gap2">
        <Icon name={p.icon} size="sm" />
        <h4>{p.title}</h4>
      </div>
      {p.children}
    </div>
  );
}

function Line(p: { icon: IconName; k: string; v: string; go?: () => void }) {
  return (
    <div class="status-item" role={p.go ? 'button' : undefined} tabIndex={p.go ? 0 : undefined} style={p.go ? { cursor: 'pointer' } : undefined} onClick={p.go} onKeyDown={(e) => e.key === 'Enter' && p.go?.()}>
      <Icon name={p.icon} size="sm" />
      <span class="k">{p.k}</span>
      <span class="v ellipsis muted">{p.v}</span>
    </div>
  );
}
