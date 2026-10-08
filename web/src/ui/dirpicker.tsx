// Folder picker inside the application (no operating system dialog), used
// for export destinations and desktop cloud folders.
import { useState } from 'preact/hooks';
import { get } from '../lib/api';
import { errText, t } from '../lib/i18n';
import { Icon } from './icons';
import { Button, Modal, Notice, Skeleton, useAsync } from './kit';

type Dir = { name: string; path: string };

const label = (d: Dir) => {
  if (d.name.startsWith('cloud.')) return t('dest.cloud', { name: d.name.slice(6) });
  const known = ['default', 'desktop', 'documents', 'downloads', 'drive'];
  return known.includes(d.name) ? t('dest.' + d.name) : d.name;
};

export function DirPicker(p: { title: string; start?: string; onPick: (path: string) => void; onClose: () => void; confirm?: string }) {
  const [path, setPath] = useState(p.start || '');
  const dirs = useAsync(() => get<Dir[]>('/api/fs/dirs?path=' + encodeURIComponent(path)), [path]);
  const sep = path.includes('\\') ? '\\' : '/';
  const parent = () => {
    const trimmed = path.replace(/[\\/]+$/, '');
    const i = trimmed.lastIndexOf(sep);
    if (i <= 0) return path.includes(':') && i === 2 ? '' : i === 0 ? '/' : '';
    const up = trimmed.slice(0, i);
    return /^[A-Za-z]:$/.test(up) ? up + '\\' : up;
  };
  return (
    <Modal
      title={p.title}
      icon="folderOpen"
      onClose={p.onClose}
      foot={
        <>
          <span class="xs faint ellipsis grow mono">{path || t('dest.choose_start')}</span>
          <Button onClick={p.onClose}>{t('ui.cancel')}</Button>
          <Button kind="primary" disabled={!path} onClick={() => p.onPick(path)}>
            {p.confirm || t('dest.use_folder')}
          </Button>
        </>
      }
    >
      <div class="col gap3">
        <div class="row">
          <Button size="sm" icon="back" disabled={!path} onClick={() => setPath(parent())}>
            {t('dest.up')}
          </Button>
          <Button size="sm" kind="ghost" icon="drive" onClick={() => setPath('')}>
            {t('dest.places')}
          </Button>
        </div>
        {dirs.error ? (
          <Notice kind="warning">{errText(dirs.error)}</Notice>
        ) : (
          <div class="dir-list" role="listbox">
            {dirs.loading && !dirs.data
              ? [1, 2, 3, 4].map(() => (
                  <div style={{ padding: '9px 12px' }}>
                    <Skeleton w="60%" />
                  </div>
                ))
              : (dirs.data || []).map((d) => (
                  <button onClick={() => setPath(d.path)} onDblClick={() => p.onPick(d.path)} title={d.path}>
                    <Icon name={path ? 'folder' : d.name === 'drive' ? 'usb' : d.name.startsWith('cloud.') ? 'cloud' : 'folder'} size="sm" />
                    <span class="grow ellipsis">{path ? d.name : label(d)}</span>
                    {!path && <span class="xs faint ellipsis mono" style={{ maxWidth: '50%' }}>{d.path}</span>}
                    <Icon name="right" size="sm" />
                  </button>
                ))}
            {dirs.data && dirs.data.length === 0 && <div class="small faint" style={{ padding: 'var(--s4)' }}>{t('dest.no_subfolders')}</div>}
          </div>
        )}
      </div>
    </Modal>
  );
}
