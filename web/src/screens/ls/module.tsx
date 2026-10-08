// LIGHTSCATTERING: File Library, Graph Library and Cycle Library.
import { useEffect } from 'preact/hooks';
import { seen } from '../../lib/actions';
import { t } from '../../lib/i18n';
import { useCycles, useFiles, useGraphs } from '../../lib/library';
import { match, navigate, useRoute } from '../../lib/route';
import { openPanel } from '../../lib/state';
import { Tabs } from '../../ui/kit';
import { CycleView } from './cycle';
import { CycleLibrary } from './cycles';
import { FileLibrary } from './files';
import { GraphView } from './graph';
import { GraphLibrary } from './graphs';

export function LsModule() {
  const { path } = useRoute();
  const files = useFiles();
  const graphs = useGraphs();
  const cycles = useCycles();
  const tab = path.startsWith('/ls/graphs') ? 'graphs' : path.startsWith('/ls/cycles') ? 'cycles' : 'files';

  useEffect(() => {
    if (!seen('ls')) openPanel('ls-intro');
  }, []);

  const g = match('/ls/graphs/:id', path);
  const c = match('/ls/cycles/:id', path);
  let body;
  if (g) body = <GraphView id={g.id} />;
  else if (c) body = <CycleView id={c.id} />;
  else if (tab === 'graphs') body = <GraphLibrary />;
  else if (tab === 'cycles') body = <CycleLibrary />;
  else body = <FileLibrary />;

  const detail = !!(g || c);
  return (
    <div class="page" style={{ paddingTop: 'var(--s3)', gap: 'var(--s3)' }}>
      {!detail && (
        <div class="row gap4">
          <div class="col" style={{ gap: 0 }}>
            <h1 style={{ fontSize: 'var(--fs-xl)' }}>LIGHTSCATTERING</h1>
            <span class="xs faint">{t('ls.subtitle')}</span>
          </div>
          <Tabs
            value={tab}
            onValue={(v) => navigate('/ls/' + v)}
            class="grow"
            tabs={[
              { value: 'files', label: t('ls.tab.files'), icon: 'files', count: files.data?.length },
              { value: 'graphs', label: t('ls.tab.graphs'), icon: 'chart', count: graphs.data?.length },
              { value: 'cycles', label: t('ls.tab.cycles'), icon: 'cycle', count: cycles.data?.length },
            ]}
          />
        </div>
      )}
      {body}
    </div>
  );
}
