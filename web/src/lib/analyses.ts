// Profile-scoped statistical library helpers; importing these does not load R.
import { get } from './api';
import { useRev } from './library';
import { app } from './state';
import { useStore } from './store';
import type { StatisticalAnalysis } from './statistics';
import { useAsync } from '../ui/kit';

export function statisticsHeaders() {
  const s = app.get().s;
  return { 'X-Account-ID': s?.account?.id || '', 'X-Profile-ID': s?.profile?.id || '' };
}
export function useAnalyses() {
  const s = useStore(app, x => x.s), rev = useRev();
  return useAsync(() => get<StatisticalAnalysis[]>('/api/statistics', { headers: statisticsHeaders() }), [s?.account?.id, s?.profile?.id, rev]);
}
