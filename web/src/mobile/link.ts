// The phone's encrypted link to the computer. Pairing uses the single-use
// secret from the QR code (URL fragment, never sent over the network);
// afterwards every request, response and event is sealed with the device
// key (XChaCha20-Poly1305) and numbered against replay.
import { xchacha20poly1305 } from '@noble/ciphers/chacha';
import { randomBytes } from '@noble/ciphers/webcrypto';
import { hmac } from '@noble/hashes/hmac';
import { sha256 } from '@noble/hashes/sha256';

const enc = new TextEncoder();
const dec = new TextDecoder();

export function b64(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
export function unb64(s: string): Uint8Array {
  const n = s.replace(/-/g, '+').replace(/_/g, '/');
  const bin = atob(n + '==='.slice((n.length + 3) % 4));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function seal(key: Uint8Array, plaintext: Uint8Array, aad: string): Uint8Array {
  const nonce = randomBytes(24);
  const ct = xchacha20poly1305(key, nonce, enc.encode(aad)).encrypt(plaintext);
  const out = new Uint8Array(1 + 24 + ct.length);
  out[0] = 1;
  out.set(nonce, 1);
  out.set(ct, 25);
  return out;
}
function open(key: Uint8Array, sealed: Uint8Array, aad: string): Uint8Array {
  if (sealed.length < 1 + 24 + 16 || sealed[0] !== 1) throw new LinkError('mobile.session_ended');
  return xchacha20poly1305(key, sealed.subarray(1, 25), enc.encode(aad)).decrypt(sealed.subarray(25));
}
const mac = (key: Uint8Array, ...parts: string[]) => b64(hmac(sha256, key, enc.encode(parts.join('|'))));

export class LinkError extends Error {
  code: string;
  constructor(code: string) {
    super(code);
    this.code = code;
  }
}

type Session = { sid: string; key: string };
const STORE = 'mnelab.mobile';

export function saved(): Session | null {
  try {
    const s = JSON.parse(sessionStorage.getItem(STORE) || 'null');
    return s && s.sid && s.key ? s : null;
  } catch {
    return null;
  }
}
export function forget() {
  try {
    sessionStorage.removeItem(STORE);
  } catch {
    /* storage unavailable */
  }
}

/** pair exchanges the QR code's secret for a device session. */
export async function pair(p: string, k: string, label: string): Promise<Session> {
  const key = unb64(k);
  const ts = Date.now();
  const n = b64(randomBytes(16));
  let res: Response;
  try {
    res = await fetch('/m/api/pair', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ p, ts, n, label, mac: mac(key, 'mnelab-pair', p, String(ts), n, label) }) });
  } catch {
    throw new LinkError('app.unreachable');
  }
  const j = await res.json().catch(() => ({}));
  if (!res.ok) throw new LinkError(j.error || 'mobile.pairing_invalid');
  const box = open(key, unb64(j.box), 'mnelab-pair-resp|' + j.sid);
  key.fill(0);
  const s = { sid: j.sid as string, key: JSON.parse(dec.decode(box)).key as string };
  try {
    sessionStorage.setItem(STORE, JSON.stringify(s));
  } catch {
    /* kept in memory only */
  }
  return s;
}

export class Link {
  private key: Uint8Array;
  private seq = 0;
  private queue: Promise<unknown> = Promise.resolve();
  private es: EventSource | null = null;
  private retry = 0;
  ended = false;
  constructor(public s: Session) {
    this.key = unb64(s.key);
  }

  /** call runs one operation; calls are serialized so numbering stays in order. */
  call<T = any>(op: string, args?: unknown): Promise<T> {
    const run = async () => {
      if (this.ended) throw new LinkError('mobile.session_ended');
      this.seq = Math.max(this.seq + 1, Date.now());
      const seq = this.seq;
      const body = b64(seal(this.key, enc.encode(JSON.stringify({ seq, ts: Date.now(), op, args })), 'mnelab-req|' + this.s.sid));
      let res: Response;
      try {
        res = await fetch('/m/api/call', { method: 'POST', headers: { 'X-Sid': this.s.sid, 'Content-Type': 'text/plain' }, body });
      } catch {
        throw new LinkError('app.unreachable');
      }
      if (res.status === 401) {
        const j = await res.json().catch(() => ({}));
        if (j.error === 'mobile.replayed') throw new LinkError('mobile.replayed');
        this.end();
        throw new LinkError('mobile.session_ended');
      }
      if (!res.ok) {
        const j = await res.json().catch(() => ({}));
        throw new LinkError(j.error || 'app.internal_error');
      }
      const r = JSON.parse(dec.decode(open(this.key, unb64(await res.text()), 'mnelab-res|' + this.s.sid + '|' + seq)));
      if (!r.ok) throw new LinkError(r.error || 'app.internal_error');
      return r.data as T;
    };
    const p = this.queue.then(run, run);
    this.queue = p.catch(() => undefined);
    return p;
  }

  /** events delivers decrypted core events until the session ends. */
  events(fn: (type: string, data: any) => void, onState: (live: boolean) => void) {
    const connect = () => {
      if (this.ended) return;
      const ts = Date.now();
      const url = `/m/api/events?sid=${encodeURIComponent(this.s.sid)}&ts=${ts}&mac=${mac(this.key, 'mnelab-events', this.s.sid, String(ts))}`;
      const es = new EventSource(url);
      this.es = es;
      es.onopen = () => {
        this.retry = 0;
        onState(true);
      };
      es.onmessage = (e) => {
        try {
          const ev = JSON.parse(dec.decode(open(this.key, unb64(e.data), 'mnelab-evt|' + this.s.sid)));
          fn(ev.type, ev.data);
        } catch {
          /* not for this session */
        }
      };
      es.onerror = () => {
        es.close();
        onState(false);
        // A new URL each time: the computer refuses a repeated timestamp.
        setTimeout(connect, Math.min(1000 * 2 ** this.retry++, 10000));
      };
    };
    connect();
  }

  end() {
    this.ended = true;
    this.es?.close();
    this.key.fill(0);
    forget();
  }
}
