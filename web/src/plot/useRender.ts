// Renders a graph definition at the size of its container. The core does
// the scientific computation and the layout; the interface only draws.
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { post } from '../lib/api';
import type { Figure, GraphDef, Series, Visual } from '../lib/types';

export type Axis = { label: string; unit: string; scale: string; min: number; max: number };
export type Source = { measurementId: string; fileId: string; fileName: string; sha256: string; parser: string; spec: string; sampleId?: string; measuredAt?: string; lines?: string; sourceSheet?: string; distributionId?: string; method?: string; format?: string };
export type Provenance = { engine: string; spec: string; rules: string[] | null; sources: Source[] | null; transformations: string[] | null; statistics?: string; computedAt: string };
export type Rendered = { figure: Figure; warnings: string[] | null; series: Series[] | null; provenance: Provenance; x: Axis; y: Axis; gaps?: number[] | null; weightings?: string[] | null };

export const defaultVisual = (): Visual => ({ legend: true, metadata: true, grid: true, points: false, lineWidth: 1.75, fontScale: 1, view: null });

/** useSize reports an element's content size (CSS pixels). */
export function useSize<T extends HTMLElement>(): [(el: T | null) => void, { w: number; h: number }] {
  const [size, setSize] = useState({ w: 0, h: 0 });
  const ro = useRef<ResizeObserver | null>(null);
  const ref = (el: T | null) => {
    ro.current?.disconnect();
    if (!el) return;
    ro.current = new ResizeObserver(([e]) => {
      const r = e.contentRect;
      setSize((s) => (Math.abs(s.w - r.width) < 2 && Math.abs(s.h - r.height) < 2 ? s : { w: Math.round(r.width), h: Math.round(r.height) }));
    });
    ro.current.observe(el);
  };
  useLayoutEffect(() => () => ro.current?.disconnect(), []);
  return [ref, size];
}

/** useRendered lays out def at w×h; stale requests are discarded. */
export function useRendered(def: GraphDef | null, w: number, h: number, rev = 0) {
  const [state, setState] = useState<{ data: Rendered | null; error: string | null; loading: boolean }>({ data: null, error: null, loading: true });
  const seq = useRef(0);
  const key = def ? JSON.stringify(def) : '';
  useEffect(() => {
    if (!def || w < 200 || h < 150) return;
    const n = ++seq.current;
    setState((s) => ({ ...s, loading: true }));
    const timer = setTimeout(() => {
      post<Rendered>('/api/graphs/render', { definition: def, width: w, height: h })
        .then((data) => n === seq.current && setState({ data, error: null, loading: false }))
        .catch((e) => n === seq.current && setState({ data: null, error: e.code || 'app.internal_error', loading: false }));
    }, 90);
    return () => clearTimeout(timer);
  }, [key, w, h, rev]);
  return state;
}
