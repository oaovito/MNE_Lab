import { api, connectEvents, get, on } from './api';
import { setDecimal } from './format';
import { errText, setLang } from './i18n';
import { createStore } from './store';
import type { StateView } from './types';

export type Toast = { id: number; kind: 'info' | 'success' | 'warning' | 'error'; text: string; action?: { label: string; run: () => void }; sticky?: boolean };

type AppState = {
  s: StateView | null;
  online: boolean; // connection to the local core
  libraryRev: number; // bumps when files, graphs or cycles change
  toasts: Toast[];
  exitSteps: { step: string; state: string; detail?: string }[];
  palette: boolean;
  panel: string | null; // overlays opened from anywhere: mobile, help, whatsnew, export…
  panelArg: any;
};

export const app = createStore<AppState>({ s: null, online: true, libraryRev: 0, toasts: [], exitSteps: [], palette: false, panel: null, panelArg: null });

let toastId = 0;
export function toast(kind: Toast['kind'], text: string, action?: Toast['action'], sticky = false) {
  const id = ++toastId;
  app.set((st) => ({ toasts: [...st.toasts.slice(-3), { id, kind, text, action, sticky }] }));
  if (!sticky) setTimeout(() => dismiss(id), kind === 'error' ? 7000 : 4200);
  return id;
}
export const dismiss = (id: number) => app.set((st) => ({ toasts: st.toasts.filter((t) => t.id !== id) }));

export const openPanel = (panel: string, arg: any = null) => app.set({ panel, panelArg: arg });
export const closePanel = () => app.set({ panel: null, panelArg: null });

function applyState(s: StateView) {
  setLang(s.lang);
  setDecimal(s.decimal);
  const theme = s.profile?.settings.theme || s.settings.theme || 'system';
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.classList.toggle('turbo', !!s.settings.turbo);
  root.dataset.mode = s.mode;
  const dark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', dark ? '#0B0F14' : '#F4F6F9');
}

let pending: Promise<void> | null = null;
export function refresh(): Promise<void> {
  if (pending) return pending;
  pending = get<StateView>('/api/state')
    .then((s) => {
      applyState(s);
      app.set({ s, online: true });
    })
    .catch(() => app.set({ online: false }))
    .finally(() => (pending = null));
  return pending;
}

let timer = 0;
const soon = () => {
  clearTimeout(timer);
  timer = window.setTimeout(refresh, 60);
};

export function start() {
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    const s = app.get().s;
    if (s) applyState(s);
  });
  for (const ev of ['state', 'sync', 'update', 'mobile', 'present', 'activity']) on(ev, soon);
  on('library', () => app.set((st) => ({ libraryRev: st.libraryRev + 1 })));
  on('exit', (step) => app.set((st) => ({ exitSteps: [...st.exitSteps, step] })));
  on('connection', (ok) => {
    app.set({ online: ok });
    if (ok) refresh();
  });
  connectEvents();
  return refresh();
}

/** run wraps an action: shows errors as toasts and returns undefined on
 * failure (null when the action succeeded without a body). */
export async function run<T>(fn: () => Promise<T>, onError?: (code: string) => void): Promise<T | null | undefined> {
  try {
    const v = await fn();
    return v === undefined ? null : v;
  } catch (e: any) {
    const code = e?.code || 'app.internal_error';
    if (onError) onError(code);
    else toast('error', errText(code));
    return undefined;
  }
}

export const S = () => app.get().s!;
export { api };
