import { useEffect, useState } from 'preact/hooks';

export type Store<T> = {
  get(): T;
  set(p: Partial<T> | ((s: T) => Partial<T>)): void;
  subscribe(fn: () => void): () => void;
};

export function createStore<T extends object>(init: T): Store<T> {
  let s = init;
  const subs = new Set<() => void>();
  return {
    get: () => s,
    set(p) {
      const next = typeof p === 'function' ? (p as (s: T) => Partial<T>)(s) : p;
      s = { ...s, ...next };
      subs.forEach((f) => f());
    },
    subscribe(fn) {
      subs.add(fn);
      return () => subs.delete(fn);
    },
  };
}

export function useStore<T extends object, R>(store: Store<T>, sel: (s: T) => R): R {
  const [v, setV] = useState(() => sel(store.get()));
  useEffect(() => {
    const check = () => {
      const n = sel(store.get());
      setV((old) => (Object.is(old, n) ? old : n));
    };
    check();
    return store.subscribe(check);
  }, [store]);
  return v;
}
