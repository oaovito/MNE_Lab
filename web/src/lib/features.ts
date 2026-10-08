// The feature registry: every capability with where to find it and why it
// may be unavailable right now. Help, the command palette and contextual
// guidance all read from here, so a feature is described once.
import type { IconName } from '../ui/icons';
import { exitApp, saveNow, syncNow, toggleTurbo } from './actions';
import { navigate } from './route';
import { app, openPanel } from './state';
import type { StateView } from './types';

export type Feature = {
  id: string;
  group: string;
  icon: IconName;
  /** how to reach it; absent for features that are always at work */
  open?: () => void;
  /** an i18n key explaining why it is unavailable now, or null */
  avail?: (s: StateView) => string | null;
  /** extra words people may search for (not shown) */
  words?: string;
  isNew?: boolean;
};

export const GROUPS = ['account', 'storage', 'cloud', 'backup', 'portable', 'temporary', 'mobile', 'performance', 'updates', 'locked', 'privacy', 'security', 'ls', 'graphs', 'cycles', 'export'] as const;

const settings = (tab: string) => () => openPanel('settings', tab);
const cloudProfile = (s: StateView) => (!s.profile ? 'avail.needs_profile' : s.profile.storageMode === 'usb_only' ? 'avail.needs_cloud' : null);
const portableOnly = (s: StateView) => (s.mode === 'portable' ? null : 'avail.portable_only');
const needsProfile = (s: StateView) => (s.profile ? null : 'avail.needs_profile');

export const FEATURES: Feature[] = [
  { id: 'account', group: 'account', icon: 'key', open: settings('account'), words: 'passphrase sign in login' },
  { id: 'profiles', group: 'account', icon: 'users', open: () => navigate('/'), words: 'profile switch five' },
  { id: 'avatar', group: 'account', icon: 'camera', open: settings('profile'), avail: needsProfile, words: 'photo gif picture' },
  { id: 'profile_password', group: 'account', icon: 'lock', open: settings('profile'), avail: needsProfile },
  { id: 'change_account', group: 'account', icon: 'logout', words: 'logout sign out' },
  { id: 'mystuff', group: 'account', icon: 'archive', open: () => openPanel('mystuff'), avail: needsProfile, words: 'my stuff history' },

  { id: 'storage_modes', group: 'storage', icon: 'drive', open: settings('storage'), words: 'usb cloud only' },
  { id: 'save_state', group: 'storage', icon: 'save', open: () => saveNow(), avail: needsProfile, words: 'saved status' },

  { id: 'providers', group: 'cloud', icon: 'cloud', open: settings('storage'), words: 'google drive one icloud onedrive microsoft' },
  { id: 'sync', group: 'cloud', icon: 'refresh', open: () => syncNow(), avail: cloudProfile, words: 'synchronize queue pending offline' },
  { id: 'conflicts', group: 'cloud', icon: 'layers', open: settings('storage'), avail: cloudProfile, words: 'conflict versions' },

  { id: 'backups', group: 'backup', icon: 'history', open: settings('backups'), avail: needsProfile, words: 'backup snapshot restore' },
  { id: 'recovery', group: 'backup', icon: 'lifebuoy', words: 'crash recover backup' },
  { id: 'exit', group: 'backup', icon: 'power', open: () => exitApp(), avail: needsProfile, words: 'save clean exit close quit' },

  { id: 'portable', group: 'portable', icon: 'usb', avail: portableOnly, words: 'usb pen drive' },
  { id: 'offline_unlock', group: 'portable', icon: 'wifiOff', open: settings('account'), avail: portableOnly, words: 'remember offline' },

  { id: 'temporary', group: 'temporary', icon: 'laptop', words: 'temporary machine computer' },
  { id: 'auto_clean', group: 'temporary', icon: 'sparkles', words: 'clean cleanup traces' },

  { id: 'mobile', group: 'mobile', icon: 'qr', open: () => openPanel('mobile'), avail: needsProfile, words: 'mobile phone qr pairing' },
  { id: 'controller', group: 'mobile', icon: 'phone', open: () => openPanel('mobile'), avail: needsProfile, words: 'mobile phone remote controller slides' },
  { id: 'viewer', group: 'mobile', icon: 'devices', open: () => openPanel('mobile'), avail: needsProfile, words: 'mobile viewer phone graphs' },

  { id: 'turbo', group: 'performance', icon: 'gauge', open: () => toggleTurbo(), avail: needsProfile, words: 'fast performance animations' },

  { id: 'auto_update', group: 'updates', icon: 'package', open: settings('updates'), words: 'update stable' },
  { id: 'whatsnew', group: 'updates', icon: 'gift', open: () => openPanel('whatsnew'), words: 'news changelog' },

  { id: 'lockedbuild', group: 'locked', icon: 'lock', open: settings('updates'), avail: portableOnly, words: 'locked pin version' },
  { id: 'builds', group: 'locked', icon: 'list', open: settings('updates'), avail: (s) => (s.mode !== 'portable' ? 'avail.portable_only' : !s.update.locked ? 'avail.needs_locked' : null), words: 'downgrade build select' },

  { id: 'privacy', group: 'privacy', icon: 'shield', open: settings('privacy'), words: 'telemetry analytics data' },
  { id: 'encryption', group: 'security', icon: 'shieldOk', open: settings('privacy'), words: 'encrypted keys security' },
  { id: 'recovery_key', group: 'security', icon: 'key', open: settings('account'), words: 'recovery key lost passphrase' },

  { id: 'ls', group: 'ls', icon: 'flask', open: () => navigate('/ls/files'), avail: needsProfile, words: 'dls nanobrook brookhaven light scattering' },
  { id: 'import', group: 'ls', icon: 'fileUp', open: () => navigate('/ls/files'), avail: needsProfile, words: 'import file upload drag' },
  { id: 'file_library', group: 'ls', icon: 'files', open: () => navigate('/ls/files'), avail: needsProfile, words: 'files library search filter' },
  { id: 'original', group: 'ls', icon: 'file', open: () => navigate('/ls/files'), avail: needsProfile, words: 'original raw source checksum' },
  { id: 'provenance', group: 'ls', icon: 'workflow', avail: needsProfile, words: 'provenance traceability hash' },

  { id: 'dls_graph', group: 'graphs', icon: 'chart', open: () => navigate('/ls/files'), avail: needsProfile, words: 'graph distribution intensity diameter' },
  { id: 'compare', group: 'graphs', icon: 'layers', open: () => navigate('/ls/files'), avail: needsProfile, words: 'compare overlay multiple' },
  { id: 'graph_library', group: 'graphs', icon: 'grid', open: () => navigate('/ls/graphs'), avail: needsProfile, words: 'saved graphs dls lightscattering' },
  { id: 'present', group: 'graphs', icon: 'present', open: () => navigate('/ls/graphs'), avail: needsProfile, words: 'presentation slides fullscreen' },

  { id: 'cycles', group: 'cycles', icon: 'cycle', open: () => openPanel('cycle-wizard', { measurements: [] }), avail: needsProfile, words: 'cycle stability time dls lightscattering' },
  { id: 'association', group: 'cycles', icon: 'listChecks', open: () => navigate('/ls/cycles'), avail: needsProfile, words: 'assign point confirm' },
  { id: 'cycle_stats', group: 'cycles', icon: 'sigma', open: () => navigate('/ls/cycles'), avail: needsProfile, words: 'mean standard deviation replicates' },

  { id: 'smart_export', group: 'export', icon: 'download', open: () => navigate('/ls/graphs'), avail: needsProfile, words: 'export png svg pdf tiff csv xlsx' },
  { id: 'package', group: 'export', icon: 'package', open: () => navigate('/ls/cycles'), avail: needsProfile, words: 'research package zip manifest' },
];

export const feature = (id: string) => FEATURES.find((f) => f.id === id);

/** Help opens on a feature (or a guide) from anywhere. */
export const openHelp = (section?: string) => app.set({ panel: 'help', panelArg: section || null });
