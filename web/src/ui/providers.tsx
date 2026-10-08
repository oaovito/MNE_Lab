// Cloud provider presentation: official logos, names and the selection
// cards used by Sign In, profile creation and Settings.
import { useState } from 'preact/hooks';
import google from '../assets/providers/google.svg';
import icloud from '../assets/providers/icloud.svg';
import onedrive from '../assets/providers/onedrive.svg';
import { t } from '../lib/i18n';
import type { ProviderOption } from '../lib/types';
import { Icon } from './icons';
import { Badge, Button, Spinner } from './kit';

const logos: Record<string, string> = { google, icloud, onedrive };

// Brand names stay as their owners write them, in every language.
export const providerName = (id?: string) =>
  ({ google: 'Google Drive / Google One', icloud: 'iCloud', onedrive: 'Microsoft OneDrive' })[id || ''] || id || '';

export function ProviderLogo(p: { id?: string; size?: number }) {
  const svg = p.id ? logos[p.id] : undefined;
  const style = p.size ? { width: p.size, height: p.size, borderRadius: p.size / 3.6 } : undefined;
  if (!svg)
    return (
      <span class="logo fallback" style={style}>
        <Icon name="cloud" />
      </span>
    );
  return <span class="logo" style={style} dangerouslySetInnerHTML={{ __html: svg }} />;
}

/** How a provider can be used in this build, in the person's words. */
export function availability(o: ProviderOption): { key: string; kind?: 'success' | 'warning' | 'info' } {
  if (o.api && o.hasFolder) return { key: 'prov.state.both', kind: 'success' };
  if (o.api) return { key: 'prov.state.api', kind: 'success' };
  if (o.hasFolder) return { key: 'prov.state.folder', kind: 'info' };
  return { key: 'prov.state.unavailable', kind: 'warning' };
}

export type Connected = { provider: string; transport: string; account?: { displayName?: string; maskedEmail?: string }; connection?: string };

/**
 * ProviderCards lets the person pick and connect one of the three official
 * providers. onConnect runs the real connection (authorization in the
 * browser, or the desktop client's folder) and must resolve when the
 * provider was tested for reading and writing.
 */
export function ProviderCards(p: {
  options: ProviderOption[];
  selected?: string;
  connected?: Connected | null;
  onConnect: (id: string, transport: 'api' | 'folder') => Promise<void>;
  disabled?: boolean;
}) {
  const [busy, setBusy] = useState<string | null>(null);
  const connect = async (o: ProviderOption, transport: 'api' | 'folder') => {
    setBusy(o.id + transport);
    try {
      await p.onConnect(o.id, transport);
    } finally {
      setBusy(null);
    }
  };
  return (
    <div class="providers" role="radiogroup" aria-label={t('prov.choose')}>
      {p.options.map((o) => {
        const av = availability(o);
        const on = p.selected === o.id;
        const done = p.connected?.provider === o.id;
        return (
          <div class={`provider ${on ? 'on' : ''}`} role="radio" aria-checked={on} aria-disabled={!o.configured || p.disabled}>
            {done && (
              <span class="tick">
                <Icon name="check" />
              </span>
            )}
            <ProviderLogo id={o.id} />
            <div class="col gap1" style={{ minWidth: 0 }}>
              <h4>{providerName(o.id)}</h4>
              <p>{t('prov.desc.' + o.id)}</p>
            </div>
            <div class="state col gap1" style={{ width: '100%' }}>
              {done ? (
                <div class="col gap1">
                  <Badge kind="success" icon="ok">
                    {t('prov.connected')}
                  </Badge>
                  {(p.connected?.account?.displayName || p.connected?.account?.maskedEmail) && (
                    <span class="xs muted ellipsis">{[p.connected.account.displayName, p.connected.account.maskedEmail].filter(Boolean).join(' · ')}</span>
                  )}
                  {p.connected?.transport === 'folder' && <span class="xs faint">{t('prov.via_folder')}</span>}
                </div>
              ) : busy?.startsWith(o.id) ? (
                <Spinner label={t(busy.endsWith('api') ? 'prov.authorizing' : 'prov.testing')} />
              ) : !o.configured ? (
                <span class="xs faint">{t(av.key)}</span>
              ) : (
                <div class="row wrap gap1">
                  {o.api && (
                    <Button size="sm" kind={on || !p.selected ? 'primary' : 'default'} disabled={p.disabled || !!busy} onClick={() => connect(o, 'api')}>
                      {t('prov.connect')}
                    </Button>
                  )}
                  {o.hasFolder && (
                    <Button size="sm" kind={o.api ? 'ghost' : on || !p.selected ? 'primary' : 'default'} icon="folder" disabled={p.disabled || !!busy} onClick={() => connect(o, 'folder')} tip={o.folder}>
                      {t(o.api ? 'prov.use_folder_short' : 'prov.use_folder')}
                    </Button>
                  )}
                </div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
