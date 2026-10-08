// Settings: general preferences, profile, account, storage and cloud,
// backups, updates and LockedBuild, mobile devices, privacy and about.
import type { ComponentChildren } from 'preact';
import { useState } from 'preact/hooks';
import { changeAccount, checkUpdates, markSeen, saveProfileSettings, seen, setLanguage, setTheme, setTurbo, syncNow } from '../lib/actions';
import { api, get, post } from '../lib/api';
import { fmtBytes, fmtDate, fmtRel } from '../lib/format';
import { errText, lang, LANG_NAMES, LANGS, t } from '../lib/i18n';
import { navigate } from '../lib/route';
import { app, closePanel, openPanel, refresh, run, toast } from '../lib/state';
import { useStore } from '../lib/store';
import type { Choice, StateView } from '../lib/types';
import { StorageLabel, syncView } from '../ui/common';
import { Icon, type IconName } from '../ui/icons';
import { Badge, Button, confirmDialog, Empty, Field, Modal, Notice, PasswordInput, Seg, Select, Skeleton, Switch, useAsync } from '../ui/kit';
import { ProviderCards, ProviderLogo, providerName, type Connected } from '../ui/providers';
import { AvatarPicker, ColorPick } from './profiles';

const TABS: { id: string; icon: IconName }[] = [
  { id: 'general', icon: 'sliders' },
  { id: 'profile', icon: 'user' },
  { id: 'account', icon: 'key' },
  { id: 'storage', icon: 'cloud' },
  { id: 'backups', icon: 'history' },
  { id: 'updates', icon: 'package' },
  { id: 'mobile', icon: 'phone' },
  { id: 'privacy', icon: 'shield' },
  { id: 'about', icon: 'info' },
];

export function Settings(p: { arg?: string | null }) {
  const [tab, setTab] = useState(p.arg && TABS.some((x) => x.id === p.arg) ? p.arg : 'general');
  const s = useStore(app, (x) => x.s)!;
  if (!s.profile) return null;
  return (
    <Modal size="xwide" icon="settings" title={t('settings.title')} onClose={closePanel} bodyClass="flush">
      <div class="settings">
        <nav aria-label={t('settings.title')}>
          {TABS.map((x) => (
            <button type="button" class={tab === x.id ? 'on' : ''} onClick={() => setTab(x.id)} aria-current={tab === x.id ? 'true' : undefined}>
              <Icon name={x.icon} size="sm" />
              <span class="grow">{t('settings.tab.' + x.id)}</span>
              {x.id === 'storage' && s.profile!.sync.conflicts > 0 && <span class="new-badge">{s.profile!.sync.conflicts}</span>}
              {x.id === 'updates' && (s.update.staged || (s.update.locked && s.update.newer)) && <span class="dot" style={{ color: 'var(--accent)' }} />}
            </button>
          ))}
        </nav>
        <div class="content">
          {tab === 'general' && <General s={s} />}
          {tab === 'profile' && <ProfileTab s={s} />}
          {tab === 'account' && <AccountTab s={s} />}
          {tab === 'storage' && <StorageTab s={s} />}
          {tab === 'backups' && <BackupsTab s={s} />}
          {tab === 'updates' && <UpdatesTab s={s} />}
          {tab === 'mobile' && <MobileTab s={s} />}
          {tab === 'privacy' && <PrivacyTab s={s} />}
          {tab === 'about' && <AboutTab s={s} />}
        </div>
      </div>
    </Modal>
  );
}

function Section(p: { title: string; children: ComponentChildren; sub?: string }) {
  return (
    <section class="col" style={{ gap: 0 }}>
      <h3 style={{ marginBottom: 2 }}>{p.title}</h3>
      {p.sub && <span class="small muted" style={{ marginBottom: 'var(--s2)' }}>{p.sub}</span>}
      {p.children}
    </section>
  );
}

function Row(p: { title: string; sub?: ComponentChildren; children?: ComponentChildren }) {
  return (
    <div class="setting">
      <div class="txt">
        <b>{p.title}</b>
        {p.sub && <span>{p.sub}</span>}
      </div>
      {p.children}
    </div>
  );
}

// ---- general ----

function General(p: { s: StateView }) {
  const s = p.s;
  const st = s.profile!.settings;
  return (
    <>
      <Section title={t('settings.tab.general')}>
        <Row title={t('set.language')} sub={t('set.language.d')}>
          <Select value={s.lang} onValue={(l) => run(() => setLanguage(l))} options={LANGS.map((l) => ({ value: l, label: LANG_NAMES[l] }))} label={t('set.language')} />
        </Row>
        <Row title={t('set.theme')}>
          <Seg
            size="sm"
            value={st.theme || 'system'}
            onValue={(th) => run(() => setTheme(th))}
            options={(['system', 'light', 'dark'] as const).map((th) => ({ value: th, label: t('set.theme.' + th), icon: th === 'light' ? 'sun' : th === 'dark' ? 'moon' : 'monitor' }))}
          />
        </Row>
        <Row title={t('set.decimal')} sub={t('set.decimal.d')}>
          <Seg
            size="sm"
            value={st.decimal || 'auto'}
            onValue={(d) => run(() => saveProfileSettings({ decimal: d === 'auto' ? '' : d }))}
            options={[
              { value: 'auto', label: t('set.decimal.auto') },
              { value: '.', label: '1.5' },
              { value: ',', label: '1,5' },
            ]}
          />
        </Row>
        <Row title="Turbo" sub={t('set.turbo.d')}>
          <Switch checked={s.settings.turbo} onChange={(on) => (seen('turbo') ? setTurbo(on) : openPanel('turbo'))} />
        </Row>
      </Section>
      <Section title={t('set.guidance')}>
        <Row title={t('set.tour')} sub={t('set.tour.d')}>
          <Button size="sm" icon="play" onClick={() => openPanel('tour')}>
            {t('set.tour.go')}
          </Button>
        </Row>
        <Row title={t('set.tips')} sub={t('set.tips.d')}>
          <Button size="sm" icon="reset" onClick={() => run(() => saveProfileSettings({ onboarding: {} })).then(() => toast('success', t('set.tips.reset')))}>
            {t('set.tips.go')}
          </Button>
        </Row>
      </Section>
    </>
  );
}

// ---- profile ----

function ProfileTab(p: { s: StateView }) {
  const pr = p.s.profile!;
  const [name, setName] = useState(pr.username);
  const [busy, setBusy] = useState('');
  const [pw, setPw] = useState({ cur: '', next: '', again: '' });
  const [delPw, setDelPw] = useState('');
  const [deleting, setDeleting] = useState(false);
  const patch = async (b: Record<string, unknown>) => {
    setBusy('patch');
    await run(() => api('PATCH', `/api/profiles/${pr.id}`, b));
    setBusy('');
    await refresh();
  };
  const upload = async (f: File | null) => {
    setBusy('avatar');
    if (f) await run(() => api('PUT', `/api/profiles/${pr.id}/avatar`, undefined, { raw: f, headers: { 'Content-Type': f.type } }));
    else await run(() => api('DELETE', `/api/profiles/${pr.id}/avatar`));
    setBusy('');
    await refresh();
  };
  const savePw = async (remove = false) => {
    setBusy('pw');
    const ok = await run(() => post('/api/profile/password', { current: pw.cur, next: remove ? '' : pw.next }));
    setBusy('');
    if (ok === undefined) return;
    setPw({ cur: '', next: '', again: '' });
    toast('success', remove ? t('prof.pw.removed') : t('prof.pw.saved'));
    refresh();
  };
  const del = async () => {
    setBusy('del');
    const ok = await run(() => api('DELETE', `/api/profiles/${pr.id}`, { password: delPw }));
    setBusy('');
    if (ok === undefined) return;
    closePanel();
    await refresh();
    navigate('/', true);
  };
  const pwValid = pw.next.length > 0 && pw.next === pw.again && (!pr.hasPassword || pw.cur);
  return (
    <>
      <Section title={t('settings.tab.profile')}>
        <Row title={t('prof.photo')}>
          <AvatarPicker name={pr.username} color={pr.color} file={null} onFile={upload} current={{ id: pr.id, photo: pr.avatar, hash: pr.avatarHash }} />
        </Row>
        {pr.avatar && (
          <Row title={t('prof.photo.remove')}>
            <Button size="sm" kind="ghost" icon="trash" busy={busy === 'avatar'} onClick={() => upload(null)}>
              {t('prof.photo.remove')}
            </Button>
          </Row>
        )}
        <Row title={t('prof.username')} sub={t('prof.username.hint')}>
          <div class="row gap2">
            <input class="input sm" value={name} maxLength={32} onInput={(e) => setName((e.target as HTMLInputElement).value)} aria-label={t('prof.username')} style={{ width: 200 }} />
            <Button size="sm" disabled={!name.trim() || name.trim() === pr.username} busy={busy === 'patch'} onClick={() => patch({ username: name.trim() })}>
              {t('ui.save')}
            </Button>
          </div>
        </Row>
        <Row title={t('prof.color')}>
          <ColorPick value={pr.color} onValue={(color) => patch({ color })} />
        </Row>
        <Row title={t('prof.mode')} sub={t('prof.mode.fixed')}>
          <StorageLabel mode={pr.storageMode} />
        </Row>
      </Section>
      <Section title={t('prof.pw')} sub={pr.hasPassword ? t('prof.pw.on') : t('prof.pw.off')}>
        <div class="col gap3" style={{ maxWidth: 420, paddingTop: 'var(--s2)' }}>
          {pr.hasPassword && (
            <Field label={t('prof.pw.current')}>
              <PasswordInput value={pw.cur} onValue={(cur) => setPw({ ...pw, cur })} autocomplete="current-password" />
            </Field>
          )}
          <Field label={t('prof.pw.new')} hint={t('prof.pw.hint')}>
            <PasswordInput value={pw.next} onValue={(next) => setPw({ ...pw, next })} autocomplete="new-password" />
          </Field>
          <Field label={t('prof.pw.again')} error={pw.again && pw.again !== pw.next ? t('acct.passphrase.mismatch') : undefined}>
            <PasswordInput value={pw.again} onValue={(again) => setPw({ ...pw, again })} autocomplete="new-password" onEnter={() => pwValid && savePw()} />
          </Field>
          <div class="row gap2">
            <Button kind="primary" size="sm" disabled={!pwValid} busy={busy === 'pw'} onClick={() => savePw()}>
              {pr.hasPassword ? t('prof.pw.change') : t('prof.pw.set')}
            </Button>
            {pr.hasPassword && (
              <Button size="sm" kind="ghost" disabled={!pw.cur} onClick={() => savePw(true)}>
                {t('prof.pw.remove')}
              </Button>
            )}
          </div>
        </div>
      </Section>
      <Section title={t('prof.delete')} sub={t('prof.delete.d')}>
        {!deleting ? (
          <div style={{ paddingTop: 'var(--s2)' }}>
            <Button size="sm" kind="danger" icon="trash" onClick={() => setDeleting(true)}>
              {t('prof.delete')}
            </Button>
          </div>
        ) : (
          <div class="col gap3" style={{ maxWidth: 420, paddingTop: 'var(--s2)' }}>
            <Notice kind="danger">{t('prof.delete.warn', { name: pr.username })}</Notice>
            {pr.hasPassword && (
              <Field label={t('prof.pw.current')}>
                <PasswordInput value={delPw} onValue={setDelPw} autocomplete="current-password" />
              </Field>
            )}
            <div class="row gap2">
              <Button size="sm" onClick={() => setDeleting(false)}>
                {t('ui.cancel')}
              </Button>
              <Button size="sm" kind="danger" busy={busy === 'del'} disabled={pr.hasPassword && !delPw} onClick={del}>
                {t('prof.delete.go')}
              </Button>
            </div>
          </div>
        )}
      </Section>
    </>
  );
}

// ---- account ----

function AccountTab(p: { s: StateView }) {
  const s = p.s;
  const acct = s.account!;
  const [pp, setPp] = useState({ cur: '', next: '', again: '' });
  const [busy, setBusy] = useState(false);
  const valid = pp.cur && pp.next.length >= 8 && pp.next === pp.again;
  const change = async () => {
    setBusy(true);
    const ok = await run(() => post('/api/account/passphrase', { current: pp.cur, next: pp.next }));
    setBusy(false);
    if (ok === undefined) return;
    setPp({ cur: '', next: '', again: '' });
    toast('success', t('acct.pp.saved'));
  };
  const remember = async (on: boolean) => {
    const ok = await run(() => post('/api/account/remember', { on }));
    if (ok !== undefined) refresh();
  };
  return (
    <>
      <Section title={t('settings.tab.account')}>
        <Row title={acct.name || t('acct.title')} sub={t('acct.profiles', { n: acct.profiles?.length || 0 })}>
          <Button size="sm" icon="logout" onClick={() => (closePanel(), changeAccount())}>
            {t('hdr.change_account')}
          </Button>
        </Row>
        <Row title={t('acct.remember')} sub={s.mode === 'portable' ? t('acct.remember.d') : t('avail.portable_only')}>
          <Switch checked={acct.remembered} disabled={s.mode !== 'portable' && !acct.remembered} onChange={remember} />
        </Row>
        <Row title={t('acct.recovery')} sub={t('acct.recovery.d')} />
      </Section>
      <Section title={t('acct.pp')} sub={t('acct.pp.d')}>
        <div class="col gap3" style={{ maxWidth: 420, paddingTop: 'var(--s2)' }}>
          <Field label={t('acct.pp.current')}>
            <PasswordInput value={pp.cur} onValue={(cur) => setPp({ ...pp, cur })} autocomplete="current-password" />
          </Field>
          <Field label={t('acct.pp.new')} hint={t('start.pp.hint')}>
            <PasswordInput value={pp.next} onValue={(next) => setPp({ ...pp, next })} autocomplete="new-password" />
          </Field>
          <Field label={t('acct.pp.again')} error={pp.again && pp.again !== pp.next ? t('acct.passphrase.mismatch') : undefined}>
            <PasswordInput value={pp.again} onValue={(again) => setPp({ ...pp, again })} autocomplete="new-password" onEnter={() => valid && change()} />
          </Field>
          <div>
            <Button kind="primary" size="sm" disabled={!valid} busy={busy} onClick={change}>
              {t('acct.pp.change')}
            </Button>
          </div>
        </div>
      </Section>
    </>
  );
}

// ---- storage and cloud ----

type Conflict = { collection: string; id: string; localDeleted: boolean; localUpdated: string; remote: { hash: string; deleted: boolean; updated: string; device: string }[] };

function StorageTab(p: { s: StateView }) {
  const s = p.s;
  const pr = s.profile!;
  const busy = (s.activities || []).some((a) => a.kind === 'sync');
  const v = syncView(pr.sync, busy);
  const cloud = pr.storageMode !== 'usb_only';
  const conn = useAsync(() => get<{ provider?: string; transport?: string; account?: Connected['account'] }>('/api/profile/provider'), [pr.provider, pr.sync.state]);
  const conflicts = useAsync(() => (cloud ? get<Conflict[]>('/api/sync/conflicts') : Promise.resolve([] as Conflict[])), [pr.sync.conflicts]);
  const [changing, setChanging] = useState(false);
  const connect = async (id: string, transport: 'api' | 'folder') => {
    const r = await run(() => post('/api/profile/provider', { provider: id, transport }));
    if (r === undefined) return;
    setChanging(false);
    if (!seen('cloud')) markSeen('cloud');
    toast('success', t('stor.connected', { name: providerName(id) }));
    refresh();
    conn.reload();
  };
  const disconnect = async () => {
    if (!(await confirmDialog({ title: t('stor.disconnect.q', { name: providerName(pr.provider) }), body: t('stor.disconnect.d'), confirm: t('stor.disconnect'), danger: true }))) return;
    let r = await run(() => api('DELETE', '/api/profile/provider'), (code) => code !== 'provider.pending_changes' && toast('error', errText(code)));
    if (r === undefined) {
      if (!(await confirmDialog({ title: t('stor.pending.q'), body: t('stor.pending.d', { n: pr.sync.pending }), confirm: t('stor.disconnect_anyway'), danger: true }))) return;
      r = await run(() => api('DELETE', '/api/profile/provider?force=1'));
      if (r === undefined) return;
    }
    refresh();
    conn.reload();
  };
  const resolve = async (c: Conflict, choice: string) => {
    const r = await run(() => post('/api/sync/conflicts/resolve', { collection: c.collection, id: c.id, choice }));
    if (r !== undefined) (conflicts.reload(), refresh());
  };
  return (
    <>
      <Section title={t('settings.tab.storage')}>
        <Row title={t('stor.mode')} sub={t('stor.mode.d.' + pr.storageMode)}>
          <StorageLabel mode={pr.storageMode} />
        </Row>
        <Row title={t('stor.state')} sub={v.detail || (pr.sync.lastSync ? t('status.last_sync', { when: fmtRel(pr.sync.lastSync) }) : undefined)}>
          <div class="row gap2">
            <Badge kind={v.tone === 'ok' ? 'success' : v.tone === 'busy' ? 'info' : v.tone === 'warn' ? 'warning' : 'danger'}>{v.text}</Badge>
            {cloud && (
              <Button size="sm" icon="refresh" busy={busy} onClick={syncNow}>
                {t('status.sync_now')}
              </Button>
            )}
          </div>
        </Row>
        {pr.sync.pending > 0 && <Row title={t('stor.pending')} sub={t('stor.pending.n', { n: pr.sync.pending })} />}
      </Section>
      {cloud && (
        <Section title={t('stor.cloud')} sub={t('stor.cloud.d')}>
          {conn.loading && !conn.data ? (
            <Skeleton h={64} />
          ) : conn.data?.provider && !changing ? (
            <div class="setting">
              <ProviderLogo id={conn.data.provider} size={36} />
              <div class="txt">
                <b>{providerName(conn.data.provider)}</b>
                <span>{[conn.data.account?.displayName, conn.data.account?.maskedEmail, conn.data.transport === 'folder' ? t('prov.via_folder') : ''].filter(Boolean).join(' · ') || t('prov.connected')}</span>
              </div>
              <div class="row gap2">
                <Button size="sm" onClick={() => setChanging(true)}>
                  {t('stor.change')}
                </Button>
                <Button size="sm" kind="ghost" onClick={disconnect}>
                  {t('stor.disconnect')}
                </Button>
              </div>
            </div>
          ) : (
            <div class="col gap3" style={{ paddingTop: 'var(--s2)' }}>
              {pr.provider && !conn.data?.provider && <Notice kind="warning">{t('stor.reconnect', { name: providerName(pr.provider) })}</Notice>}
              {changing && <Notice icon="info">{t('stor.change.d')}</Notice>}
              <ProviderCards options={s.providers} selected={pr.provider} onConnect={connect} />
              {changing && (
                <div>
                  <Button size="sm" kind="ghost" onClick={() => setChanging(false)}>
                    {t('ui.cancel')}
                  </Button>
                </div>
              )}
            </div>
          )}
        </Section>
      )}
      {cloud && (conflicts.data?.length || 0) > 0 && (
        <Section title={t('stor.conflicts')} sub={t('stor.conflicts.d')}>
          {conflicts.data!.map((c) => (
            <div class="setting">
              <Icon name="layers" />
              <div class="txt">
                <b>{t('stor.coll.' + c.collection) !== 'stor.coll.' + c.collection ? t('stor.coll.' + c.collection) : c.collection}</b>
                <span>
                  {t('stor.conflict.mine', { when: fmtDate(c.localUpdated, true) })}
                  {c.remote.map((r) => ' · ' + t('stor.conflict.theirs', { when: fmtDate(r.updated, true) }))}
                </span>
              </div>
              <div class="row gap1">
                <Button size="sm" onClick={() => resolve(c, 'mine')}>
                  {t('stor.keep_mine')}
                </Button>
                <Button size="sm" onClick={() => resolve(c, 'theirs')}>
                  {t('stor.keep_theirs')}
                </Button>
                <Button size="sm" kind="primary" onClick={() => resolve(c, 'both')}>
                  {t('stor.keep_both')}
                </Button>
              </div>
            </div>
          ))}
        </Section>
      )}
    </>
  );
}

// ---- backups ----

type Backup = { file: string; created: string; size: number; reason: string; schema: number; verified: boolean };

function BackupsTab(p: { s: StateView }) {
  const list = useAsync(() => get<Backup[] | null>('/api/backups').then((x) => x || []), []);
  const [busy, setBusy] = useState('');
  const create = async () => {
    setBusy('new');
    const r = await run(() => post('/api/backups'));
    setBusy('');
    if (r !== undefined) (toast('success', t('bk.created')), list.reload());
  };
  const restore = async (b: Backup) => {
    if (!(await confirmDialog({ title: t('bk.restore.q', { when: fmtDate(b.created, true) }), body: t('bk.restore.d'), confirm: t('bk.restore') }))) return;
    setBusy(b.file);
    const r = await run(() => post('/api/backups/restore', { file: b.file }));
    setBusy('');
    if (r !== undefined) {
      toast('success', t('bk.restored'));
      app.set((x) => ({ libraryRev: x.libraryRev + 1 }));
      list.reload();
    }
  };
  return (
    <Section title={t('settings.tab.backups')} sub={t(p.s.mode === 'portable' ? 'bk.d.portable' : 'bk.d.temporary')}>
      <div class="row" style={{ padding: 'var(--s2) 0' }}>
        <span class="small muted grow">{t('bk.auto')}</span>
        <Button size="sm" kind="primary" icon="save" busy={busy === 'new'} onClick={create}>
          {t('bk.create')}
        </Button>
      </div>
      {list.loading && !list.data ? (
        <Skeleton h={120} />
      ) : !list.data?.length ? (
        <Empty icon="history" title={t('bk.none')} body={t('bk.none.d')} />
      ) : (
        list.data.map((b) => (
          <div class="setting">
            <Icon name={b.verified ? 'shieldOk' : 'warning'} />
            <div class="txt">
              <b>{fmtDate(b.created, true)}</b>
              <span>
                {t('bk.reason.' + b.reason)} · {fmtBytes(b.size)} · {b.verified ? t('bk.verified') : t('bk.unverified')}
              </span>
            </div>
            <Button size="sm" busy={busy === b.file} disabled={!!busy || !b.verified} onClick={() => restore(b)}>
              {t('bk.restore')}
            </Button>
          </div>
        ))
      )}
    </Section>
  );
}

// ---- updates and LockedBuild ----

function UpdatesTab(p: { s: StateView }) {
  const u = p.s.update;
  const [busy, setBusy] = useState('');
  const [explain, setExplain] = useState(false);
  const toggleLocked = async (on: boolean) => {
    if (on && !seen('locked')) return setExplain(true);
    setBusy('locked');
    const r = await run(() => post('/api/update/locked', { on }));
    setBusy('');
    if (r !== undefined) refresh();
  };
  const choose = async (c: Choice) => {
    const down = !c.newer && !c.current;
    const notes = c.notes?.[lang()] || c.notes?.en;
    const ok = await confirmDialog({
      title: down ? t('upd.downgrade.q', { v: c.version }) : t('upd.choose.q', { v: c.version }),
      body: (
        <div class="col gap2">
          {notes && <span>{notes}</span>}
          <span>{down ? t('upd.downgrade.d') : t('upd.choose.d')}</span>
          {c.migration && <span>{t('upd.migration')}</span>}
        </div>
      ),
      confirm: down ? t('upd.downgrade') : t('upd.install'),
      danger: down,
    });
    if (!ok) return;
    setBusy(c.version);
    const r = await run(() => post('/api/update/choose', { version: c.version }));
    setBusy('');
    if (r !== undefined) (toast('success', t('upd.chosen', { v: c.version })), refresh());
  };
  const check = async () => {
    setBusy('check');
    await checkUpdates();
    setBusy('');
  };
  return (
    <>
      <Section title={t('settings.tab.updates')}>
        <Row title={t('upd.current', { v: u.current })} sub={[t('upd.channel.' + (u.channel || 'stable')), u.buildDate ? fmtDate(u.buildDate) : '', u.platform].filter(Boolean).join(' · ')}>
          <Button size="sm" icon="refresh" busy={busy === 'check'} disabled={!u.configured || !u.online} tip={!u.online ? t('avail.needs_internet') : undefined} onClick={check}>
            {t('upd.check')}
          </Button>
        </Row>
        <Row
          title={u.locked ? 'LockedBuild' : t('upd.auto')}
          sub={!u.configured ? t('upd.not_configured') : u.locked ? t('upd.locked.on', { v: u.current }) : u.portable ? t('upd.auto.d') : t('upd.auto.d.temporary')}
        />
        {u.staged && <Notice kind="success" icon="package">{t('upd.staged', { v: u.staged })}</Notice>}
        {u.error && <Notice kind="warning">{errText(u.error)}</Notice>}
        {u.lastCheck && <span class="xs faint">{t('upd.last_check', { when: fmtRel(u.lastCheck) })}</span>}
      </Section>
      <Section title="LockedBuild" sub={t('upd.locked.d')}>
        <Row title={t('upd.locked.switch')} sub={u.portable ? t('upd.locked.switch.d') : t('upd.locked.portable_only')}>
          <Switch checked={u.locked} disabled={!u.portable || busy === 'locked'} onChange={toggleLocked} />
        </Row>
        {u.locked && (
          <div class="builds" style={{ paddingTop: 'var(--s2)' }}>
            {!u.choices?.length ? (
              <span class="small muted">{u.online ? t('upd.builds.none') : t('upd.builds.offline')}</span>
            ) : (
              u.choices.map((c) => (
                <div class={`build ${c.current ? 'current' : ''}`}>
                  <Icon name={c.current ? 'pin' : c.security ? 'shield' : 'package'} />
                  <div class="col grow" style={{ gap: 2, minWidth: 0 }}>
                    <div class="row gap2">
                      <b>{c.version}</b>
                      {c.current && <Badge kind="accent">{t('upd.b.current')}</Badge>}
                      {c.latest && <Badge kind="success">{t('upd.b.latest')}</Badge>}
                      {c.security && <Badge kind="warning">{t('upd.b.security')}</Badge>}
                      {!c.compatible && <Badge kind="danger">{t('upd.b.incompatible')}</Badge>}
                    </div>
                    <span class="xs muted ellipsis">{c.notes?.[lang()] || c.notes?.en || fmtDate(c.date)}</span>
                    {c.problem && <span class="xs" style={{ color: 'var(--danger)' }}>{errText(c.problem)}</span>}
                  </div>
                  {!c.current && (
                    <Button size="sm" kind={c.newer ? 'primary' : 'default'} busy={busy === c.version} disabled={!c.compatible || !!busy} onClick={() => choose(c)}>
                      {c.newer ? t('upd.install') : t('upd.downgrade')}
                    </Button>
                  )}
                </div>
              ))
            )}
          </div>
        )}
      </Section>
      {explain && (
        <Modal
          size="narrow"
          icon="lock"
          title={t('upd.locked.intro')}
          onClose={() => setExplain(false)}
          foot={
            <>
              <span class="spacer" />
              <Button onClick={() => setExplain(false)}>{t('ui.cancel')}</Button>
              <Button
                kind="primary"
                onClick={async () => {
                  setExplain(false);
                  await markSeen('locked');
                  toggleLocked(true);
                }}
              >
                {t('upd.locked.enable')}
              </Button>
            </>
          }
        >
          <div class="col gap2 muted">
            <span>{t('upd.locked.intro.1')}</span>
            <span>{t('upd.locked.intro.2')}</span>
            <span>{t('upd.locked.intro.3')}</span>
          </div>
        </Modal>
      )}
    </>
  );
}

// ---- mobile ----

function MobileTab(p: { s: StateView }) {
  const m = p.s.mobile;
  const revoke = async (id: string) => {
    if (await confirmDialog({ title: t('mob.revoke.q'), body: t('mob.revoke.d'), confirm: t('mob.revoke'), danger: true })) run(() => api('DELETE', '/api/mobile/devices/' + id)).then(refresh);
  };
  return (
    <Section title={t('settings.tab.mobile')} sub={t('mob.d')}>
      <Row title={t('hdr.mobile')} sub={m.active ? t('mob.on', { addr: m.address || '' }) : t('home.sys.off')}>
        <div class="row gap2">
          <Button size="sm" kind="primary" icon="qr" onClick={() => openPanel('mobile')}>
            {m.active ? t('mob.show_code') : t('home.mobile.go')}
          </Button>
          {m.active && (
            <Button size="sm" kind="ghost" onClick={() => run(() => post('/api/mobile/stop')).then(refresh)}>
              {t('mob.stop')}
            </Button>
          )}
        </div>
      </Row>
      <span class="section-title" style={{ marginTop: 'var(--s4)' }}>
        {t('mob.devices')}
      </span>
      {!m.devices?.length ? (
        <span class="small muted" style={{ padding: 'var(--s2) 0' }}>
          {t('mob.devices.none')}
        </span>
      ) : (
        m.devices.map((d) => (
          <div class="setting">
            <Icon name="phone" />
            <div class="txt">
              <b>{d.label || t('mob.device')}</b>
              <span>{t('mob.device.seen', { paired: fmtDate(d.paired, true), seen: fmtRel(d.lastSeen) })}</span>
            </div>
            <Button size="sm" kind="ghost" onClick={() => revoke(d.id)}>
              {t('mob.revoke')}
            </Button>
          </div>
        ))
      )}
      <span class="xs faint" style={{ marginTop: 'var(--s3)' }}>
        {t('mob.privacy')}
      </span>
    </Section>
  );
}

// ---- privacy ----

function PrivacyTab(p: { s: StateView }) {
  const points: { icon: IconName; k: string }[] = [
    { icon: 'server', k: 'no_server' },
    { icon: 'shieldOk', k: 'encrypted' },
    { icon: 'activity', k: 'no_telemetry' },
    { icon: 'cloud', k: 'your_cloud' },
    { icon: 'download', k: 'exports' },
    { icon: 'phone', k: 'mobile' },
    { icon: 'package', k: 'updates' },
    { icon: p.s.mode === 'temporary' ? 'laptop' : 'usb', k: p.s.mode === 'temporary' ? 'temporary' : 'portable' },
  ];
  return (
    <Section title={t('settings.tab.privacy')} sub={t('priv.d')}>
      <div class="col gap2" style={{ paddingTop: 'var(--s2)' }}>
        {points.map((x) => (
          <div class="setting" style={{ alignItems: 'flex-start' }}>
            <Icon name={x.icon} />
            <div class="txt">
              <b>{t('priv.' + x.k)}</b>
              <span>{t('priv.' + x.k + '.d')}</span>
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

// ---- about ----

const THIRD_PARTY: [string, string][] = [
  ['Preact', 'MIT'],
  ['Lucide icons', 'ISC'],
  ['noble-ciphers, noble-hashes', 'MIT'],
  ['Inter, JetBrains Mono', 'SIL OFL 1.1'],
  ['bbolt', 'MIT'],
  ['excelize, efp, nfp', 'BSD-3-Clause'],
  ['go-pdf/fpdf', 'MIT'],
  ['fogleman/gg', 'MIT'],
  ['golang/freetype', 'FreeType License'],
  ['nativewebp', 'MIT'],
  ['go-qrcode', 'MIT'],
  ['fyne.io/systray', 'Apache-2.0'],
  ['godbus/dbus', 'BSD-2-Clause'],
  ['mscfb, msoleps', 'Apache-2.0'],
  ['go-deepcopy', 'MIT'],
  ['Go x/crypto, x/image, x/net, x/sys, x/text', 'BSD-3-Clause'],
];

function AboutTab(p: { s: StateView }) {
  const s = p.s;
  return (
    <>
      <Section title="MNE Lab" sub={t('about.d')}>
        <Row title={t('about.version')} sub={[s.channel, s.buildDate ? fmtDate(s.buildDate) : ''].filter(Boolean).join(' · ')}>
          <span class="num">{s.version}</span>
        </Row>
        <Row title={t('about.mode')} sub={t('mode.' + s.mode + '.tip')}>
          <span>{t('mode.' + s.mode)}</span>
        </Row>
        <Row title={t('about.schema')}>
          <span class="num">{s.update.dataSchema}</span>
        </Row>
        <Row title={t('about.exports')} sub={s.exports} />
      </Section>
      <Section title={t('about.science')} sub={t('about.science.d')}>
        {Object.entries(s.science || {}).map(([k, v]) => (
          <Row title={t('about.science.' + k)}>
            <span class="num small">{v}</span>
          </Row>
        ))}
      </Section>
      <Section title={t('about.licenses')} sub={t('about.licenses.d')}>
        {THIRD_PARTY.map(([n, l]) => (
          <Row title={n}>
            <span class="small muted">{l}</span>
          </Row>
        ))}
        <span class="xs faint" style={{ marginTop: 'var(--s3)' }}>
          {t('about.trademarks')}
        </span>
      </Section>
      <span class="small muted">made by oaovito</span>
    </>
  );
}
