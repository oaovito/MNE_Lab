import { useEffect, useState } from 'preact/hooks';

type Loc = { path: string; query: URLSearchParams };
const subs = new Set<() => void>();
const read = (): Loc => ({ path: location.pathname, query: new URLSearchParams(location.search) });

export function navigate(to: string, replace = false) {
  if (to === location.pathname + location.search) return;
  if (replace) history.replaceState(null, '', to);
  else history.pushState(null, '', to);
  subs.forEach((f) => f());
}

window.addEventListener('popstate', () => subs.forEach((f) => f()));

export function useRoute(): Loc {
  const [loc, setLoc] = useState(read);
  useEffect(() => {
    const f = () => setLoc(read());
    subs.add(f);
    return () => {
      subs.delete(f);
    };
  }, []);
  return loc;
}

/** match('/ls/graphs/:id', path) → { id } or null */
export function match(pattern: string, path: string): Record<string, string> | null {
  const a = pattern.split('/').filter(Boolean);
  const b = path.split('/').filter(Boolean);
  if (a.length !== b.length) return null;
  const out: Record<string, string> = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(':')) out[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return out;
}
