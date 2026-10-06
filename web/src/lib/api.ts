// Client of the local MNE Lab core (loopback only, session cookie).

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(code: string, status: number) {
    super(code);
    this.code = code;
    this.status = status;
  }
}

type Opts = { raw?: BodyInit; headers?: Record<string, string>; signal?: AbortSignal; text?: boolean };

export async function api<T = unknown>(method: string, path: string, body?: unknown, opts: Opts = {}): Promise<T> {
  const headers: Record<string, string> = { ...(opts.headers || {}) };
  let payload: BodyInit | undefined;
  if (opts.raw !== undefined) payload = opts.raw;
  else if (body !== undefined) {
    payload = JSON.stringify(body);
    headers['Content-Type'] = 'application/json';
  }
  if (method !== 'GET') headers['X-MNE-Lab'] = '1';
  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: payload, signal: opts.signal, credentials: 'same-origin' });
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw new ApiError('request.canceled', 0);
    throw new ApiError('app.unreachable', 0);
  }
  if (res.status === 204) return undefined as T;
  if (!res.ok) {
    let code = 'app.internal_error';
    try {
      const j = await res.json();
      if (j && typeof j.error === 'string') code = j.error;
    } catch {
      /* not JSON */
    }
    throw new ApiError(code, res.status);
  }
  if (opts.text) return (await res.text()) as T;
  const ct = res.headers.get('Content-Type') || '';
  if (ct.includes('application/json')) return (await res.json()) as T;
  return (await res.text()) as T;
}

export const get = <T>(p: string, o?: Opts) => api<T>('GET', p, undefined, o);
export const post = <T>(p: string, b?: unknown, o?: Opts) => api<T>('POST', p, b ?? {}, o);
export const put = <T>(p: string, b?: unknown, o?: Opts) => api<T>('PUT', p, b ?? {}, o);
export const patch = <T>(p: string, b?: unknown) => api<T>('PATCH', p, b ?? {});
export const del = <T>(p: string, b?: unknown) => api<T>('DELETE', p, b ?? {});

// ---- events (Server-Sent Events) ----

type Listener = (data: any) => void;
const listeners = new Map<string, Set<Listener>>();
let source: EventSource | null = null;
let retry = 500;

export function on(type: string, fn: Listener): () => void {
  if (!listeners.has(type)) listeners.set(type, new Set());
  listeners.get(type)!.add(fn);
  return () => listeners.get(type)?.delete(fn);
}

function emit(type: string, data: unknown) {
  listeners.get(type)?.forEach((fn) => fn(data));
  listeners.get('*')?.forEach((fn) => fn({ type, data }));
}

export function connectEvents() {
  if (source) return;
  source = new EventSource('/api/events');
  source.onopen = () => {
    retry = 500;
    emit('connection', true);
  };
  source.onmessage = (e) => {
    try {
      const ev = JSON.parse(e.data);
      emit(ev.type, ev.data);
    } catch {
      /* ignore */
    }
  };
  source.onerror = () => {
    source?.close();
    source = null;
    emit('connection', false);
    setTimeout(connectEvents, retry);
    retry = Math.min(retry * 2, 8000);
  };
}
