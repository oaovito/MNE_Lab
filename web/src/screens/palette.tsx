// Command palette (Ctrl K): find features, pages, settings, actions, help,
// files, graphs and cycles from anywhere.
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { changeAccount, exitApp, saveNow, setTheme, switchProfile, syncNow, toggleTurbo } from '../lib/actions';
import { get } from '../lib/api';
import { statisticsHeaders } from '../lib/analyses';
import { FEATURES, openHelp } from '../lib/features';
import { lang, t } from '../lib/i18n';
import { navigate } from '../lib/route';
import { app, openPanel } from '../lib/state';
import { useStore } from '../lib/store';
import type { CycleDoc, FileView, GraphDef } from '../lib/types';
import type { StatisticalAnalysis } from '../lib/statistics';
import { Icon, type IconName } from '../ui/icons';
import { Kbd, Portal } from '../ui/kit';
import { importFiles } from './ls/files';

type Entry = { id: string; kind: 'action' | 'page' | 'feature' | 'file' | 'graph' | 'cycle' | 'analysis'; icon: IconName; title: string; sub?: string; run: () => void; words?: string };

export function Palette() {
  const open = useStore(app, (s) => s.palette);
  if (!open) return null;
  return <Box />;
}

const close = () => app.set({ palette: false });

function Box() {
  const s = useStore(app, (x) => x.s)!;
  const [q, setQ] = useState('');
  const [i, setI] = useState(0);
  const [data, setData] = useState<{ files: FileView[]; graphs: GraphDef[]; cycles: CycleDoc[]; analyses:StatisticalAnalysis[] }>({ files: [], graphs: [], cycles: [], analyses:[] });
  const list = useRef<HTMLDivElement>(null);
  const prof = !!s.profile;

  useEffect(() => {
    if (!prof) return;
    Promise.all([get<FileView[] | null>('/api/files'), get<GraphDef[] | null>('/api/graphs'), get<CycleDoc[] | null>('/api/cycles'),get<StatisticalAnalysis[]>('/api/statistics',{headers:statisticsHeaders()})])
      .then(([files, graphs, cycles,analyses]) => setData({ files: files || [], graphs: graphs || [], cycles: cycles || [],analyses:analyses||[] }))
      .catch(() => undefined);
  }, []);

  const base = useMemo<Entry[]>(() => {
    const out: Entry[] = [];
    const page = (id: string, icon: IconName, title: string, run: () => void, words = '') => out.push({ id: 'p.' + id, kind: 'page', icon, title, run, words });
    const act = (id: string, icon: IconName, title: string, run: () => void, words = '') => out.push({ id: 'a.' + id, kind: 'action', icon, title, run, words });
    if (prof) {
      page('home', 'grid', t('nav.home'), () => navigate('/'));
      page('files', 'files', t('ls.tab.files'), () => navigate('/ls/files'), 'lightscattering dls');
      page('graphs', 'chart', t('ls.tab.graphs'), () => navigate('/ls/graphs'), 'lightscattering dls');
      page('cycles', 'cycle', t('ls.tab.cycles'), () => navigate('/ls/cycles'), 'lightscattering dls');
      page('statistics', 'sigma', t('ls.tab.statistics'), () => navigate('/ls/statistics'), 'anova tukey welch statistical analysis estatistica');
      page('mystuff', 'archive', t('menu.mystuff'), () => openPanel('mystuff'));
      for (const tab of ['general', 'profile', 'account', 'storage', 'backups', 'updates', 'mobile', 'privacy', 'about'])
        page('settings.' + tab, 'settings', t('settings.title') + ' · ' + t('settings.tab.' + tab), () => openPanel('settings', tab), t('settings.tab.' + tab));
      act('import', 'fileUp', t('pal.import'), () => (navigate('/ls/files'), importFiles()), 'upload');
      act('cycle', 'plus', t('cycles.new'), () => openPanel('cycle-wizard', { measurements: [] }));
      act('save', 'save', t('pal.save'), saveNow);
      if (s.profile!.storageMode !== 'usb_only') act('sync', 'refresh', t('status.sync_now'), syncNow);
      act('turbo', 'gauge', s.settings.turbo ? t('hdr.turbo_on') : t('hdr.turbo_off'), toggleTurbo, 'turbo');
      act('mobile', 'qr', t('hdr.mobile'), () => openPanel('mobile'), 'qr phone mobile');
      act('switch', 'users', t('menu.switch_profile'), switchProfile);
      act('change', 'logout', t('hdr.change_account'), changeAccount);
      act('exit', 'power', t('menu.exit'), exitApp, 'quit close');
    }
    act('theme.light', 'sun', t('pal.theme_light'), () => setTheme('light'), 'theme');
    act('theme.dark', 'moon', t('pal.theme_dark'), () => setTheme('dark'), 'theme');
    act('theme.system', 'monitor', t('pal.theme_system'), () => setTheme('system'), 'theme');
    page('help', 'book', t('menu.help'), () => openHelp());
    page('news', 'gift', t('menu.whatsnew'), () => openPanel('whatsnew'));
    for (const f of FEATURES) out.push({ id: 'f.' + f.id, kind: 'feature', icon: f.icon, title: t('feat.' + f.id), sub: t('fgroup.' + f.group), run: () => openHelp(f.id), words: (f.words || '') + ' ' + t('feat.' + f.id + '.what') });
    return out;
  }, [lang(), prof, s.settings.turbo]);

  const results = useMemo(() => {
    const n = q.trim().toLowerCase();
    const data2: Entry[] = [];
    if (n) {
      for (const f of data.files) data2.push({ id: 'file.' + f.id, kind: 'file', icon: 'file', title: f.name, sub: [f.experiment, f.group].filter(Boolean).join(' · ') || t('ls.tab.files'), run: () => navigate('/ls/files?file=' + f.id), words: (f.tags || []).join(' ') + ' ' + (f.items || []).map((m) => m.sampleId || '').join(' ') });
      for (const g of data.graphs) data2.push({ id: 'graph.' + g.id, kind: 'graph', icon: 'chart', title: g.title || t('graph.kind.' + g.kind), sub: t('graph.kind.' + g.kind), run: () => navigate('/ls/graphs/' + g.id), words: (g.tags || []).join(' ') });
      for (const c of data.cycles) data2.push({ id: 'cycle.' + c.id, kind: 'cycle', icon: 'cycle', title: c.config.name, sub: t('ls.tab.cycles'), run: () => navigate('/ls/cycles/' + c.id), words: [c.config.sampleId, c.config.experiment].join(' ') });
      for(const a of data.analyses)data2.push({id:'analysis.'+a.id,kind:'analysis',icon:'sigma',title:a.snapshot.definition.title,sub:t('stat.title'),run:()=>navigate('/ls/statistics/'+a.id),words:a.snapshot.definition.module+' '+t('stat.method.'+a.results.method)});
    }
    const all = [...base, ...data2];
    if (!n) return all.filter((e) => e.kind === 'action' || e.kind === 'page').slice(0, 12);
    const terms = n.split(/\s+/);
    const score = (e: Entry) => {
      const title = e.title.toLowerCase();
      const hay = (title + ' ' + (e.sub || '') + ' ' + (e.words || '')).toLowerCase();
      if (!terms.every((x) => hay.includes(x))) return -1;
      return (title.startsWith(terms[0]) ? 4 : title.includes(terms[0]) ? 2 : 0) + (e.kind === 'action' || e.kind === 'page' ? 1 : 0);
    };
    return all
      .map((e) => [score(e), e] as const)
      .filter(([sc]) => sc >= 0)
      .sort((a, b) => b[0] - a[0])
      .slice(0, 40)
      .map(([, e]) => e);
  }, [q, base, data]);

  useEffect(() => setI(0), [q]);
  useEffect(() => {
    list.current?.querySelector('.item.on')?.scrollIntoView({ block: 'nearest' });
  }, [i]);

  const go = (e?: Entry) => {
    if (!e) return;
    close();
    e.run();
  };
  const key = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown') (e.preventDefault(), setI(Math.min(i + 1, results.length - 1)));
    else if (e.key === 'ArrowUp') (e.preventDefault(), setI(Math.max(i - 1, 0)));
    else if (e.key === 'Enter') (e.preventDefault(), go(results[i]));
    else if (e.key === 'Escape') (e.preventDefault(), close());
  };

  return (
    <Portal>
      <div class="overlay" onMouseDown={(e) => e.target === e.currentTarget && close()}>
        <div class="palette" role="dialog" aria-modal="true" aria-label={t('pal.open')}>
          <div class="search">
            <Icon name="search" />
            <input autoFocus value={q} onInput={(e) => setQ((e.target as HTMLInputElement).value)} onKeyDown={key} placeholder={t('pal.placeholder')} aria-label={t('pal.placeholder')} role="combobox" aria-expanded="true" aria-controls="pal-results" />
          </div>
          <div class="results" ref={list} id="pal-results" role="listbox">
            {!q && <div class="menu-label">{t('pal.suggested')}</div>}
            {results.map((e, k) => (
              <button type="button" role="option" aria-selected={k === i} data-id={e.id} class={`item ${k === i ? 'on' : ''}`} onMouseMove={() => k !== i && setI(k)} onClick={() => go(e)}>
                <span class="ic">
                  <Icon name={e.icon} size="sm" />
                </span>
                <div class="col grow" style={{ gap: 0, minWidth: 0 }}>
                  <b class="ellipsis">{e.title}</b>
                  {e.sub && <p class="ellipsis">{e.sub}</p>}
                </div>
                <span class="xs faint">{t('pal.kind.' + e.kind)}</span>
              </button>
            ))}
            {q && !results.length && <p class="muted small" style={{ padding: 'var(--s4)' }}>{t('pal.none')}</p>}
          </div>
          <div class="foot">
            <span>
              <Kbd>↑↓</Kbd> {t('pal.move')}
            </span>
            <span>
              <Kbd>Enter</Kbd> {t('pal.go')}
            </span>
            <span>
              <Kbd>Esc</Kbd> {t('ui.close')}
            </span>
          </div>
        </div>
      </div>
    </Portal>
  );
}
