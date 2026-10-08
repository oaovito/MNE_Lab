// Help: what MNE Lab can do, grouped by context, with how to reach each
// feature and why it may be unavailable; short guides for LIGHTSCATTERING
// and Smart Export; What's New. Introductions can be replayed from here.
import { useEffect, useMemo, useState } from 'preact/hooks';
import { saveProfileSettings } from '../lib/actions';
import { feature, FEATURES, GROUPS, type Feature } from '../lib/features';
import { has, lang, t } from '../lib/i18n';
import { app, closePanel, openPanel, S } from '../lib/state';
import { useStore } from '../lib/store';
import { Icon, type IconName } from '../ui/icons';
import { Badge, Button, Input, Kbd, Modal } from '../ui/kit';

/** Notes of this build, newest first. Each entry lists i18n keys. */
export const NEWS: { version: string; items: { icon: IconName; key: string }[] }[] = [
  {
    version: '0.1.0',
    items: [
      { icon: 'users', key: 'news.0.accounts' },
      { icon: 'cloud', key: 'news.0.storage' },
      { icon: 'usb', key: 'news.0.modes' },
      { icon: 'phone', key: 'news.0.mobile' },
      { icon: 'lock', key: 'news.0.locked' },
      { icon: 'flask', key: 'news.0.ls' },
      { icon: 'cycle', key: 'news.0.cycles' },
      { icon: 'download', key: 'news.0.export' },
    ],
  },
];

/** hasNews is true while the open profile has not seen this build's notes. */
export function hasNews() {
  const s = S();
  if (!s?.profile) return false;
  const cur = NEWS.find((n) => s.version.startsWith(n.version));
  return !!cur && s.profile.settings.whatsNewSeen !== cur.version;
}

const GROUP_ICON: Record<string, IconName> = {
  account: 'users', storage: 'drive', cloud: 'cloud', backup: 'history', portable: 'usb', temporary: 'laptop', mobile: 'phone', performance: 'gauge',
  updates: 'package', locked: 'lock', privacy: 'shield', security: 'shieldOk', ls: 'flask', graphs: 'chart', cycles: 'cycle', export: 'download',
};
const GUIDES: { id: string; icon: IconName; parts: string[] }[] = [
  { id: 'ls', icon: 'microscope', parts: ['files', 'params', 'meaning', 'graphs', 'combine', 'cycles', 'present', 'export'] },
  { id: 'export', icon: 'image', parts: ['kinds', 'vector', 'lossless', 'dpi', 'publication', 'package'] },
];

export function HelpCenter(p: { arg?: string | null }) {
  const start = p.arg && feature(p.arg) ? feature(p.arg)!.group : p.arg || 'start';
  const [sec, setSec] = useState<string>(start);
  const [q, setQ] = useState('');
  const s = useStore(app, (x) => x.s)!;
  const focus = p.arg && feature(p.arg) ? p.arg : null;

  useEffect(() => {
    if (focus) document.getElementById('feat-' + focus)?.scrollIntoView({ block: 'center' });
  }, []);

  const found = useMemo(() => {
    const n = q.trim().toLowerCase();
    if (!n) return null;
    return FEATURES.filter((f) => [t('feat.' + f.id), t('feat.' + f.id + '.what'), t('fgroup.' + f.group), f.words || '', f.id].join(' ').toLowerCase().includes(n));
  }, [q, lang()]);

  return (
    <Modal size="xwide" icon="book" title={t('help.title')} sub={t('help.sub')} onClose={closePanel} bodyClass="flush">
      <div class="help">
        <nav aria-label={t('help.title')}>
          <div style={{ padding: '0 0 var(--s2)' }}>
            <Input size="sm" icon="search" value={q} onValue={setQ} placeholder={t('help.search')} aria-label={t('help.search')} />
          </div>
          <NavBtn on={sec === 'start' && !found} icon="sparkles" label={t('help.start')} onClick={() => (setSec('start'), setQ(''))} />
          <span class="menu-label">{t('help.features')}</span>
          {GROUPS.map((g) => (
            <NavBtn on={sec === g && !found} icon={GROUP_ICON[g]} label={t('fgroup.' + g)} onClick={() => (setSec(g), setQ(''))} />
          ))}
          <span class="menu-label">{t('help.guides')}</span>
          {GUIDES.map((g) => (
            <NavBtn on={sec === 'guide.' + g.id && !found} icon={g.icon} label={t('guide.' + g.id)} onClick={() => (setSec('guide.' + g.id), setQ(''))} />
          ))}
          <NavBtn on={sec === 'keys' && !found} icon="keyboard" label={t('help.keys')} onClick={() => (setSec('keys'), setQ(''))} />
        </nav>
        <div class="content">
          {found ? (
            <>
              <h3>{t('help.results', { n: found.length })}</h3>
              {found.map((f) => (
                <FeatureCard f={f} s={s} />
              ))}
              {!found.length && <p class="muted">{t('help.no_results')}</p>}
            </>
          ) : sec === 'start' ? (
            <Start />
          ) : sec === 'keys' ? (
            <Keys />
          ) : sec.startsWith('guide.') ? (
            <Guide g={GUIDES.find((g) => 'guide.' + g.id === sec)!} />
          ) : (
            <>
              <div class="row gap2">
                <Icon name={GROUP_ICON[sec] || 'info'} />
                <h3>{t('fgroup.' + sec)}</h3>
              </div>
              {FEATURES.filter((f) => f.group === sec).map((f) => (
                <FeatureCard f={f} s={s} focus={f.id === focus} />
              ))}
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}

function NavBtn(p: { on: boolean; icon: IconName; label: string; onClick: () => void }) {
  return (
    <button type="button" class={p.on ? 'on' : ''} onClick={p.onClick} aria-current={p.on ? 'true' : undefined}>
      <Icon name={p.icon} size="sm" />
      <span class="clamp2">{p.label}</span>
    </button>
  );
}

function FeatureCard(p: { f: Feature; s: any; focus?: boolean }) {
  const f = p.f;
  const why = f.avail?.(p.s) || null;
  const req = 'feat.' + f.id + '.req';
  return (
    <div class="feature" id={'feat-' + f.id} style={p.focus ? { borderColor: 'var(--accent)' } : undefined}>
      <Icon name={f.icon} />
      <div class="row gap2">
        <h4 class="grow">{t('feat.' + f.id)}</h4>
        {f.isNew && <span class="new-badge">{t('ui.new')}</span>}
        {why && (
          <Badge kind="warning" icon="info">
            {t(why)}
          </Badge>
        )}
      </div>
      <p>{t('feat.' + f.id + '.what')}</p>
      <div class="col gap1">
        <p class="small muted">{t('feat.' + f.id + '.when')}</p>
        <div class="how">
          <span>
            <b>{t('help.how')}</b> {t('feat.' + f.id + '.how')}
          </span>
          {has(req) && (
            <span>
              <b>{t('help.req')}</b> {t(req)}
            </span>
          )}
        </div>
        {f.open && !why && (
          <div>
            <Button
              size="sm"
              kind="ghost"
              trail="next"
              onClick={() => {
                closePanel();
                f.open!();
              }}
            >
              {t('help.open')}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

function Start() {
  const items: { icon: IconName; k: string }[] = [
    { icon: 'users', k: 'account' },
    { icon: 'drive', k: 'storage' },
    { icon: 'usb', k: 'portable' },
    { icon: 'phone', k: 'mobile' },
    { icon: 'flask', k: 'ls' },
    { icon: 'power', k: 'exit' },
  ];
  return (
    <div class="col gap4">
      <div>
        <h3>{t('help.start.title')}</h3>
        <p class="muted">{t('help.start.body')}</p>
      </div>
      <div class="tour-points">
        {items.map((x) => (
          <div>
            <Icon name={x.icon} size="sm" />
            <span>{t('help.start.' + x.k)}</span>
          </div>
        ))}
      </div>
      <div class="row gap2 wrap">
        <Button icon="play" onClick={() => openPanel('tour')}>
          {t('help.replay_tour')}
        </Button>
        <Button icon="flask" onClick={() => openPanel('ls-intro')}>
          {t('help.replay_ls')}
        </Button>
        <Button icon="gift" onClick={() => openPanel('whatsnew')}>
          {t('menu.whatsnew')}
        </Button>
      </div>
    </div>
  );
}

function Guide(p: { g: (typeof GUIDES)[number] }) {
  return (
    <div class="col gap3">
      <div class="row gap2">
        <Icon name={p.g.icon} />
        <h3>{t('guide.' + p.g.id)}</h3>
      </div>
      <p class="muted">{t('guide.' + p.g.id + '.intro')}</p>
      {p.g.parts.map((k) => (
        <div class="feature" style={{ gridTemplateColumns: '1fr' }}>
          <h4>{t('guide.' + p.g.id + '.' + k)}</h4>
          <p>{t('guide.' + p.g.id + '.' + k + '.d')}</p>
        </div>
      ))}
      {p.g.id === 'ls' && <p class="xs faint">{t('guide.ls.source')}</p>}
    </div>
  );
}

function Keys() {
  const rows: [string, string][] = [
    ['Ctrl K', 'keys.palette'],
    ['F1', 'keys.help'],
    ['Ctrl S', 'keys.save'],
    ['Ctrl I', 'keys.import'],
    ['→ / Space', 'keys.next'],
    ['←', 'keys.prev'],
    ['L', 'keys.legend'],
    ['Esc', 'keys.close'],
  ];
  return (
    <div class="col gap3">
      <h3>{t('help.keys')}</h3>
      <table class="table">
        <tbody>
          {rows.map(([k, d]) => (
            <tr>
              <td style={{ width: 160 }}>
                <Kbd>{k}</Kbd>
              </td>
              <td>{t(d)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function WhatsNew() {
  const s = useStore(app, (x) => x.s)!;
  useEffect(() => {
    const cur = NEWS.find((n) => s.version.startsWith(n.version));
    if (cur && s.profile && s.profile.settings.whatsNewSeen !== cur.version) saveProfileSettings({ whatsNewSeen: cur.version }).catch(() => undefined);
  }, []);
  const locked = s.update.locked && s.update.newer;
  return (
    <Modal icon="gift" title={t('news.title')} sub={t('news.version', { v: s.version })} onClose={closePanel}>
      <div class="col gap3">
        {NEWS.map((n) => (
          <div class="col gap2">
            <span class="section-title">{t('news.in', { v: n.version })}</span>
            {n.items.map((it) => (
              <div class="row gap3" style={{ alignItems: 'flex-start' }}>
                <span class="modal-icon" style={{ width: 32, height: 32 }}>
                  <Icon name={it.icon} size="sm" />
                </span>
                <div class="col" style={{ gap: 2 }}>
                  <b>{t(it.key)}</b>
                  <span class="small muted">{t(it.key + '.d')}</span>
                </div>
              </div>
            ))}
          </div>
        ))}
        {locked && (
          <div class="row gap2 small">
            <Icon name="lock" size="sm" />
            <span class="grow muted">{t('news.locked')}</span>
            <Button size="sm" onClick={() => openPanel('settings', 'updates')}>
              {t('news.see_builds')}
            </Button>
          </div>
        )}
      </div>
    </Modal>
  );
}
