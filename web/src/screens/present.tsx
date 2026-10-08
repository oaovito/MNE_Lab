// Presentation mode: saved graphs, one at a time, full screen on white
// paper. Display choices made here (legend, hidden series) are only for the
// presentation and never change the saved graph. The phone's Remote
// Controller drives the same state.
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { get, post } from '../lib/api';
import { t } from '../lib/i18n';
import { useRev } from '../lib/library';
import { app } from '../lib/state';
import { useStore } from '../lib/store';
import type { GraphDef, Presentation as Pr } from '../lib/types';
import { FigureView, seriesLabel } from '../plot/Figure';
import { useRendered, useSize } from '../plot/useRender';
import { Button, Menu, Spinner, useAsync, type MenuEntry } from '../ui/kit';

const send = (action: string, extra: Record<string, unknown> = {}) => post('/api/present', { action, ...extra }).catch(() => undefined);

export function Presentation() {
  const pr = useStore(app, (s) => s.s?.present);
  if (!pr?.active || !pr.graphs?.length) return null;
  return <Stage pr={pr} />;
}

function Stage(p: { pr: Pr }) {
  const pr = p.pr;
  const graphs = pr.graphs || [];
  const id = graphs[Math.min(pr.index, graphs.length - 1)];
  const rev = useRev();
  const g = useAsync(() => get<GraphDef>('/api/graphs/' + id), [id, rev]);
  const hidden = (pr.hidden || []).join();
  const def = useMemo(() => {
    if (!g.data) return null;
    const d: GraphDef = { ...g.data, visual: { ...g.data.visual, view: g.data.visual.view, fontScale: Math.max(g.data.visual.fontScale || 1, 1.2) } };
    if (pr.noLegend) d.visual.legend = false;
    if (pr.hidden?.length) {
      d.series = { ...(d.series || {}) };
      for (const s of pr.hidden) d.series[s] = { ...(d.series[s] || {}), hidden: true };
    }
    return d;
  }, [g.data, pr.noLegend, hidden]);
  const [ref, size] = useSize<HTMLDivElement>();
  const r = useRendered(def, size.w, size.h);
  const [idle, setIdle] = useState(false);
  const [menu, setMenu] = useState(false);
  const seriesBtn = useRef<HTMLButtonElement>(null);
  const wantFull = useRef(false);

  // Full screen when the browser allows it; the window is already chromeless.
  useEffect(() => {
    const el = document.documentElement;
    if (!document.fullscreenElement && el.requestFullscreen) {
      wantFull.current = true;
      el.requestFullscreen().catch(() => (wantFull.current = false));
    }
    return () => {
      if (document.fullscreenElement) document.exitFullscreen().catch(() => undefined);
    };
  }, []);

  // Controls fade while the pointer rests.
  useEffect(() => {
    let timer = 0;
    const wake = () => {
      setIdle(false);
      clearTimeout(timer);
      timer = window.setTimeout(() => setIdle(true), 2600);
    };
    wake();
    document.addEventListener('pointermove', wake);
    document.addEventListener('keydown', wake);
    return () => {
      clearTimeout(timer);
      document.removeEventListener('pointermove', wake);
      document.removeEventListener('keydown', wake);
    };
  }, []);

  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (menu || (e.target as HTMLElement)?.closest?.('input, textarea, select')) return;
      switch (e.key) {
        case 'ArrowRight':
        case 'PageDown':
        case ' ':
          e.preventDefault();
          send('next');
          break;
        case 'ArrowLeft':
        case 'PageUp':
          e.preventDefault();
          send('prev');
          break;
        case 'Home':
          send('goto', { index: 0 });
          break;
        case 'End':
          send('goto', { index: graphs.length - 1 });
          break;
        case 'l':
        case 'L':
          send('legend');
          break;
        case 'Escape':
          e.preventDefault();
          send('stop');
          break;
      }
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [menu, graphs.length]);

  const savedHidden = (sid: string) => !!g.data?.series?.[sid]?.hidden;
  const items: MenuEntry[] = (r.data?.series || [])
    .filter((s) => !savedHidden(s.id))
    .map((s) => ({ label: seriesLabel(g.data?.series?.[s.id]?.label || s.label), checked: !pr.hidden?.includes(s.id), run: () => send('series', { series: s.id }) }));
  const toggleFull = () => (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => undefined);

  return (
    <div class={`present ${idle && !menu ? 'idle' : ''}`} role="dialog" aria-modal="true" aria-label={t('pres.title')}>
      <div class="stage">
        <div class="paper" ref={ref}>
          {r.data && size.w > 0 ? (
            <FigureView figure={r.data.figure} series={r.data.series || []} label={g.data?.title} />
          ) : (
            <div class="grow" style={{ display: 'grid', placeItems: 'center', color: '#5b6573' }}>
              {r.error || g.error ? t('pres.unavailable') : <Spinner label={t('ui.loading')} />}
            </div>
          )}
        </div>
      </div>
      <div class="controls">
        <Button icon="left" tip={t('pres.prev') + ' (←)'} disabled={pr.index <= 0} onClick={() => send('prev')} />
        <span class="counter" aria-live="polite">
          {pr.index + 1} / {graphs.length}
        </span>
        <Button icon="right" tip={t('pres.next') + ' (→)'} disabled={pr.index >= graphs.length - 1} onClick={() => send('next')} />
        <span style={{ width: 18 }} />
        <Button icon="list" selected={!pr.noLegend} tip={t('pres.legend') + ' (L)'} onClick={() => send('legend')}>
          {t('pres.legend')}
        </Button>
        <Button btnRef={seriesBtn} icon="eye" trail="down" disabled={items.length < 1} onClick={() => setMenu(!menu)}>
          {t('pres.series')}
        </Button>
        <Button icon={document.fullscreenElement ? 'minimize' : 'maximize'} tip={t('pres.fullscreen')} onClick={toggleFull} />
        <span style={{ width: 18 }} />
        <Button icon="x" tip={t('pres.exit') + ' (Esc)'} onClick={() => send('stop')}>
          {t('pres.exit')}
        </Button>
      </div>
      {menu && seriesBtn.current && <Menu anchor={seriesBtn.current} items={items} onClose={() => setMenu(false)} />}
    </div>
  );
}
