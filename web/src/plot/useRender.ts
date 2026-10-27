// Renders a graph definition at the size of its container. The core does
// the scientific computation and the layout; the interface only draws.
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { post } from '../lib/api';
import type { Figure, GraphDef, Series, Visual } from '../lib/types';

export type Axis = { label: string; unit: string; scale: string; min: number; max: number; title?: string; categories?: string[] | null; conversion?: { from: string; to: string; factor: string } | null };
export type Source = { measurementId: string; fileId: string; fileName: string; sha256: string; parser: string; spec: string; sampleId?: string; measuredAt?: string; lines?: string; sourceSheet?: string; distributionId?: string; method?: string; format?: string };
export type Provenance = { engine: string; spec: string; rules: string[] | null; sources: Source[] | null; transformations: string[] | null; statistics?: string; computedAt: string };
export type Rendered = { figure: Figure; warnings: string[] | null; series: Series[] | null; provenance: Provenance; x: Axis; y: Axis; gaps?: number[] | null; weightings?: string[] | null; errorBars?: string };

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

export type RenderStage = 'idle' | 'queued' | 'computing';

/**
 * useRendered lays out def at w×h. A change waits briefly for the next one,
 * and a newer request aborts the one in flight (the core stops computing
 * it), so quick edits never pile up. stage and since tell the interface
 * what is happening; cancel stops the current render.
 */
export function useRendered(def: GraphDef | null, w: number, h: number, rev = 0) {
  const [state, setState] = useState<{ data: Rendered | null; error: string | null; loading: boolean; stage: RenderStage; since: number }>({ data: null, error: null, loading: true, stage: 'queued', since: 0 });
  const seq = useRef(0);
  const ctl = useRef<AbortController | null>(null);
  const key = def ? JSON.stringify(def) : '';
  const cancel = () => {
    seq.current++;
    ctl.current?.abort();
    ctl.current = null;
    setState((s) => ({ ...s, loading: false, stage: 'idle' }));
  };
  useEffect(() => {
    if (!def || w < 200 || h < 150) return;
    const n = ++seq.current;
    ctl.current?.abort();
    ctl.current = null;
    setState((s) => ({ ...s, loading: true, stage: 'queued', since: Date.now() }));
    const timer = setTimeout(() => {
      const c = new AbortController();
      ctl.current = c;
      setState((s) => (n === seq.current ? { ...s, stage: 'computing' } : s));
      post<Rendered>('/api/graphs/render', { definition: def, width: w, height: h }, { signal: c.signal })
        .then((data) => n === seq.current && setState({ data, error: null, loading: false, stage: 'idle', since: 0 }))
        .catch((e) => {
          if (n !== seq.current || e.code === 'request.canceled' || e.code === 'graph.render_cancelled') return;
          setState({ data: null, error: e.code || 'app.internal_error', loading: false, stage: 'idle', since: 0 });
        })
        .finally(() => ctl.current === c && (ctl.current = null));
    }, 90);
    return () => clearTimeout(timer);
  }, [key, w, h, rev]);
  useEffect(() => () => ctl.current?.abort(), []);
  return { ...state, cancel };
}
