// MNE Lab component kit. Screens are composed only from these parts so the
// whole application keeps one visual language.
import type { ComponentChildren, JSX, Ref } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { t } from '../lib/i18n';
import { app, dismiss } from '../lib/state';
import { createStore, useStore } from '../lib/store';
import { Icon, type IconName } from './icons';
import mark from '../../../assets/brand/mnelab-mark.svg';

type BtnProps = {
  children?: ComponentChildren;
  kind?: 'primary' | 'ghost' | 'danger' | 'default';
  size?: 'sm' | 'lg';
  icon?: IconName;
  trail?: IconName;
  busy?: boolean;
  disabled?: boolean;
  block?: boolean;
  selected?: boolean;
  tip?: string;
  type?: 'button' | 'submit';
  onClick?: (e: MouseEvent) => void;
  class?: string;
  label?: string; // accessible name for icon-only buttons
  btnRef?: Ref<HTMLButtonElement>;
  kbd?: string;
};

export function Button(p: BtnProps) {
  const cls = ['btn', p.kind && p.kind !== 'default' ? p.kind : '', p.size || '', p.block ? 'block' : '', !p.children ? 'icon-only' : '', p.selected ? 'selected' : '', p.class || ''].join(' ');
  return (
    <button ref={p.btnRef} type={p.type || 'button'} class={cls} disabled={p.disabled || p.busy} onClick={p.onClick} data-tip={p.tip} aria-label={p.label || p.tip} aria-busy={p.busy || undefined}>
      {p.busy ? <span class="spin" /> : p.icon && <Icon name={p.icon} size={p.size === 'sm' ? 'sm' : undefined} />}
      {p.children}
      {p.trail && <Icon name={p.trail} size="sm" />}
      {p.kbd && <kbd>{p.kbd}</kbd>}
    </button>
  );
}

export function Field(p: { label?: string; hint?: string; error?: string; children: ComponentChildren; class?: string; id?: string }) {
  return (
    <div class={`field ${p.class || ''}`}>
      {p.label && <label for={p.id}>{p.label}</label>}
      {p.children}
      {p.error ? <div class="error" role="alert">{p.error}</div> : p.hint && <div class="hint">{p.hint}</div>}
    </div>
  );
}

type InputProps = Omit<JSX.InputHTMLAttributes<HTMLInputElement>, 'onInput' | 'value' | 'icon' | 'size'> & {
  value: string;
  onValue: (v: string) => void;
  icon?: IconName;
  invalid?: boolean;
  inputRef?: Ref<HTMLInputElement>;
  size?: 'sm';
};

export function Input({ value, onValue, icon, invalid, inputRef, size, class: cls, ...rest }: InputProps) {
  const el = (
    <input
      ref={inputRef}
      class={`input ${size || ''} ${invalid ? 'invalid' : ''} ${cls || ''}`}
      value={value}
      onInput={(e) => onValue((e.target as HTMLInputElement).value)}
      aria-invalid={invalid || undefined}
      spellcheck={false}
      {...(rest as any)}
    />
  );
  if (!icon) return el;
  return (
    <div class="input-wrap">
      <Icon name={icon} size="sm" />
      {el}
    </div>
  );
}

export function PasswordInput(p: { value: string; onValue: (v: string) => void; placeholder?: string; autoFocus?: boolean; id?: string; invalid?: boolean; autocomplete?: string; onEnter?: () => void }) {
  const [show, setShow] = useState(false);
  return (
    <div class="input-wrap">
      <Icon name="key" size="sm" />
      <input
        id={p.id}
        class={`input has-trail ${p.invalid ? 'invalid' : ''}`}
        type={show ? 'text' : 'password'}
        value={p.value}
        placeholder={p.placeholder}
        autoFocus={p.autoFocus}
        autocomplete={p.autocomplete || 'current-password'}
        spellcheck={false}
        onInput={(e) => p.onValue((e.target as HTMLInputElement).value)}
        onKeyDown={(e) => e.key === 'Enter' && p.onEnter?.()}
      />
      <span class="trail">
        <Button kind="ghost" size="sm" icon={show ? 'eyeOff' : 'eye'} tip={show ? t('ui.hide') : t('ui.show')} onClick={() => setShow(!show)} />
      </span>
    </div>
  );
}

export function Select<T extends string>(p: { value: T; onValue: (v: T) => void; options: { value: T; label: string; disabled?: boolean }[]; size?: 'sm'; id?: string; label?: string }) {
  return (
    <select id={p.id} aria-label={p.label} class={`select ${p.size || ''}`} value={p.value} onChange={(e) => p.onValue((e.target as HTMLSelectElement).value as T)}>
      {p.options.map((o) => (
        <option value={o.value} disabled={o.disabled}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function Switch(p: { checked: boolean; onChange: (v: boolean) => void; label?: ComponentChildren; disabled?: boolean; tip?: string }) {
  return (
    <label class="switch" data-tip={p.tip}>
      <input type="checkbox" role="switch" checked={p.checked} disabled={p.disabled} onChange={(e) => p.onChange((e.target as HTMLInputElement).checked)} />
      <span class="track" />
      {p.label && <span>{p.label}</span>}
    </label>
  );
}

export function Check(p: { checked: boolean; onChange: (v: boolean) => void; label?: ComponentChildren; indeterminate?: boolean; disabled?: boolean; aria?: string }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!p.indeterminate;
  }, [p.indeterminate]);
  return (
    <label class="check" onClick={(e) => e.stopPropagation()}>
      <input ref={ref} type="checkbox" checked={p.checked} disabled={p.disabled} aria-label={p.aria} onChange={(e) => p.onChange((e.target as HTMLInputElement).checked)} />
      {p.label && <span>{p.label}</span>}
    </label>
  );
}

export function Seg<T extends string>(p: { value: T; onValue: (v: T) => void; options: { value: T; label: ComponentChildren; icon?: IconName; tip?: string }[]; size?: 'sm'; label?: string }) {
  return (
    <div class={`seg ${p.size || ''}`} role="radiogroup" aria-label={p.label}>
      {p.options.map((o) => (
        <button type="button" role="radio" aria-checked={o.value === p.value} class={o.value === p.value ? 'on' : ''} onClick={() => p.onValue(o.value)} data-tip={o.tip}>
          {o.icon && <Icon name={o.icon} size="sm" />}
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Badge(p: { kind?: 'accent' | 'success' | 'warning' | 'danger' | 'info'; icon?: IconName; children: ComponentChildren; tip?: string }) {
  return (
    <span class={`badge ${p.kind || ''}`} data-tip={p.tip}>
      {p.icon && <Icon name={p.icon} />}
      {p.children}
    </span>
  );
}

export function Notice(p: { kind?: 'info' | 'warning' | 'danger' | 'success'; icon?: IconName; children: ComponentChildren; action?: ComponentChildren }) {
  const icon = p.icon || (p.kind === 'warning' ? 'warning' : p.kind === 'danger' ? 'alert' : p.kind === 'success' ? 'ok' : 'info');
  return (
    <div class={`notice ${p.kind || ''}`} role={p.kind === 'danger' ? 'alert' : undefined}>
      <Icon name={icon} size="sm" />
      <div class="grow">{p.children}</div>
      {p.action}
    </div>
  );
}

export function Empty(p: { icon: IconName; title: string; body?: ComponentChildren; children?: ComponentChildren }) {
  return (
    <div class="empty enter">
      <div class="art">
        <Icon name={p.icon} />
      </div>
      <h3>{p.title}</h3>
      {p.body && <p>{p.body}</p>}
      {p.children && <div class="row wrap" style={{ justifyContent: 'center', marginTop: 'var(--s2)' }}>{p.children}</div>}
    </div>
  );
}

export function Skeleton(p: { w?: string | number; h?: string | number; r?: string; style?: JSX.CSSProperties }) {
  return <div class="skeleton" aria-hidden="true" style={{ width: p.w ?? '100%', height: p.h ?? 14, borderRadius: p.r, ...p.style }} />;
}

export function Spinner(p: { label?: string }) {
  return (
    <span class="row muted small" role="status">
      <span class="spin" />
      {p.label}
    </span>
  );
}

export function Tabs<T extends string>(p: { value: T; onValue: (v: T) => void; tabs: { value: T; label: string; icon?: IconName; count?: number }[]; class?: string }) {
  return (
    <div class={`tabs ${p.class || ''}`} role="tablist">
      {p.tabs.map((x) => (
        <button role="tab" aria-selected={x.value === p.value} class={`tab ${x.value === p.value ? 'on' : ''}`} onClick={() => p.onValue(x.value)}>
          {x.icon && <Icon name={x.icon} size="sm" />}
          {x.label}
          {x.count !== undefined && <span class="count">{x.count}</span>}
        </button>
      ))}
    </div>
  );
}

// ---- avatar (photo, or the official generic avatar) ----

export function Avatar(p: { id?: string; name?: string; color?: number; photo?: boolean; hash?: string; size?: number; src?: string }) {
  const size = p.size || 32;
  const style = { '--size': `${size}px`, '--c': `var(--pc${(p.color ?? 0) % 8})` } as JSX.CSSProperties;
  const [broken, setBroken] = useState(false);
  const src = p.src || (p.photo && p.id ? `/api/profiles/${p.id}/avatar?h=${p.hash || ''}` : '');
  if (src && !broken) {
    return (
      <span class="avatar" style={style}>
        <img src={src} alt="" onError={() => setBroken(true)} draggable={false} />
      </span>
    );
  }
  // The official generic avatar: the MNE Lab mark on the profile's color.
  return <span class="avatar generic" style={style} aria-hidden="true" dangerouslySetInnerHTML={{ __html: mark }} />;
}

export function BrandMark(p: { size?: number; base?: string }) {
  return <img class="brand-mark" src={(p.base ?? '/') + 'icon.svg'} width={p.size || 28} height={p.size || 28} alt="" draggable={false} />;
}

// ---- overlays ----

export function Portal(p: { children: ComponentChildren }) {
  return createPortal(p.children as any, document.body);
}

const modalStack: (() => void)[] = [];

export function Modal(p: {
  title?: ComponentChildren;
  sub?: ComponentChildren;
  icon?: IconName;
  onClose?: () => void;
  children: ComponentChildren;
  foot?: ComponentChildren;
  size?: 'wide' | 'xwide' | 'narrow';
  labelledBy?: string;
  dismissable?: boolean;
  bodyClass?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const close = () => p.dismissable !== false && p.onClose?.();
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    modalStack.push(close);
    const first = ref.current?.querySelector<HTMLElement>('[autofocus], input, select, textarea, button.primary, button');
    first?.focus();
    const key = (e: KeyboardEvent) => {
      if (modalStack[modalStack.length - 1] !== close) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        close();
      } else if (e.key === 'Tab' && ref.current) {
        const f = ref.current.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"]), a[href]');
        if (!f.length) return;
        const a = f[0],
          b = f[f.length - 1];
        if (e.shiftKey && document.activeElement === a) {
          e.preventDefault();
          b.focus();
        } else if (!e.shiftKey && document.activeElement === b) {
          e.preventDefault();
          a.focus();
        }
      }
    };
    document.addEventListener('keydown', key);
    return () => {
      document.removeEventListener('keydown', key);
      modalStack.splice(modalStack.indexOf(close), 1);
      prev?.focus?.();
    };
  }, []);
  return (
    <Portal>
      <div class="overlay" onMouseDown={(e) => e.target === e.currentTarget && close()}>
        <div ref={ref} class={`modal ${p.size || ''}`} role="dialog" aria-modal="true" aria-label={typeof p.title === 'string' ? p.title : undefined}>
          {(p.title || p.onClose) && (
            <div class="modal-head">
              {p.icon && (
                <span class="modal-icon">
                  <Icon name={p.icon} />
                </span>
              )}
              <div class="grow">
                {p.title && <h2>{p.title}</h2>}
                {p.sub && <div class="sub">{p.sub}</div>}
              </div>
              {p.onClose && p.dismissable !== false && <Button kind="ghost" icon="x" tip={t('ui.close')} onClick={p.onClose} />}
            </div>
          )}
          <div class={`modal-body ${p.bodyClass || ''}`}>{p.children}</div>
          {p.foot && <div class="modal-foot">{p.foot}</div>}
        </div>
      </div>
    </Portal>
  );
}

export function Drawer(p: { title: ComponentChildren; onClose: () => void; children: ComponentChildren; foot?: ComponentChildren; width?: number }) {
  useEffect(() => {
    const k = (e: KeyboardEvent) => e.key === 'Escape' && p.onClose();
    document.addEventListener('keydown', k);
    return () => document.removeEventListener('keydown', k);
  }, []);
  return (
    <Portal>
      <div class="overlay" style={{ background: 'transparent', backdropFilter: 'none' }} onMouseDown={(e) => e.target === e.currentTarget && p.onClose()} />
      <aside class="drawer" role="dialog" aria-modal="true" style={p.width ? { width: `min(${p.width}px, 100vw)` } : undefined}>
        <div class="panel-head">
          <h3 class="grow">{p.title}</h3>
          <Button kind="ghost" icon="x" tip={t('ui.close')} onClick={p.onClose} />
        </div>
        <div class="panel-body" style={{ padding: 'var(--s4)' }}>
          {p.children}
        </div>
        {p.foot && <div class="modal-foot">{p.foot}</div>}
      </aside>
    </Portal>
  );
}

export type MenuEntry =
  | { label: string; icon?: IconName; run: () => void; danger?: boolean; meta?: string; disabled?: boolean; checked?: boolean; badge?: string }
  | { sep: true }
  | { heading: string };

export function Menu(p: { anchor: HTMLElement; items: MenuEntry[]; onClose: () => void; align?: 'start' | 'end'; children?: ComponentChildren }) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ top: -9999, left: -9999 });
  useLayoutEffect(() => {
    const r = p.anchor.getBoundingClientRect();
    const m = ref.current!.getBoundingClientRect();
    let left = p.align === 'end' ? r.right - m.width : r.left;
    let top = r.bottom + 6;
    if (top + m.height > innerHeight - 8) top = Math.max(8, r.top - m.height - 6);
    left = Math.min(Math.max(8, left), innerWidth - m.width - 8);
    setPos({ top, left });
    ref.current!.querySelector<HTMLElement>('.menu-item')?.focus();
  }, []);
  useEffect(() => {
    const down = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && !p.anchor.contains(e.target as Node) && p.onClose();
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') p.onClose();
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const items = [...(ref.current?.querySelectorAll<HTMLElement>('.menu-item:not([disabled])') || [])];
        const i = items.indexOf(document.activeElement as HTMLElement);
        items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus();
      }
    };
    document.addEventListener('mousedown', down);
    document.addEventListener('keydown', key);
    window.addEventListener('blur', p.onClose);
    return () => {
      document.removeEventListener('mousedown', down);
      document.removeEventListener('keydown', key);
      window.removeEventListener('blur', p.onClose);
    };
  }, []);
  return (
    <Portal>
      <div ref={ref} class="menu" role="menu" style={pos}>
        {p.children}
        {p.items.map((it) =>
          'sep' in it ? (
            <div class="menu-sep" />
          ) : 'heading' in it ? (
            <div class="menu-label">{it.heading}</div>
          ) : (
            <button
              role="menuitem"
              class={`menu-item ${it.danger ? 'danger' : ''}`}
              disabled={it.disabled}
              style={it.disabled ? { opacity: 0.45, cursor: 'not-allowed' } : undefined}
              onClick={() => {
                p.onClose();
                it.run();
              }}
            >
              {it.icon && <Icon name={it.icon} size="sm" />}
              <span class="grow ellipsis">{it.label}</span>
              {it.badge && <span class="new-badge">{it.badge}</span>}
              {it.meta && <span class="meta">{it.meta}</span>}
              {it.checked && <Icon name="check" size="sm" class="on" />}
            </button>
          ),
        )}
      </div>
    </Portal>
  );
}

/** MenuButton opens a menu anchored to itself. */
export function MenuButton(p: Omit<BtnProps, 'onClick'> & { items: MenuEntry[] | (() => MenuEntry[]); align?: 'start' | 'end'; header?: ComponentChildren }) {
  const ref = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const { items, align, header, ...b } = p;
  return (
    <>
      <Button {...b} btnRef={ref} onClick={() => setOpen(!open)} />
      {open && ref.current && <Menu anchor={ref.current} items={typeof items === 'function' ? items() : items} align={align} onClose={() => setOpen(false)}>{header}</Menu>}
    </>
  );
}

// ---- tooltips: one shared element for every [data-tip] ----

export function TooltipHost() {
  const [tip, setTip] = useState<{ text: string; x: number; y: number; below: boolean } | null>(null);
  useEffect(() => {
    let timer = 0;
    let cur: HTMLElement | null = null;
    const show = (el: HTMLElement) => {
      const text = el.getAttribute('data-tip');
      if (!text) return;
      const r = el.getBoundingClientRect();
      const below = r.top < 60;
      setTip({ text, x: r.left + r.width / 2, y: below ? r.bottom + 8 : r.top - 8, below });
    };
    const over = (e: Event) => {
      const el = (e.target as HTMLElement).closest?.('[data-tip]') as HTMLElement | null;
      if (el === cur) return;
      cur = el;
      clearTimeout(timer);
      setTip(null);
      // Focus shows a tip only when it came from the keyboard, not when a dialog opens.
      if (el && (e.type !== 'focusin' || el.matches(':focus-visible'))) timer = window.setTimeout(() => show(el), e.type === 'focusin' ? 0 : 450);
    };
    const hide = () => {
      clearTimeout(timer);
      cur = null;
      setTip(null);
    };
    document.addEventListener('pointerover', over);
    document.addEventListener('focusin', over);
    document.addEventListener('pointerdown', hide);
    document.addEventListener('keydown', hide);
    document.addEventListener('scroll', hide, true);
    return () => {
      document.removeEventListener('pointerover', over);
      document.removeEventListener('focusin', over);
      document.removeEventListener('pointerdown', hide);
      document.removeEventListener('keydown', hide);
      document.removeEventListener('scroll', hide, true);
    };
  }, []);
  const ref = useRef<HTMLDivElement>(null);
  const [dx, setDx] = useState(0);
  useLayoutEffect(() => {
    if (!tip || !ref.current) return;
    const r = ref.current.getBoundingClientRect();
    setDx(r.left < 8 ? 8 - r.left : r.right > innerWidth - 8 ? innerWidth - 8 - r.right : 0);
  }, [tip]);
  if (!tip) return null;
  return (
    <div ref={ref} class="tooltip" role="tooltip" style={{ left: tip.x + dx, top: tip.y, transform: `translate(-50%, ${tip.below ? '0' : '-100%'})` }}>
      {tip.text}
    </div>
  );
}

export function Toasts() {
  const toasts = useStore(app, (s) => s.toasts);
  const icon: Record<string, IconName> = { success: 'ok', error: 'alert', warning: 'warning', info: 'info' };
  return (
    <div class="toasts" aria-live="polite">
      {toasts.map((x) => (
        <div class={`toast ${x.kind}`} key={x.id} role={x.kind === 'error' ? 'alert' : 'status'}>
          <Icon name={icon[x.kind]} />
          <span class="grow">{x.text}</span>
          {x.action && (
            <Button
              size="sm"
              kind="ghost"
              onClick={() => {
                x.action!.run();
                dismiss(x.id);
              }}
            >
              {x.action.label}
            </Button>
          )}
          <Button size="sm" kind="ghost" icon="x" tip={t('ui.close')} onClick={() => dismiss(x.id)} />
        </div>
      ))}
    </div>
  );
}

// ---- confirmation dialogs ----

type Dialog = {
  id: number;
  title: string;
  body?: ComponentChildren;
  confirm: string;
  cancel?: string;
  danger?: boolean;
  icon?: IconName;
  resolve: (ok: boolean) => void;
};
const dialogs = createStore<{ list: Dialog[] }>({ list: [] });
let dialogId = 0;

export function confirmDialog(d: Omit<Dialog, 'id' | 'resolve'>): Promise<boolean> {
  return new Promise((resolve) => {
    const id = ++dialogId;
    dialogs.set((s) => ({ list: [...s.list, { ...d, id, resolve }] }));
  });
}

export function DialogHost() {
  const list = useStore(dialogs, (s) => s.list);
  const done = (d: Dialog, ok: boolean) => {
    dialogs.set((s) => ({ list: s.list.filter((x) => x.id !== d.id) }));
    d.resolve(ok);
  };
  return (
    <>
      {list.map((d) => (
        <Modal
          key={d.id}
          size="narrow"
          icon={d.icon || (d.danger ? 'warning' : undefined)}
          title={d.title}
          onClose={() => done(d, false)}
          foot={
            <>
              <span class="spacer" />
              <Button onClick={() => done(d, false)}>{d.cancel || t('ui.cancel')}</Button>
              <Button kind={d.danger ? 'danger' : 'primary'} onClick={() => done(d, true)}>
                {d.confirm}
              </Button>
            </>
          }
        >
          {d.body && <div class="muted">{d.body}</div>}
        </Modal>
      ))}
    </>
  );
}

/** useAsync loads data and reloads when deps change. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): { data: T | undefined; error: string | null; loading: boolean; reload: () => void; set: (v: T) => void } {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [n, setN] = useState(0);
  useEffect(() => {
    let alive = true;
    setLoading(true);
    fn()
      .then((d) => {
        if (!alive) return;
        setData(d);
        setError(null);
      })
      .catch((e) => alive && setError(e?.code || 'app.internal_error'))
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [...deps, n]);
  return { data, error, loading, reload: () => setN((x) => x + 1), set: setData };
}

/** Kbd renders a keyboard shortcut. */
export const Kbd = (p: { children: ComponentChildren }) => <kbd>{p.children}</kbd>;
