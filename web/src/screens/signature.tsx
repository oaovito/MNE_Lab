import { useEffect, useRef, useState } from 'preact/hooks';
import { api } from '../lib/api';
import { errText, t } from '../lib/i18n';
import { closePanel } from '../lib/state';
import { Button, Field, Modal, Notice } from '../ui/kit';

type Verification = {
  status: string; signature: string; content: string; trust: string;
  certificateTime: string; certificateSHA256?: string; subject?: string;
  claimedSigningTime?: string; revocation: string; trustedTimestamp: string;
};

// Files are transient browser handles. Changing either clears stale results;
// closing aborts the request and drops handles. Nothing is stored or imported.
export function VerifySignature() {
  const [content, setContent] = useState<File | null>(null);
  const [signature, setSignature] = useState<File | null>(null);
  const [result, setResult] = useState<Verification | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  const reset = () => { pending.current?.abort(); pending.current = null; setBusy(false); setResult(null); setError(''); };
  const verify = async () => {
    if (!content || !signature) return;
    if (content.size > 64 * 1024 * 1024 || signature.size > 4 * 1024 * 1024) { setError('signature.limit'); return; }
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true); setResult(null); setError('');
    const body = new FormData(); body.append('content', content); body.append('signature', signature);
    try {
      const r = await api<Verification>('POST', '/api/signatures/verify', undefined, { raw: body, signal: controller.signal });
      if (!controller.signal.aborted) setResult(r);
    } catch (e) {
      if (!controller.signal.aborted) setError((e as { code?: string }).code || 'signature.invalid');
    } finally {
      if (pending.current === controller) { pending.current = null; setBusy(false); }
    }
  };
  return (
    <Modal title={t('signature.title')} icon="shieldOk" size="wide" onClose={closePanel}
      foot={<><span class="spacer" /><Button onClick={closePanel}>{t('ui.close')}</Button><Button kind="primary" busy={busy} disabled={!content || !signature} onClick={verify}>{t('signature.verify')}</Button></>}>
      <div class="col gap3">
        <span class="small muted">{t('signature.scope')}</span>
        <Field label={t('signature.file')} id="verify-content">
          <input id="verify-content" class="input" type="file" onChange={(e) => { reset(); setContent(e.currentTarget.files?.[0] || null); }} />
        </Field>
        <Field label={t('signature.detached')} id="verify-cms">
          <input id="verify-cms" class="input" type="file" accept=".p7s,application/pkcs7-signature" onChange={(e) => { reset(); setSignature(e.currentTarget.files?.[0] || null); }} />
        </Field>
        {error && <Notice kind="danger">{errText(error)}</Notice>}
        {result && <div class="col gap2" role="status">
          <Notice kind={result.signature === 'valid' && result.content === 'matches' ? 'info' : 'danger'}>{t('signature.state.' + result.status)}</Notice>
          <div class="small">{t('signature.math')}: {t('signature.value.' + result.signature)}</div>
          <div class="small">{t('signature.bytes')}: {t('signature.value.' + result.content)}</div>
          <div class="small">{t('signature.trust')}: {t('signature.value.' + result.trust)}</div>
          <div class="small">{t('signature.time')}: {t('signature.value.' + result.certificateTime)}</div>
          {result.subject && <div class="small" style={{ overflowWrap: 'anywhere' }}>{t('signature.subject')}: {result.subject}</div>}
          {result.certificateSHA256 && <div class="xs muted" style={{ overflowWrap: 'anywhere' }}>SHA-256: {result.certificateSHA256}</div>}
          <span class="small muted">{t('signature.unchecked')}</span>
        </div>}
      </div>
    </Modal>
  );
}
