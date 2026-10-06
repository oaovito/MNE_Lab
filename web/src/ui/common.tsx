// Small pieces shared by several screens.
import { setLanguage, setTheme } from '../lib/actions';
import { fmtRel } from '../lib/format';
import { errText, LANG_NAMES, LANGS, t, type Lang } from '../lib/i18n';
import { app, run } from '../lib/state';
import { useStore } from '../lib/store';
import type { StateView, SyncStatus } from '../lib/types';
import { useState } from 'preact/hooks';
import { Icon, type IconName } from './icons';
import { Badge, MenuButton } from './kit';

export function LangSelect(p: { compact?: boolean }) {
  const s = useStore(app, (x) => x.s)!;
  const cur = s.lang as Lang;
  return (
    <MenuButton
      kind="ghost"
      size="sm"
      icon="globe"
      tip={t('set.language')}
      align="end"
      items={() => [
        { heading: t('set.language') },
        ...LANGS.map((l) => ({ label: LANG_NAMES[l], checked: l === cur, run: () => run(() => setLanguage(l)) })),
      ]}
    >
      {p.compact ? undefined : LANG_NAMES[cur]}
    </MenuButton>
  );
}

export function ThemeToggle() {
  const s = useStore(app, (x) => x.s)!;
  const theme = s.profile?.settings.theme || s.settings.theme || 'system';
  const icon: IconName = theme === 'light' ? 'sun' : theme === 'dark' ? 'moon' : 'monitor';
  return (
    <MenuButton
      kind="ghost"
      size="sm"
      icon={icon}
      tip={t('set.theme')}
      align="end"
      items={() =>
        (['system', 'light', 'dark'] as const).map((th) => ({
          label: t('set.theme.' + th),
          icon: (th === 'light' ? 'sun' : th === 'dark' ? 'moon' : 'monitor') as IconName,
          checked: th === theme,
          run: () => run(() => setTheme(th)),
        }))
      }
    />
  );
}

export function ModeBadge(p: { mode: StateView['mode'] }) {
  return p.mode === 'temporary' ? (
    <Badge kind="info" icon="laptop" tip={t('mode.temporary.tip')}>
      {t('mode.temporary')}
    </Badge>
  ) : (
    <Badge kind="accent" icon="usb" tip={t('mode.portable.tip')}>
      {t('mode.portable')}
    </Badge>
  );
}

export const storageIcons = (m: string): IconName[] => (m === 'usb_cloud' ? ['usb', 'cloud'] : m === 'usb_only' ? ['usb'] : ['cloud']);

export function StorageLabel(p: { mode: string }) {
  return (
    <span class="row gap1">
      {storageIcons(p.mode).map((i) => (
        <Icon name={i} size="sm" />
      ))}
      <span>{t('storage.' + p.mode)}</span>
    </span>
  );
}

/** syncView turns the synchronization state into a short sentence and a tone. */
export function syncView(st: SyncStatus | undefined, busy: boolean): { text: string; tone: 'ok' | 'busy' | 'warn' | 'err'; detail: string } {
  if (!st) return { text: t('status.no_profile'), tone: 'warn', detail: '' };
  const last = st.lastSync ? t('status.last_sync', { when: fmtRel(st.lastSync) }) : '';
  switch (st.state) {
    case 'local':
      return { text: t('status.local'), tone: 'ok', detail: t('status.local.detail') };
    case 'synced':
      return { text: t('status.synced'), tone: 'ok', detail: last };
    case 'syncing':
      return { text: t('status.syncing'), tone: 'busy', detail: last };
    case 'pending':
      return { text: busy ? t('status.syncing') : t('status.pending', { n: st.pending }), tone: busy ? 'busy' : 'warn', detail: last };
    case 'offline':
      return { text: t('status.offline'), tone: 'warn', detail: st.pending ? t('status.pending', { n: st.pending }) : t('status.offline.detail') };
    case 'reconnect':
      return { text: t('status.reconnect'), tone: 'err', detail: t('status.reconnect.detail') };
    case 'not_connected':
      return { text: t('status.not_connected'), tone: 'warn', detail: t('status.not_connected.detail') };
    case 'conflict':
      return { text: t('status.conflict', { n: st.conflicts }), tone: 'err', detail: t('status.conflict.detail') };
    case 'error':
      return { text: t('status.error'), tone: 'err', detail: st.lastError ? errText(st.lastError) : '' };
  }
  return { text: st.state, tone: 'warn', detail: '' };
}

export const activityLabel = (kind: string) => t('activity.' + kind);

/** Thumb shows a saved graph's preview, or a quiet placeholder when it cannot be drawn. */
export function Thumb(p: { id: string; updated?: string }) {
  const [failed, setFailed] = useState(false);
  if (failed) return <Icon name="chart" class="thumb-fallback" />;
  return <img src={`/api/graphs/${p.id}/thumb.svg?u=${encodeURIComponent(p.updated || '')}`} alt="" loading="lazy" draggable={false} onError={() => setFailed(true)} />;
}
