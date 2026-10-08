// Profile selection (up to five per account) and profile creation.
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { api, post } from '../lib/api';
import { changeAccount } from '../lib/actions';
import { fmtBytes } from '../lib/format';
import { errText, t } from '../lib/i18n';
import { navigate } from '../lib/route';
import { app, refresh, run, toast } from '../lib/state';
import { useStore } from '../lib/store';
import type { ProfileEntry } from '../lib/types';
import { LangSelect, ModeBadge, StorageLabel, storageIcons, ThemeToggle } from '../ui/common';
import { Icon } from '../ui/icons';
import { Avatar, BrandMark, Button, Field, Input, Modal, Notice, PasswordInput } from '../ui/kit';
import { ProviderCards, type Connected } from '../ui/providers';

export const AVATAR_MAX = 5 * 1024 * 1024;
export const AVATAR_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'];

export function Profiles() {
  const s = useStore(app, (x) => x.s)!;
  const acct = s.account!;
  const profiles = acct.profiles || [];
  const [creating, setCreating] = useState(false);
  const [unlocking, setUnlocking] = useState<ProfileEntry | null>(null);
  const [opening, setOpening] = useState('');

  const open = async (p: ProfileEntry) => {
    if (p.hasPassword) return setUnlocking(p);
    setOpening(p.id);
    const ok = await run(() => post(`/api/profiles/${p.id}/open`, { password: '' }));
    setOpening('');
    if (ok !== undefined) {
      await refresh();
      navigate('/', true);
    }
  };

  return (
    <div class="screen">
      <header class="header" style={{ background: 'transparent', borderBottom: 0, backdropFilter: 'none' }}>
        <div class="brand" style={{ cursor: 'default' }}>
          <BrandMark size={26} />
          <span class="wordmark">
            MNE <b>Lab</b>
          </span>
        </div>
        <ModeBadge mode={s.mode} />
        <span class="spacer" />
        <span class="small muted row gap1">
          <Icon name="user" size="sm" />
          {acct.name}
        </span>
        <LangSelect compact />
        <ThemeToggle />
        <Button kind="ghost" size="sm" icon="logout" onClick={changeAccount}>
          {t('hdr.change_account')}
        </Button>
      </header>
      <div class="profiles">
        <div class="col gap2" style={{ alignItems: 'center' }}>
          <h1>{profiles.length ? t('prof.who') : t('prof.first')}</h1>
          <p class="muted" style={{ textAlign: 'center', maxWidth: '36em' }}>
            {profiles.length ? t('prof.who.lead') : t('prof.first.lead')}
          </p>
        </div>
        <div class="profile-grid" role="list">
          {profiles.map((p, i) => (
            <button class="profile-tile" role="listitem" style={{ animationDelay: `${i * 40}ms` }} onClick={() => open(p)} aria-label={p.username} disabled={!!opening}>
              <span class="pic">
                <Avatar id={p.id} name={p.username} color={p.color} photo={p.avatar} hash={p.avatarHash} size={96} />
                {p.hasPassword && (
                  <span class="lock" data-tip={t('prof.has_password')}>
                    <Icon name="lock" />
                  </span>
                )}
              </span>
              <span class="name ellipsis">{opening === p.id ? <span class="spin" style={{ display: 'inline-block' }} /> : p.username}</span>
              <span class="meta">
                {storageIcons(p.storageMode).map((ic) => (
                  <Icon name={ic} />
                ))}
                {t('storage.' + p.storageMode)}
              </span>
            </button>
          ))}
          {!acct.maxReached && (
            <button class="profile-tile add" role="listitem" style={{ animationDelay: `${profiles.length * 40}ms` }} onClick={() => setCreating(true)} autoFocus={!profiles.length}>
              <span class="pic">
                <Icon name="plus" />
              </span>
              <span class="name">{t('prof.create')}</span>
              <span class="meta">{t('prof.count', { n: profiles.length, max: 5 })}</span>
            </button>
          )}
        </div>
        {acct.maxReached && (
          <p class="small faint row gap1">
            <Icon name="info" size="sm" />
            {t('prof.max_reached')}
          </p>
        )}
      </div>
      {creating && <CreateProfile onClose={() => setCreating(false)} />}
      {unlocking && <UnlockProfile profile={unlocking} onClose={() => setUnlocking(null)} />}
    </div>
  );
}

function UnlockProfile(p: { profile: ProfileEntry; onClose: () => void }) {
  const [pass, setPass] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const [recover, setRecover] = useState(false);
  const [rk, setRk] = useState('');
  const [np, setNp] = useState('');
  const go = async () => {
    setBusy(true);
    setErr('');
    const ok = recover
      ? await run(() => post(`/api/profiles/${p.profile.id}/recover`, { recoveryKey: rk.trim(), password: np }), setErr)
      : await run(() => post(`/api/profiles/${p.profile.id}/open`, { password: pass }), setErr);
    setBusy(false);
    if (ok !== undefined) {
      if (recover) toast('success', t('prof.recovered'));
      await refresh();
      navigate('/', true);
    }
  };
  return (
    <Modal
      size="narrow"
      onClose={p.onClose}
      foot={
        <>
          <Button kind="ghost" size="sm" onClick={() => (setRecover(!recover), setErr(''))}>
            {recover ? t('prof.use_password') : t('prof.forgot')}
          </Button>
          <span class="spacer" />
          <Button kind="primary" busy={busy} disabled={recover ? rk.trim().length < 10 : !pass} onClick={go}>
            {t('prof.open')}
          </Button>
        </>
      }
    >
      <div class="col gap4" style={{ alignItems: 'center', textAlign: 'center', paddingTop: 'var(--s4)' }}>
        <Avatar id={p.profile.id} name={p.profile.username} color={p.profile.color} photo={p.profile.avatar} hash={p.profile.avatarHash} size={72} />
        <h2>{p.profile.username}</h2>
      </div>
      <div class="col gap3" style={{ marginTop: 'var(--s4)' }}>
        {recover ? (
          <>
            <p class="small muted">{t('prof.recover.lead')}</p>
            <Field label={t('acct.rk.label')} id="prk">
              <Input id="prk" class="mono" icon="key" value={rk} onValue={setRk} autoFocus autocomplete="off" />
            </Field>
            <Field label={t('prof.password.new_optional')} id="pnp">
              <PasswordInput id="pnp" value={np} onValue={setNp} autocomplete="new-password" onEnter={go} />
            </Field>
          </>
        ) : (
          <Field label={t('prof.password')} id="ppw">
            <PasswordInput id="ppw" value={pass} onValue={setPass} autoFocus onEnter={go} invalid={!!err} />
          </Field>
        )}
        {err && <Notice kind="danger">{errText(err)}</Notice>}
      </div>
    </Modal>
  );
}

/** AvatarPicker previews a photo locally (GIFs keep animating) and checks
 * format and size before anything is sent. */
export function AvatarPicker(p: { name: string; color: number; file: File | null; onFile: (f: File | null) => void; current?: { id: string; photo: boolean; hash?: string } }) {
  const input = useRef<HTMLInputElement>(null);
  const [url, setUrl] = useState('');
  const [err, setErr] = useState('');
  useEffect(() => {
    if (!p.file) return setUrl('');
    const u = URL.createObjectURL(p.file);
    setUrl(u);
    return () => URL.revokeObjectURL(u);
  }, [p.file]);
  const pick = (f?: File) => {
    setErr('');
    if (!f) return;
    if (!AVATAR_TYPES.includes(f.type)) return setErr(t('err.avatar.unsupported_format'));
    if (f.size > AVATAR_MAX) return setErr(t('err.avatar.too_large'));
    p.onFile(f);
  };
  return (
    <div
      class="avatar-pick"
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        pick(e.dataTransfer?.files?.[0]);
      }}
    >
      <Avatar src={url || undefined} id={p.current?.id} photo={!url && p.current?.photo} hash={p.current?.hash} name={p.name || '?'} color={p.color} size={72} />
      <div class="col gap1" style={{ minWidth: 0 }}>
        <div class="row gap1">
          <Button size="sm" icon="imageUp" onClick={() => input.current?.click()}>
            {p.file || p.current?.photo ? t('prof.photo.change') : t('prof.photo.add')}
          </Button>
          {p.file && <Button size="sm" kind="ghost" icon="x" tip={t('prof.photo.remove')} onClick={() => p.onFile(null)} />}
        </div>
        <span class="xs faint">{err ? <span style={{ color: 'var(--danger)' }}>{err}</span> : p.file ? `${p.file.name} · ${fmtBytes(p.file.size)}` : t('prof.photo.hint')}</span>
        <input ref={input} type="file" accept=".png,.jpg,.jpeg,.webp,.gif,image/png,image/jpeg,image/webp,image/gif" hidden onChange={(e) => pick((e.target as HTMLInputElement).files?.[0])} />
      </div>
    </div>
  );
}

export function ColorPick(p: { value: number; onValue: (n: number) => void }) {
  return (
    <div class="colors" role="radiogroup" aria-label={t('prof.color')}>
      {[0, 1, 2, 3, 4, 5, 6, 7].map((c) => (
        <button type="button" role="radio" aria-checked={p.value === c} aria-label={t('prof.color') + ' ' + (c + 1)} class={p.value === c ? 'on' : ''} style={{ '--c': `var(--pc${c})` } as any} onClick={() => p.onValue(c)} />
      ))}
    </div>
  );
}

function CreateProfile(p: { onClose: () => void }) {
  const s = useStore(app, (x) => x.s)!;
  const modes = s.storageModes;
  const [step, setStep] = useState(0);
  const [username, setUsername] = useState('');
  const [color, setColor] = useState((s.account?.profiles?.length || 0) % 8);
  const [file, setFile] = useState<File | null>(null);
  const [mode, setMode] = useState(modes.length === 1 ? modes[0] : '');
  const [conn, setConn] = useState<Connected | null>(null);
  const [pass, setPass] = useState('');
  const [pass2, setPass2] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const cloud = mode === 'usb_cloud' || mode === 'cloud_only';
  const fileUrl = useMemo(() => (file ? URL.createObjectURL(file) : ''), [file]);
  useEffect(() => () => fileUrl && URL.revokeObjectURL(fileUrl), [fileUrl]);
  const taken = (s.account?.profiles || []).some((x) => x.username.toLowerCase() === username.trim().toLowerCase());

  const connect = async (provider: string, transport: 'api' | 'folder') => {
    setErr('');
    const r = await run(() => post<Connected & { connection: string }>('/api/providers/connect', { provider, transport }), setErr);
    if (r) setConn(r);
  };

  const create = async () => {
    setBusy(true);
    setErr('');
    const prof = await run(
      () => post<ProfileEntry>('/api/profiles', { username: username.trim(), storageMode: mode, provider: cloud ? conn?.provider : '', connection: cloud ? conn?.connection : '', password: pass, color }),
      setErr,
    );
    if (!prof) return setBusy(false);
    if (file) {
      const up = await run(() => api('PUT', `/api/profiles/${prof.id}/avatar`, undefined, { raw: file }));
      if (up === undefined) toast('warning', t('prof.photo.failed'));
    }
    const opened = await run(() => post(`/api/profiles/${prof.id}/open`, { password: pass }), setErr);
    setBusy(false);
    if (opened !== undefined) {
      await refresh();
      navigate('/', true);
    }
  };

  const steps = [t('prof.step.identity'), t('prof.step.storage'), t('prof.step.security')];
  const canNext = step === 0 ? username.trim().length > 0 && !taken : step === 1 ? !!mode && (!cloud || !!conn) : pass === pass2;

  return (
    <Modal
      size="wide"
      title={t('prof.create')}
      sub={steps.map((x, i) => (i === step ? <b style={{ color: 'var(--text)' }}>{x}</b> : <span>{x}</span>)).reduce((a: any[], x, i) => (i ? [...a, '  ·  ', x] : [x]), [])}
      icon="user"
      onClose={p.onClose}
      foot={
        <>
          {step > 0 && (
            <Button kind="ghost" icon="back" onClick={() => (setStep(step - 1), setErr(''))}>
              {t('ui.back')}
            </Button>
          )}
          <span class="spacer" />
          {step < 2 ? (
            <Button kind="primary" trail="next" disabled={!canNext} onClick={() => setStep(step + 1)}>
              {t('ui.next')}
            </Button>
          ) : (
            <Button kind="primary" icon="check" busy={busy} disabled={!canNext} onClick={create}>
              {t('prof.create.go')}
            </Button>
          )}
        </>
      }
    >
      <div class="steps" style={{ marginBottom: 'var(--s5)' }} aria-hidden="true">
        {steps.map((_, i) => (
          <span class={i <= step ? 'on' : ''} />
        ))}
      </div>
      {step === 0 && (
        <div class="col gap5">
          <Field label={t('prof.username')} hint={t('prof.username.hint')} error={taken ? t('err.profile.username_taken') : undefined} id="un">
            <Input id="un" icon="user" value={username} onValue={setUsername} maxLength={32} autoFocus onKeyDown={(e: KeyboardEvent) => e.key === 'Enter' && canNext && setStep(1)} />
          </Field>
          <div class="field">
            <span class="label">{t('prof.photo')}</span>
            <AvatarPicker name={username} color={color} file={file} onFile={setFile} />
          </div>
          <div class="field">
            <span class="label">{t('prof.color')}</span>
            <ColorPick value={color} onValue={setColor} />
          </div>
        </div>
      )}
      {step === 1 && (
        <div class="col gap5">
          <div class="field">
            <span class="label">{t('prof.storage_mode')}</span>
            <div class="storage-modes" role="radiogroup">
              {['usb_cloud', 'usb_only', 'cloud_only'].map((m) => {
                const ok = modes.includes(m);
                return (
                  <button
                    type="button"
                    role="radio"
                    aria-checked={mode === m}
                    class={`mode-card ${mode === m ? 'on' : ''}`}
                    disabled={!ok}
                    style={!ok ? { opacity: 0.5, cursor: 'not-allowed' } : undefined}
                    onClick={() => (setMode(m), m === 'usb_only' && setConn(null))}
                  >
                    <span class="ics">
                      {storageIcons(m).map((i) => (
                        <Icon name={i} />
                      ))}
                    </span>
                    <h4>{t('storage.' + m)}</h4>
                    <p>{ok ? t('storage.' + m + '.desc') : t('storage.needs_usb')}</p>
                  </button>
                );
              })}
            </div>
          </div>
          {cloud && (
            <div class="field">
              <span class="label">{t('prov.choose')}</span>
              <ProviderCards options={s.providers} selected={conn?.provider} connected={conn} onConnect={connect} />
              {s.providers.every((o) => !o.configured) && <Notice kind="info">{t('prov.none_available')}</Notice>}
              <Notice kind="info" icon="shieldOk">
                {t('prov.explain')}
              </Notice>
            </div>
          )}
          {err && <Notice kind="danger">{errText(err)}</Notice>}
        </div>
      )}
      {step === 2 && (
        <div class="col gap4">
          <p class="muted small">{t('prof.password.lead')}</p>
          <div class="row gap3" style={{ alignItems: 'flex-start' }}>
            <Field label={t('prof.password.optional')} class="grow" id="npw">
              <PasswordInput id="npw" value={pass} onValue={setPass} autocomplete="new-password" autoFocus />
            </Field>
            <Field label={t('prof.password.confirm')} class="grow" error={pass2 && pass !== pass2 ? t('acct.passphrase.mismatch') : undefined} id="npw2">
              <PasswordInput id="npw2" value={pass2} onValue={setPass2} autocomplete="new-password" />
            </Field>
          </div>
          <div class="card pad col gap3" style={{ background: 'var(--surface-2)' }}>
            <span class="section-title">{t('prof.summary')}</span>
            <div class="row gap3">
              <Avatar name={username} color={color} src={fileUrl || undefined} size={40} />
              <div class="col gap1" style={{ gap: 0 }}>
                <b>{username}</b>
                <span class="small muted">
                  <StorageLabel mode={mode} />
                </span>
              </div>
              <span class="spacer" />
              <span class="small muted row gap1">
                <Icon name={pass ? 'lock' : 'unlock'} size="sm" />
                {pass ? t('prof.with_password') : t('prof.without_password')}
              </span>
            </div>
          </div>
          {err && <Notice kind="danger">{errText(err)}</Notice>}
        </div>
      )}
    </Modal>
  );
}
