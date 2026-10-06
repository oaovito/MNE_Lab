// Every overlay is opened through app.panel, so the tray, the palette, the
// shortcuts and the screens all reach the same components.
import { app } from '../lib/state';
import { useStore } from '../lib/store';
import { ExportDialog } from './export';
import { HelpCenter, WhatsNew } from './help';
import { CycleWizard } from './ls/cycles';
import { MyStuff } from './mystuff';
import { Busy, ExitConfirm, LsIntro, MobilePanel, Tour, TurboPanel } from './overlays';
import { Settings } from './settings';

export function Panels() {
  const panel = useStore(app, (s) => s.panel);
  const arg = useStore(app, (s) => s.panelArg);
  const hasProfile = useStore(app, (s) => !!s.s?.profile);
  if (!panel) return null;
  switch (panel) {
    case 'help':
      return <HelpCenter arg={arg} />;
    case 'restarting':
    case 'switching':
      return <Busy what={panel} arg={arg} />;
  }
  if (!hasProfile) return null;
  switch (panel) {
    case 'whatsnew':
      return <WhatsNew />;
    case 'mobile':
      return <MobilePanel />;
    case 'export':
      return <ExportDialog arg={arg} />;
    case 'cycle-wizard':
      return <CycleWizard arg={arg} />;
    case 'settings':
      return <Settings arg={arg} />;
    case 'mystuff':
      return <MyStuff />;
    case 'tour':
      return <Tour />;
    case 'ls-intro':
      return <LsIntro />;
    case 'turbo':
      return <TurboPanel />;
    case 'exit-confirm':
      return <ExitConfirm />;
  }
  return null;
}
