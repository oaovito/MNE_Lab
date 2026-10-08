// First interaction: Create Account or Sign In, unlocking an account kept
// on this drive, account recovery and the Temporary Mode recovery prompt.
import { useEffect, useState } from 'preact/hooks';
import { post } from '../lib/api';
import { fmtDate } from '../lib/format';
import { errText, t } from '../lib/i18n';
import { app, refresh, run, toast } from '../lib/state';
import { createStore, useStore } from '../lib/store';
import type { AccountView } from '../lib/types';
import { LangSelect, ModeBadge, ThemeToggle } from '../ui/common';
import { DirPicker } from '../ui/dirpicker';
import { Icon } from '../ui/icons';
import { BrandMark, Button, Check, confirmDialog, Field, Input, MenuButton, Notice, PasswordInput, Switch } from '../ui/kit';
import { ProviderCards, ProviderLogo, providerName } from '../ui/providers';

/** While the recovery key is shown, the start screen stays in front even
 * though the new account is already unlocked. */
export const startHold = createStore<{ recoveryKey: string | null }>({ recoveryKey: null });

type View = 'welcome' | 'create' | 'signin' | 'unlock' | 'recover';

export function Start() {
  const s = useStore(app, (x) => x.s)!;
  const hold = useStore(startHold, (x) => x.recoveryKey);
  const [view, setView] = useState<View>(s.accounts.length ? 'unlock' : 'welcome');
  const [accountId, setAccountId] = useState(s.settings.lastAccount && s.accounts.some((a) => a.id === s.settings.lastAccount) ? s.settings.lastAccount : s.accounts[0]?.id || '');

  let body;
  if (hold) body = <RecoveryKey value={hold} />;
  else if (view === 'create') body = <CreateAccount onBack={() => setView(s.accounts.length ? 'unlock' : 'welcome')} />;
  else if (view === 'signin') body = <SignIn onBack={() => setView(s.accounts.length ? 'unlock' : 'welcome')} />;
  else if (view === 'recover') body = <RecoverAccount id={accountId} onBack={() => setView('unlock')} />;
  else if (view === 'unlock' && s.accounts.length)
    body = <Unlock accountId={accountId} setAccountId={setAccountId} onCreate={() => setView('create')} onSignIn={() => setView('signin')} onRecover={() => setView('recover')} />;
  else body = <Welcome onCreate={() => setView('create')} onSignIn={() => setView('signin')} />;

  return (
    <div class="start screen">
      <Art />
      <main class="start-main">
        <div class="start-top">
          <ModeBadge mode={s.mode} />
          <span class="spacer" />
          <LangSelect />
          <ThemeToggle />
        </div>
        <div class="start-body">
          <div class="col gap4" style={{ width: '100%', alignItems: 'center' }}>
            {!hold && <Notices />}
            {body}
          </div>
        </div>
      </main>
    </div>
  );
}

function Art() {
  const s = useStore(app, (x) => x.s)!;
  return (
    <aside class="start-art" aria-hidden="true">
      <svg class="rays" viewBox="0 0 800 500" fill="none">
        <rect x="-40" y="292" width="420" height="16" rx="8" fill="url(#a1)" />
        <circle cx="400" cy="300" r="44" fill="url(#a2)" />
        <path d="M400 256 L400 40" stroke="url(#a3)" stroke-width="8" stroke-linecap="round" />
        {[-58, -34, 34, 58, 150, 128, -150, -128].map((deg, i) => {
          const r = (deg * Math.PI) / 180;
          return <line x1={400 + Math.sin(r) * 70} y1={300 - Math.cos(r) * 70} x2={400 + Math.sin(r) * 300} y2={300 - Math.cos(r) * 300} stroke="#5eead4" stroke-opacity={i < 4 ? 0.22 : 0.1} stroke-width="5" stroke-linecap="round" />;
        })}
        <path d="M330 26 A300 300 0 0 1 470 26" stroke="#5eead4" stroke-opacity="0.55" stroke-width="10" stroke-linecap="round" />
        <defs>
          <linearGradient id="a1" x1="0" x2="1">
            <stop offset="0" stop-color="#2dd4bf" stop-opacity="0" />
            <stop offset="1" stop-color="#ccfbf1" />
          </linearGradient>
          <radialGradient id="a2" cx="0.38" cy="0.34" r="0.75">
            <stop offset="0" stop-color="#fff" />
            <stop offset="0.45" stop-color="#a7f3e9" />
            <stop offset="1" stop-color="#0fa394" />
          </radialGradient>
          <linearGradient id="a3" x1="0" x2="0" y1="1" y2="0">
            <stop offset="0" stop-color="#ccfbf1" />
            <stop offset="1" stop-color="#5eead4" stop-opacity="0.2" />
          </linearGradient>
        </defs>
      </svg>
      <div class="row gap3" style={{ position: 'relative' }}>
        <BrandMark size={40} />
        <span class="wordmark" style={{ fontSize: 'var(--fs-xl)' }}>
          MNE <b>Lab</b>
        </span>
      </div>
      <div class="col gap4" style={{ position: 'relative' }}>
        <h1>{t('start.headline')}</h1>
        <p>{t('start.tagline')}</p>
        <div class="facts">
          <div class="fact">
            <Icon name="shieldOk" />
            <span>{t('start.fact.private')}</span>
          </div>
          <div class="fact">
            <Icon name="wifiOff" />
            <span>{t('start.fact.offline')}</span>
          </div>
          <div class="fact">
            <Icon name={s.mode === 'temporary' ? 'laptop' : 'usb'} />
            <span>{t(s.mode === 'temporary' ? 'start.fact.temporary' : 'start.fact.portable')}</span>
          </div>
        </div>
      </div>
      <div class="xs" style={{ position: 'relative', color: '#566272' }}>
        {t('start.version', { v: s.version })}
      </div>
    </aside>
  );
}

function Notices() {
  const s = useStore(app, (x) => x.s)!;
  const [busy, setBusy] = useState('');
  const rec = s.recoveries || [];
  return (
    <>
      {s.crashed && (
        <div class="start-card" style={{ gap: 0 }}>
          <Notice kind="info" icon="lifebuoy">
            {t('start.crashed')}
          </Notice>
        </div>
      )}
      {rec.map((r) => (
        <div class="start-card" style={{ gap: 0 }}>
          <Notice
            kind="warning"
            icon="lifebuoy"
            action={
              <div class="row gap1">
                <Button
                  size="sm"
                  kind="ghost"
                  disabled={!!busy}
                  onClick={async () => {
                    const ok = await confirmDialog({ title: t('rec.discard.title'), body: t('rec.discard.body'), confirm: t('rec.discard'), danger: true });
                    if (!ok) return;
                    setBusy('d' + r.account);
                    await run(() => post('/api/recoveries/discard', { account: r.account }));
                    setBusy('');
                    refresh();
                  }}
                >
                  {t('rec.discard')}
                </Button>
                <Button
                  size="sm"
                  kind="primary"
                  busy={busy === 'r' + r.account}
                  onClick={async () => {
                    setBusy('r' + r.account);
                    const ok = await run(() => post('/api/recoveries/restore', { account: r.account }));
                    setBusy('');
                    if (ok !== undefined) toast('success', t('rec.restored'));
                    refresh();
                  }}
                >
                  {t('rec.restore')}
                </Button>
              </div>
            }
          >
            <strong>{t(r.source === 'interrupted' ? 'rec.interrupted' : 'rec.title')}</strong>
            <div>{t('rec.body', { when: fmtDate(r.created, true) })}</div>
          </Notice>
        </div>
      ))}
    </>
  );
}

function Welcome(p: { onCreate: () => void; onSignIn: () => void }) {
  return (
    <div class="start-card">
      <div class="col gap2">
        <h1>{t('start.welcome')}</h1>
        <p class="lead">{t('start.welcome.lead')}</p>
      </div>
      <div class="col gap3">
        <button class="choice primary" onClick={p.onCreate} autoFocus>
          <span class="ic">
            <Icon name="sparkles" />
          </span>
          <span class="grow col gap1">
            <h3>{t('start.create')}</h3>
            <p>{t('start.create.desc')}</p>
          </span>
          <Icon name="right" />
        </button>
        <button class="choice" onClick={p.onSignIn}>
          <span class="ic">
            <Icon name="cloud" />
          </span>
          <span class="grow col gap1">
            <h3>{t('start.signin')}</h3>
            <p>{t('start.signin.desc')}</p>
          </span>
          <Icon name="right" />
        </button>
      </div>
      <p class="xs faint">{t('start.no_central')}</p>
    </div>
  );
}

function Unlock(p: { accountId: string; setAccountId: (id: string) => void; onCreate: () => void; onSignIn: () => void; onRecover: () => void }) {
  const s = useStore(app, (x) => x.s)!;
  const [pass, setPass] = useState('');
  const [remember, setRemember] = useState(false);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const acct = s.accounts.find((a) => a.id === p.accountId) || s.accounts[0];
  useEffect(() => {
    setPass('');
    setErr('');
  }, [p.accountId]);
  const unlock = async () => {
    if (!acct) return;
    setBusy(true);
    setErr('');
    const v = acct.remembered && !pass ? await run(() => post<AccountView>('/api/account/remembered', { id: acct.id }), setErr) : await run(() => post<AccountView>('/api/account/unlock', { id: acct.id, passphrase: pass, remember }), setErr);
    setBusy(false);
    if (v) refresh();
  };
  return (
    <div class="start-card">
      <div class="col gap2">
        <h1>{t('start.welcome_back')}</h1>
        <p class="lead">{t('start.unlock.lead')}</p>
      </div>
      {s.accounts.length > 1 && (
        <div class="col gap2" role="listbox" aria-label={t('start.accounts')}>
          {s.accounts.map((a) => (
            <button class={`account-row ${a.id === acct?.id ? 'on' : ''}`} role="option" aria-selected={a.id === acct?.id} onClick={() => p.setAccountId(a.id)}>
              <Icon name="user" />
              <span class="grow col gap1" style={{ gap: 0 }}>
                <b class="small">{t('start.account_created', { date: fmtDate(a.created) })}</b>
                <span class="xs faint mono">{a.id.slice(0, 8)}</span>
              </span>
              {a.remembered && (
                <span class="xs muted row gap1">
                  <Icon name="unlock" size="sm" />
                  {t('start.remembered')}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
      {acct && (
        <form
          class="col gap3"
          onSubmit={(e) => {
            e.preventDefault();
            unlock();
          }}
        >
          {acct.remembered ? (
            <Notice kind="info" icon="unlock">
              {t('start.remembered.body')}
            </Notice>
          ) : (
            <Field label={t('acct.passphrase')} error={err ? errText(err) : undefined} id="pp">
              <PasswordInput id="pp" value={pass} onValue={setPass} autoFocus invalid={!!err} onEnter={unlock} />
            </Field>
          )}
          {!acct.remembered && s.mode === 'portable' && <Switch checked={remember} onChange={setRemember} label={<span class="small muted">{t('acct.remember')}</span>} />}
          {acct.remembered && err && <Notice kind="danger">{errText(err)}</Notice>}
          <Button kind="primary" size="lg" block type="submit" busy={busy} disabled={!acct.remembered && !pass}>
            {t('start.unlock')}
          </Button>
          <div class="row" style={{ justifyContent: 'space-between' }}>
            <Button kind="ghost" size="sm" icon="key" onClick={p.onRecover}>
              {t('start.forgot')}
            </Button>
          </div>
        </form>
      )}
      <hr class="divider" />
      <div class="row gap2">
        <Button icon="sparkles" class="grow" onClick={p.onCreate}>
          {t('start.create')}
        </Button>
        <Button icon="cloud" class="grow" onClick={p.onSignIn}>
          {t('start.signin')}
        </Button>
      </div>
    </div>
  );
}

function CreateAccount(p: { onBack: () => void }) {
  const s = useStore(app, (x) => x.s)!;
  const [name, setName] = useState('');
  const [pass, setPass] = useState('');
  const [pass2, setPass2] = useState('');
  const [remember, setRemember] = useState(false);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const short = pass.length > 0 && [...pass].length < 8;
  const mismatch = pass2.length > 0 && pass !== pass2;
  const ok = name.trim() && [...pass].length >= 8 && pass === pass2;
  const create = async () => {
    if (!ok) return;
    setBusy(true);
    setErr('');
    const r = await run(() => post<{ recoveryKey: string; account: AccountView }>('/api/account/create', { name: name.trim(), passphrase: pass, remember }), setErr);
    setBusy(false);
    if (r) {
      startHold.set({ recoveryKey: r.recoveryKey });
      refresh();
    }
  };
  return (
    <form
      class="start-card"
      onSubmit={(e) => {
        e.preventDefault();
        create();
      }}
    >
      <div class="steps" aria-hidden="true">
        <span class="on" />
        <span />
      </div>
      <div class="col gap2">
        <h1>{t('start.create')}</h1>
        <p class="lead">{t('acct.create.lead')}</p>
      </div>
      <Field label={t('acct.name')} hint={t('acct.name.hint')} id="an">
        <Input id="an" icon="user" value={name} onValue={setName} maxLength={48} autoFocus autocomplete="nickname" />
      </Field>
      <Field label={t('acct.passphrase')} hint={t('acct.passphrase.hint')} error={short ? t('acct.passphrase.short') : undefined} id="p1">
        <PasswordInput id="p1" value={pass} onValue={setPass} autocomplete="new-password" invalid={short} />
      </Field>
      <Field label={t('acct.passphrase.confirm')} error={mismatch ? t('acct.passphrase.mismatch') : undefined} id="p2">
        <PasswordInput id="p2" value={pass2} onValue={setPass2} autocomplete="new-password" invalid={mismatch} onEnter={create} />
      </Field>
      {s.mode === 'portable' && (
        <Switch
          checked={remember}
          onChange={setRemember}
          label={
            <span class="col" style={{ gap: 0 }}>
              <span class="small">{t('acct.remember')}</span>
              <span class="xs faint">{t('acct.remember.hint')}</span>
            </span>
          }
        />
      )}
      {err && <Notice kind="danger">{errText(err)}</Notice>}
      <div class="row">
        <Button kind="ghost" icon="back" onClick={p.onBack}>
          {t('ui.back')}
        </Button>
        <span class="spacer" />
        <Button kind="primary" type="submit" busy={busy} disabled={!ok} trail="next">
          {t('acct.create.continue')}
        </Button>
      </div>
    </form>
  );
}

function RecoveryKey(p: { value: string }) {
  const [saved, setSaved] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(p.value);
      toast('success', t('acct.rk.copied'));
    } catch {
      toast('warning', t('acct.rk.copy_failed'));
    }
  };
  const download = () => {
    const blob = new Blob([t('acct.rk.file', { key: p.value })], { type: 'text/plain' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'MNE-Lab-recovery-key.txt';
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 2000);
  };
  return (
    <div class="start-card">
      <div class="steps" aria-hidden="true">
        <span class="on" />
        <span class="on" />
      </div>
      <div class="col gap2">
        <h1>{t('acct.rk.title')}</h1>
        <p class="lead">{t('acct.rk.lead')}</p>
      </div>
      <div class="recovery-key" aria-label={t('acct.rk.title')}>
        {p.value}
      </div>
      <div class="row gap2">
        <Button icon="copy" class="grow" onClick={copy}>
          {t('ui.copy')}
        </Button>
        <Button icon="download" class="grow" onClick={download}>
          {t('acct.rk.save_file')}
        </Button>
      </div>
      <Notice kind="warning">{t('acct.rk.warning')}</Notice>
      <Check checked={saved} onChange={setSaved} label={<span class="small">{t('acct.rk.confirm')}</span>} />
      <Button kind="primary" size="lg" block disabled={!saved} trail="next" onClick={() => startHold.set({ recoveryKey: null })}>
        {t('ui.continue')}
      </Button>
    </div>
  );
}

function RecoverAccount(p: { id: string; onBack: () => void }) {
  const [rk, setRk] = useState('');
  const [pass, setPass] = useState('');
  const [pass2, setPass2] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const ok = rk.trim().length > 10 && [...pass].length >= 8 && pass === pass2;
  const go = async () => {
    setBusy(true);
    setErr('');
    const v = await run(() => post<AccountView>('/api/account/recover', { id: p.id, recoveryKey: rk.trim(), passphrase: pass }), setErr);
    setBusy(false);
    if (v) {
      toast('success', t('acct.recovered'));
      refresh();
    }
  };
  return (
    <form
      class="start-card"
      onSubmit={(e) => {
        e.preventDefault();
        if (ok) go();
      }}
    >
      <div class="col gap2">
        <h1>{t('acct.recover.title')}</h1>
        <p class="lead">{t('acct.recover.lead')}</p>
      </div>
      <Field label={t('acct.rk.label')} id="rk">
        <Input id="rk" class="mono" icon="key" value={rk} onValue={setRk} placeholder="XXXX-XXXX-…" autoFocus autocomplete="off" />
      </Field>
      <Field label={t('acct.passphrase.new')} error={pass && [...pass].length < 8 ? t('acct.passphrase.short') : undefined} id="np">
        <PasswordInput id="np" value={pass} onValue={setPass} autocomplete="new-password" />
      </Field>
      <Field label={t('acct.passphrase.confirm')} error={pass2 && pass !== pass2 ? t('acct.passphrase.mismatch') : undefined} id="np2">
        <PasswordInput id="np2" value={pass2} onValue={setPass2} autocomplete="new-password" />
      </Field>
      {err && <Notice kind="danger">{errText(err)}</Notice>}
      <div class="row">
        <Button kind="ghost" icon="back" onClick={p.onBack}>
          {t('ui.back')}
        </Button>
        <span class="spacer" />
        <Button kind="primary" type="submit" busy={busy} disabled={!ok}>
          {t('acct.recover.go')}
        </Button>
      </div>
    </form>
  );
}

type Remote = { id: string; created: string };

function SignIn(p: { onBack: () => void }) {
  const s = useStore(app, (x) => x.s)!;
  const [step, setStep] = useState<'provider' | 'account'>('provider');
  const [prov, setProv] = useState('');
  const [found, setFound] = useState<Remote[]>([]);
  const [pick, setPick] = useState('');
  const [pass, setPass] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const [folderFor, setFolderFor] = useState<string | null>(null);

  const start = async (id: string, transport: string, folder = '') => {
    setErr('');
    setProv(id);
    const r = await run(() => post<Remote[] | null>('/api/signin/start', { provider: id, transport, folder }), setErr);
    if (r !== undefined) {
      setFound(r || []);
      setPick(r?.[0]?.id || '');
      setStep('account');
    }
  };
  const finish = async () => {
    setBusy(true);
    setErr('');
    const v = await run(() => post<AccountView>('/api/signin/finish', { id: pick, passphrase: pass }), setErr);
    setBusy(false);
    if (v) {
      toast('success', t('signin.done'));
      refresh();
    }
  };

  if (step === 'account')
    return (
      <form
        class="start-card"
        onSubmit={(e) => {
          e.preventDefault();
          if (pick && pass) finish();
        }}
      >
        <div class="row gap3">
          <ProviderLogo id={prov} />
          <div class="col gap1" style={{ gap: 0 }}>
            <h2>{t('signin.accounts')}</h2>
            <span class="small muted">{providerName(prov)}</span>
          </div>
        </div>
        {found.length === 0 ? (
          <Notice kind="info">{t('signin.none')}</Notice>
        ) : (
          <>
            <div class="col gap2">
              {found.map((a) => (
                <button type="button" class={`account-row ${a.id === pick ? 'on' : ''}`} onClick={() => setPick(a.id)}>
                  <Icon name="user" />
                  <span class="grow col" style={{ gap: 0 }}>
                    <b class="small">{t('start.account_created', { date: fmtDate(a.created) })}</b>
                    <span class="xs faint mono">{a.id.slice(0, 8)}</span>
                  </span>
                  {a.id === pick && <Icon name="check" />}
                </button>
              ))}
            </div>
            <Field label={t('acct.passphrase')} hint={t('signin.passphrase.hint')} id="sp">
              <PasswordInput id="sp" value={pass} onValue={setPass} autoFocus onEnter={() => pick && pass && finish()} />
            </Field>
          </>
        )}
        {err && <Notice kind="danger">{errText(err)}</Notice>}
        <div class="row">
          <Button kind="ghost" icon="back" onClick={() => (setStep('provider'), setErr(''))}>
            {t('ui.back')}
          </Button>
          <span class="spacer" />
          {found.length > 0 && (
            <Button kind="primary" type="submit" busy={busy} disabled={!pick || !pass}>
              {t('start.signin')}
            </Button>
          )}
        </div>
      </form>
    );

  return (
    <div class="start-card wide">
      <div class="col gap2">
        <h1>{t('start.signin')}</h1>
        <p class="lead">{t('signin.lead')}</p>
      </div>
      <ProviderCards options={s.providers} selected={prov} onConnect={(id, tr) => start(id, tr)} />
      {s.providers.every((o) => !o.configured) && <Notice kind="info">{t('prov.none_available')}</Notice>}
      {err && <Notice kind="danger">{errText(err)}</Notice>}
      <div class="row">
        <Button kind="ghost" icon="back" onClick={p.onBack}>
          {t('ui.back')}
        </Button>
        <span class="spacer" />
        <MenuFolder onPick={(id) => setFolderFor(id)} />
      </div>
      <p class="xs faint">{t('signin.privacy')}</p>
      {folderFor && (
        <DirPicker
          title={t('prov.pick_folder', { name: providerName(folderFor) })}
          onClose={() => setFolderFor(null)}
          onPick={(path) => {
            const id = folderFor;
            setFolderFor(null);
            start(id, 'folder', path);
          }}
        />
      )}
    </div>
  );
}

/** A provider whose desktop client keeps its folder somewhere unusual. */
function MenuFolder(p: { onPick: (id: string) => void }) {
  return (
    <MenuButton
      kind="ghost"
      size="sm"
      icon="folderOpen"
      align="end"
      items={['google', 'icloud', 'onedrive'].map((id) => ({ label: providerName(id), icon: 'folder' as const, run: () => p.onPick(id) }))}
    >
      {t('prov.other_folder')}
    </MenuButton>
  );
}
