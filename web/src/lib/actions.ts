// Actions shared by screens, the command palette and keyboard shortcuts.
import { get, post, put } from './api';
import { t } from './i18n';
import { navigate } from './route';
import { app, openPanel, refresh, run, S, toast } from './state';
import type { ProfileSettings } from './types';

/** saveProfileSettings merges a change into the open profile's settings. */
export async function saveProfileSettings(patch: Partial<ProfileSettings>) {
  const cur = S().profile?.settings || ({} as ProfileSettings);
  const next = { ...cur, ...patch };
  await put('/api/profile/settings', next);
  await refresh();
}

export async function saveAppSettings(patch: { language?: string; theme?: string; onboarded?: boolean }) {
  await put('/api/settings', patch);
  await refresh();
}

/** Language and theme follow the open profile, or the application before one is open. */
export async function setLanguage(lang: string) {
  if (S().profile) await saveProfileSettings({ language: lang });
  else await saveAppSettings({ language: lang });
}
export async function setTheme(theme: string) {
  if (S().profile) await saveProfileSettings({ theme });
  else await saveAppSettings({ theme });
}

// ---- first-use introductions (stored only in the person's own data) ----

export const seen = (key: string) => !!S()?.profile?.settings.onboarding?.[key];
export async function markSeen(key: string) {
  if (!S()?.profile || seen(key)) return;
  const ob = { ...(S().profile!.settings.onboarding || {}), [key]: true };
  await saveProfileSettings({ onboarding: ob }).catch(() => undefined);
}

// ---- application ----

export async function setTurbo(on: boolean) {
  const route = location.pathname + location.search;
  app.set({ panel: 'restarting', panelArg: on ? 'turbo_on' : 'turbo_off' });
  const ok = await run(() => post('/api/app/turbo', { on, route }));
  if (ok === undefined) app.set({ panel: null });
}

/** toggleTurbo explains Turbo the first time, then switches directly. */
export function toggleTurbo() {
  if (seen('turbo')) setTurbo(!S().settings.turbo);
  else openPanel('turbo');
}

/** exit explains Save, Clean & Exit the first time it is used. */
export function exitApp() {
  if (seen('exit')) startExit();
  else openPanel('exit-confirm');
}

export async function startExit() {
  app.set({ exitReturnRoute: location.pathname + location.search, exitSteps: [] });
  navigate('/exit', true);
  await run(() => post('/api/app/exit'));
}

export async function saveNow() {
  const r = await run(() => post('/api/app/save'));
  if (r !== undefined) toast('success', t('home.saved'));
}

export async function syncNow() {
  const r = await run(() => post('/api/sync'));
  if (r !== undefined) refresh();
}

export async function changeAccount() {
  app.set({ panel: 'switching' });
  const ok = await run(() => post('/api/account/change'));
  app.set({ panel: null });
  if (ok !== undefined) {
    await refresh();
    navigate('/', true);
  }
}

export async function switchProfile() {
  const ok = await run(() => post('/api/profile/close'));
  if (ok !== undefined) {
    await refresh();
    navigate('/', true);
  }
}

export async function openExternal(url: string) {
  await run(() => post('/api/app/open-external', { url }));
}

export async function checkUpdates() {
  const r = await run(() => post('/api/update/check'));
  if (r !== undefined) refresh();
}

export const ping = () => get('/api/state');
