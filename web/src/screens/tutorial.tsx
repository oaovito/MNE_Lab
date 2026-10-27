// The guided tutorial: short chapters that walk through the real screens,
// open each one and point at its controls. It appears once per profile and
// can be replayed, whole or from any chapter, from Help, Settings or search.
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { markSeen } from '../lib/actions';
import { t } from '../lib/i18n';
import { useCycles, useFiles, useGraphs } from '../lib/library';
import { navigate } from '../lib/route';
import { closePanel } from '../lib/state';
import { Icon, type IconName } from '../ui/icons';
import { Badge, Button } from '../ui/kit';

/** Saved items the stops can open, so they show real content when it exists. */
type Ctx = { file?: string; graph?: string; cycle?: string };
type Tone = 'ok' | 'busy' | 'warn' | 'err';
type Kind = 'accent' | 'success' | 'warning' | 'danger' | 'info';
/** A legend row: an indicator dot or a badge with the interface's own label, and what it means. */
type Legend = { label: string; key: string; tone?: Tone; badge?: Kind };
type Stop = {
  key: string;
  icon: IconName;
  /** Where the stop takes place; without one the current screen stays. */
  route?: (c: Ctx) => string;
  /** Controls to point at (the first visible match of each), or alt when none is shown. */
  target?: string[];
  alt?: string[];
  /** The target depends on data that may not exist (the card is then centered). */
  optional?: boolean;
  /** A tab or drawer clicked first, when the control lives behind it. */
  click?: string;
  points?: { icon: IconName; key: string }[];
  legend?: Legend[];
  flow?: boolean;
};
export type Chapter = { key: string; icon: IconName; stops: Stop[] };

const HOME = () => '/';
const FILES = () => '/ls/files';
const FILE = (c: Ctx) => (c.file ? '/ls/files?file=' + c.file : '/ls/files');
const GRAPHS = () => '/ls/graphs';
const GRAPH = (c: Ctx) => '/ls/graphs/' + (c.graph || 'new');
const CYCLES = () => '/ls/cycles';
const CYCLE = (c: Ctx) => (c.cycle ? '/ls/cycles/' + c.cycle : '/ls/cycles');
const STATS = () => '/ls/statistics';

const detailTab = (n: number) => `.split.files > .panel:last-child .tabs .tab:nth-child(${n}):not(.on)`;
const IMPORT = ['.toolbar .btn.primary'];
const AVATAR = ['.avatar-btn'];
// The graph inspector is a drawer on narrow windows: open it for its stop, close it for the others.
const DRAWER_OPEN = '.page-head .narrow-only:not(.selected)';
const DRAWER_CLOSE = '.page-head .narrow-only.selected';

export const CHAPTERS: Chapter[] = [
  {
    key: 'start',
    icon: 'sparkles',
    stops: [
      { key: 'tut.welcome', icon: 'sparkles', route: HOME, flow: true },
      { key: 'tut.home_flow', icon: 'workflow', route: HOME, target: ['.module-card .flow'] },
      {
        key: 'tut.home_import',
        icon: 'upload',
        route: HOME,
        target: ['.hero-import'],
        points: [
          { icon: 'keyboard', key: 'tut.p.ctrl_i' },
          { icon: 'fileUp', key: 'tut.p.drop' },
        ],
      },
      { key: 'tut.home_stats', icon: 'chart', route: HOME, target: ['.module-card .stats'] },
      { key: 'tut.home_system', icon: 'activity', route: HOME, target: ['.home .side > .card:first-child'] },
      { key: 'tut.home_session', icon: 'power', route: HOME, target: ['.home .side > .card:last-child'] },
    ],
  },
  {
    key: 'header',
    icon: 'grid',
    stops: [
      { key: 'tut.nav', icon: 'grid', route: HOME, target: ['.header .nav'] },
      { key: 'tut.search', icon: 'search', route: HOME, target: ['.search-btn'] },
      { key: 'tut.status', icon: 'cloudOk', route: HOME, target: ['.status-pill'] },
      {
        key: 'tut.tools',
        icon: 'qr',
        route: HOME,
        target: ['.hdr-mobile', '.hdr-turbo', '.hdr-help'],
        points: [
          { icon: 'phone', key: 'tut.p.phone' },
          { icon: 'gauge', key: 'tut.p.turbo' },
          { icon: 'help', key: 'tut.p.help' },
          { icon: 'gift', key: 'tut.p.news' },
        ],
      },
    ],
  },
  {
    key: 'files',
    icon: 'files',
    stops: [
      { key: 'tut.ls_tabs', icon: 'flask', route: FILES, target: ['.ls-tabs'] },
      {
        key: 'tut.files_import',
        icon: 'fileUp',
        route: FILES,
        target: IMPORT,
        alt: ['.empty .btn.primary'],
        points: [
          { icon: 'file', key: 'tut.p.fmt_text' },
          { icon: 'sheet', key: 'tut.p.fmt_table' },
          { icon: 'eye', key: 'tut.p.fmt_ods' },
          { icon: 'package', key: 'tut.p.fmt_dts' },
        ],
      },
      {
        key: 'tut.import_review',
        icon: 'listChecks',
        route: FILES,
        target: IMPORT,
        alt: ['.empty .btn.primary'],
        points: [
          { icon: 'warning', key: 'tut.p.imp_partial' },
          { icon: 'copy', key: 'tut.p.imp_duplicate' },
          { icon: 'calendarClock', key: 'tut.p.imp_date' },
          { icon: 'alert', key: 'tut.p.imp_failed' },
        ],
      },
      {
        key: 'tut.import_mapping',
        icon: 'table',
        route: FILES,
        target: IMPORT,
        alt: ['.empty .btn.primary'],
        points: [
          { icon: 'ruler', key: 'tut.p.map_units' },
          { icon: 'warning', key: 'tut.p.map_partial' },
          { icon: 'layers', key: 'tut.p.map_sheets' },
          { icon: 'save', key: 'tut.p.map_saved' },
        ],
      },
      {
        key: 'tut.files_list',
        icon: 'files',
        route: FILES,
        target: ['.split.files > .panel:first-child'],
        alt: ['.empty'],
        points: [
          { icon: 'ok', key: 'tut.p.file_icon' },
          { icon: 'calendarClock', key: 'tut.p.file_calendar' },
          { icon: 'layers', key: 'tut.p.file_sets' },
        ],
      },
      {
        key: 'tut.file_detail',
        icon: 'file',
        route: FILE,
        target: ['.split.files > .panel:last-child'],
        click: detailTab(1),
        points: [
          { icon: 'file', key: 'tut.p.file_original' },
          { icon: 'link', key: 'tut.p.file_used' },
        ],
      },
      {
        key: 'tut.file_values',
        icon: 'ruler',
        route: FILE,
        target: ['.split.files .meas'],
        click: detailTab(2),
        points: [
          { icon: 'info', key: 'tut.p.val_unit' },
          { icon: 'hash', key: 'tut.p.val_origin' },
          { icon: 'pencil', key: 'tut.p.val_edit' },
        ],
      },
      { key: 'tut.file_dates', icon: 'calendarClock', route: FILE, target: ['.split.files .meas .row.muted'], click: detailTab(2), points: [{ icon: 'clock', key: 'tut.p.date_tz' }] },
      {
        key: 'tut.file_distribution',
        icon: 'spline',
        route: FILE,
        target: ['.split.files .meas .select'],
        alt: ['.split.files .meas'],
        optional: true,
        click: detailTab(2),
        points: [{ icon: 'info', key: 'tut.p.dist_g' }],
      },
      { key: 'tut.files_select', icon: 'listChecks', route: FILES, target: ['.split.files thead th:first-child', '.split.files tbody tr:first-child td:first-child'] },
      { key: 'tut.files_trash', icon: 'trash', route: FILES, target: ['.toolbar .seg'], points: [{ icon: 'warning', key: 'tut.p.trash_used' }] },
    ],
  },
  {
    key: 'graphs',
    icon: 'chart',
    stops: [
      { key: 'tut.graphs_library', icon: 'chart', route: GRAPHS, target: ['.cards .gcard'], alt: ['.empty'], points: [{ icon: 'listChecks', key: 'tut.p.graphs_select' }] },
      {
        key: 'tut.graphs_kinds',
        icon: 'layers',
        route: GRAPHS,
        target: ['.toolbar .graphs-kind'],
        points: [
          { icon: 'chart', key: 'tut.p.kind_dist' },
          { icon: 'activity', key: 'tut.p.kind_param' },
          { icon: 'spline', key: 'tut.p.kind_cycle' },
          { icon: 'sigma', key: 'tut.p.kind_stat' },
          { icon: 'users', key: 'tut.p.kind_design' },
        ],
      },
      {
        key: 'tut.graph_series',
        icon: 'listChecks',
        route: GRAPH,
        target: ['.builder-left .sx'],
        alt: ['.builder-left'],
        click: DRAWER_CLOSE,
        points: [
          { icon: 'search', key: 'tut.p.series_find' },
          { icon: 'listChecks', key: 'tut.p.series_select' },
          { icon: 'eye', key: 'tut.p.series_focus' },
        ],
      },
      {
        key: 'tut.graph_setup',
        icon: 'sliders',
        route: GRAPH,
        target: ['.builder-right .gb-settings'],
        click: DRAWER_OPEN,
        points: [
          { icon: 'info', key: 'tut.p.setup_weighting' },
          { icon: 'cycle', key: 'tut.p.setup_cycle' },
          { icon: 'sigma', key: 'tut.p.setup_bars' },
        ],
      },
      { key: 'tut.graph_figure', icon: 'zoom', route: GRAPH, target: ['.figure-wrap'], click: DRAWER_CLOSE, points: [{ icon: 'warning', key: 'tut.p.figure_notes' }] },
      { key: 'tut.graph_display', icon: 'palette', route: GRAPH, target: ['.builder-right .gb-sec-lines'], alt: ['.builder-right'], click: DRAWER_OPEN },
      { key: 'tut.graph_inspector', icon: 'panel', route: GRAPH, target: ['.builder > .builder-right'], click: DRAWER_OPEN },
      { key: 'tut.graph_save', icon: 'save', route: GRAPH, target: ['.builder-bar .gb-save'], click: DRAWER_CLOSE },
      {
        key: 'tut.graph_design',
        icon: 'layers',
        route: GRAPH,
        target: ['.builder-right .gb-sec-bars', '.builder-right .gb-settings'],
        click: DRAWER_OPEN,
        points: [
          { icon: 'eye', key: 'tut.p.design_points' },
          { icon: 'link', key: 'tut.p.design_paired' },
        ],
      },
      {
        key: 'tut.graph_combine',
        icon: 'layers',
        route: GRAPH,
        target: ['.builder-bar .gb-combine', '.builder-bar .gb-analyze'],
        click: DRAWER_CLOSE,
        points: [
          { icon: 'ban', key: 'tut.p.combine_refused' },
          { icon: 'history', key: 'tut.p.combine_lineage' },
          { icon: 'sigma', key: 'tut.p.annotate_saved' },
        ],
      },
      { key: 'tut.groups', icon: 'listChecks', route: () => '/ls/groups', target: ['.groups-lib'], alt: ['.page .panel'] },
    ],
  },
  {
    key: 'cycles',
    icon: 'cycle',
    stops: [
      {
        key: 'tut.cycles_library',
        icon: 'cycle',
        route: CYCLES,
        target: ['.toolbar .btn.primary'],
        points: [
          { icon: 'copy', key: 'tut.p.cycle_replicates' },
          { icon: 'sliders', key: 'tut.p.cycle_params' },
          { icon: 'plus', key: 'tut.p.cycle_create' },
          { icon: 'listChecks', key: 'tut.p.cycle_check' },
        ],
      },
      { key: 'tut.cycle_plan', icon: 'clock', route: CYCLE, target: ['.split.cycle > .col > .card:first-child'] },
      {
        key: 'tut.cycle_assign',
        icon: 'listChecks',
        route: CYCLE,
        target: ['.split.cycle .points'],
        points: [
          { icon: 'warning', key: 'tut.p.reason_between' },
          { icon: 'ban', key: 'tut.p.reason_far' },
          { icon: 'calendarClock', key: 'tut.p.reason_date' },
          { icon: 'flask', key: 'tut.p.reason_other' },
        ],
      },
      { key: 'tut.cycle_results', icon: 'sigma', route: CYCLE, target: ['.split.cycle .cycle-results'] },
      { key: 'tut.cycle_actions', icon: 'chart', route: CYCLE, target: ['.page-head .btn.primary'] },
    ],
  },
  {
    key: 'stats',
    icon: 'sigma',
    stops: [
      { key: 'tut.stats_tab', icon: 'sigma', route: STATS, target: ['.page .panel-head .btn.primary'] },
      {
        key: 'tut.stats_flow',
        icon: 'listChecks',
        route: STATS,
        target: ['.page .panel-head .btn.primary'],
        points: [
          { icon: 'ban', key: 'tut.p.stat_missing' },
          { icon: 'history', key: 'tut.p.stat_revision' },
          { icon: 'warning', key: 'tut.p.stat_changed' },
          { icon: 'chart', key: 'tut.p.stat_graph' },
        ],
      },
      {
        key: 'tut.stats_scope',
        icon: 'shield',
        route: STATS,
        points: [
          { icon: 'ok', key: 'tut.p.scope_stats' },
          { icon: 'warning', key: 'tut.p.scope_dls' },
          { icon: 'warning', key: 'tut.p.scope_mapping' },
          { icon: 'warning', key: 'tut.p.scope_zeta' },
          { icon: 'info', key: 'tut.p.scope_signature' },
        ],
      },
    ],
  },
  {
    key: 'share',
    icon: 'present',
    stops: [
      {
        key: 'tut.present',
        icon: 'present',
        route: GRAPH,
        target: ['.builder-bar .graph-present'],
        points: [
          { icon: 'listChecks', key: 'tut.p.present_many' },
          { icon: 'cycle', key: 'tut.p.present_cycle' },
        ],
      },
      {
        key: 'tut.phone',
        icon: 'qr',
        route: GRAPH,
        target: ['.hdr-mobile'],
        points: [
          { icon: 'shieldOk', key: 'tut.p.phone_code' },
          { icon: 'power', key: 'tut.p.phone_end' },
        ],
      },
      {
        key: 'tut.export',
        icon: 'download',
        route: GRAPH,
        target: ['.builder-bar .graph-export'],
        points: [
          { icon: 'image', key: 'tut.p.export_figures' },
          { icon: 'table', key: 'tut.p.export_raw' },
          { icon: 'package', key: 'tut.p.export_package' },
          { icon: 'sliders', key: 'tut.p.export_custom' },
        ],
      },
      {
        key: 'tut.export_safe',
        icon: 'shieldOk',
        route: GRAPH,
        target: ['.builder-bar .graph-export'],
        points: [
          { icon: 'files', key: 'tut.p.export_where' },
          { icon: 'archive', key: 'tut.p.export_recent' },
        ],
      },
      {
        key: 'tut.signature',
        icon: 'shieldOk',
        route: HOME,
        target: AVATAR,
        points: [
          { icon: 'listChecks', key: 'tut.p.signature_result' },
          { icon: 'info', key: 'tut.p.signature_trust' },
        ],
      },
    ],
  },
  {
    key: 'status',
    icon: 'cloudOk',
    stops: [
      {
        key: 'tut.status_ok',
        icon: 'cloudOk',
        route: HOME,
        target: ['.status-pill'],
        legend: [
          { tone: 'ok', label: 'status.local', key: 'tut.l.local' },
          { tone: 'ok', label: 'status.synced', key: 'tut.l.synced' },
          { tone: 'busy', label: 'status.syncing', key: 'tut.l.syncing' },
          { tone: 'warn', label: 'status.pending', key: 'tut.l.pending' },
          { tone: 'warn', label: 'status.offline', key: 'tut.l.offline' },
          { tone: 'warn', label: 'status.folder_copy', key: 'tut.l.folder' },
        ],
      },
      {
        key: 'tut.status_err',
        icon: 'warning',
        route: HOME,
        target: ['.status-pill'],
        legend: [
          { tone: 'warn', label: 'status.not_connected', key: 'tut.l.not_connected' },
          { tone: 'err', label: 'status.reconnect', key: 'tut.l.reconnect' },
          { tone: 'err', label: 'status.conflict', key: 'tut.l.conflict' },
          { tone: 'err', label: 'status.error', key: 'tut.l.error' },
        ],
        points: [{ icon: 'activity', key: 'tut.p.status_activity' }],
      },
      {
        key: 'tut.status_badges',
        icon: 'tag',
        route: HOME,
        legend: [
          { badge: 'success', label: 'file.status.parsed', key: 'tut.l.read' },
          { badge: 'warning', label: 'file.status.partial', key: 'tut.l.read_notes' },
          { badge: 'danger', label: 'file.status.failed', key: 'tut.l.unreadable' },
          { badge: 'warning', label: 'tut.l.partial_label', key: 'tut.l.partial' },
          { badge: 'warning', label: 'graph.unsaved', key: 'tut.l.unsaved' },
          { badge: 'accent', label: 'ui.new', key: 'tut.l.new' },
        ],
      },
      {
        key: 'tut.status_messages',
        icon: 'info',
        route: HOME,
        legend: [
          { badge: 'info', label: 'tut.l.info_label', key: 'tut.l.info' },
          { badge: 'success', label: 'tut.l.success_label', key: 'tut.l.success' },
          { badge: 'warning', label: 'tut.l.warning_label', key: 'tut.l.warning' },
          { badge: 'danger', label: 'tut.l.danger_label', key: 'tut.l.danger' },
        ],
        points: [{ icon: 'undo', key: 'tut.p.messages_undo' }],
      },
    ],
  },
  {
    key: 'profile',
    icon: 'user',
    stops: [
      {
        key: 'tut.profile_menu',
        icon: 'user',
        route: HOME,
        target: AVATAR,
        points: [
          { icon: 'archive', key: 'tut.p.menu_mystuff' },
          { icon: 'settings', key: 'tut.p.menu_help' },
          { icon: 'users', key: 'tut.p.menu_switch' },
          { icon: 'power', key: 'tut.p.menu_exit' },
        ],
      },
      {
        key: 'tut.profiles',
        icon: 'users',
        route: HOME,
        target: AVATAR,
        points: [
          { icon: 'key', key: 'tut.p.profiles_key' },
          { icon: 'usb', key: 'tut.p.profiles_remember' },
        ],
      },
      { key: 'tut.settings_general', icon: 'settings', route: HOME, target: AVATAR, points: [{ icon: 'user', key: 'tut.p.settings_profile' }] },
      {
        key: 'tut.settings_storage',
        icon: 'cloud',
        route: HOME,
        target: ['.home .side .status-item:nth-child(1)'],
        points: [
          { icon: 'usb', key: 'tut.p.storage_usb_cloud' },
          { icon: 'drive', key: 'tut.p.storage_usb_only' },
          { icon: 'cloud', key: 'tut.p.storage_cloud_only' },
          { icon: 'info', key: 'tut.p.storage_providers' },
        ],
      },
      {
        key: 'tut.settings_safety',
        icon: 'history',
        route: HOME,
        target: ['.home .side .status-item:nth-child(2)', '.home .side .status-item:nth-child(3)'],
        points: [
          { icon: 'package', key: 'tut.p.safety_updates' },
          { icon: 'lock', key: 'tut.p.safety_locked' },
        ],
      },
      { key: 'tut.settings_privacy', icon: 'shield', route: HOME, target: AVATAR, points: [{ icon: 'phone', key: 'tut.p.privacy_tabs' }] },
    ],
  },
  {
    key: 'session',
    icon: 'power',
    stops: [
      {
        key: 'tut.modes',
        icon: 'usb',
        route: HOME,
        target: ['.home-hero .sub .badge'],
        points: [
          { icon: 'usb', key: 'tut.p.mode_portable' },
          { icon: 'laptop', key: 'tut.p.mode_temporary' },
        ],
      },
      { key: 'tut.save', icon: 'save', route: HOME },
      {
        key: 'tut.exit',
        icon: 'power',
        route: HOME,
        target: ['.home .side .quick-row:last-child'],
        points: [
          { icon: 'usb', key: 'tut.p.exit_usb' },
          { icon: 'lifebuoy', key: 'tut.p.exit_recovery' },
        ],
      },
      { key: 'tut.done', icon: 'sparkles', route: HOME },
    ],
  },
];

const STOPS = CHAPTERS.flatMap((c, ch) => c.stops.map((s) => ({ ...s, ch })));
const FIRST = CHAPTERS.map((_, ch) => STOPS.findIndex((s) => s.ch === ch));
const LS = CHAPTERS.findIndex((c) => c.key === 'files');

const FLOW: { icon: IconName; k: string }[] = [
  { icon: 'upload', k: 'home.flow.import' },
  { icon: 'listChecks', k: 'home.flow.review' },
  { icon: 'chart', k: 'home.flow.graph' },
  { icon: 'cycle', k: 'home.flow.cycle' },
  { icon: 'sigma', k: 'tut.flow.statistics' },
  { icon: 'present', k: 'home.flow.present' },
  { icon: 'download', k: 'home.flow.export' },
];
const TONE: Record<Tone, string> = { ok: 'var(--success)', busy: 'var(--info)', warn: 'var(--warning)', err: 'var(--danger)' };

type Box = { x: number; y: number; w: number; h: number };
// The narrowest card that still reads well beside a wide target.
const NARROW = 280;
type Spot = { left: number; top: number; width?: number; side?: 'top' | 'bottom' | 'left' | 'right'; caret?: number };

const shown = (e: HTMLElement) => {
  if (e.closest('.coach')) return false;
  const r = e.getBoundingClientRect();
  return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== 'hidden';
};
const first = (q: string) => [...document.querySelectorAll<HTMLElement>(q)].find(shown);

/** The first visible match of each selector, or null when none is shown. */
function find(sel?: string[]): HTMLElement[] | null {
  const els = (sel || []).map(first).filter((e): e is HTMLElement => !!e);
  return els.length ? els : null;
}

/** The union of the elements, padded and kept inside the window. */
function boxOf(els: HTMLElement[]): Box | null {
  const rs = els.map((e) => e.getBoundingClientRect());
  const x = Math.max(4, Math.min(...rs.map((r) => r.left)) - 6);
  const y = Math.max(4, Math.min(...rs.map((r) => r.top)) - 6);
  const r = Math.min(innerWidth - 4, Math.max(...rs.map((r) => r.right)) + 6);
  const b = Math.min(innerHeight - 4, Math.max(...rs.map((r) => r.bottom)) + 6);
  return r - x > 8 && b - y > 8 ? { x: Math.round(x), y: Math.round(y), w: Math.round(r - x), h: Math.round(b - y) } : null;
}

/**
 * place puts the card next to its target: below, above, to the right or to
 * the left, whichever fits first; then beside it a little narrower; otherwise
 * in the window corner that covers the least of it. Without a target the card
 * is centered.
 */
function place(b: Box | null, W: number, H: number, vw: number, vh: number): Spot {
  const m = 16,
    g = 14;
  const cx = (x: number) => Math.max(m, Math.min(x, vw - m - W));
  const cy = (y: number) => Math.max(m, Math.min(y, vh - m - H));
  const caret = (at: number, len: number) => Math.max(22, Math.min(at, len - 22));
  if (!b) return { left: Math.round((vw - W) / 2), top: Math.round(cy((vh - H) / 2)) };
  const mx = b.x + b.w / 2,
    my = b.y + b.h / 2;
  if (b.y + b.h + g + H <= vh - m) {
    const left = cx(mx - W / 2);
    return { left, top: b.y + b.h + g, side: 'top', caret: caret(mx - left, W) };
  }
  if (b.y - g - H >= m) {
    const left = cx(mx - W / 2);
    return { left, top: b.y - g - H, side: 'bottom', caret: caret(mx - left, W) };
  }
  if (b.x + b.w + g + W <= vw - m) {
    const top = cy(my - H / 2);
    return { left: b.x + b.w + g, top, side: 'left', caret: caret(my - top, H) };
  }
  if (b.x - g - W >= m) {
    const top = cy(my - H / 2);
    return { left: b.x - g - W, top, side: 'right', caret: caret(my - top, H) };
  }
  const toRight = vw - m - (b.x + b.w + g),
    toLeft = b.x - g - m;
  if (Math.max(toRight, toLeft) >= NARROW) {
    const width = Math.floor(Math.min(W, Math.max(toRight, toLeft)));
    const top = cy(my - H / 2);
    return toRight >= toLeft
      ? { left: b.x + b.w + g, top, width, side: 'left', caret: caret(my - top, H) }
      : { left: b.x - g - width, top, width, side: 'right', caret: caret(my - top, H) };
  }
  const over = (l: number, t: number) => Math.max(0, Math.min(l + W, b.x + b.w) - Math.max(l, b.x)) * Math.max(0, Math.min(t + H, b.y + b.h) - Math.max(t, b.y));
  const spots: [number, number][] = [
    [m, m],
    [vw - m - W, m],
    [m, cy(vh - m - H)],
    [vw - m - W, cy(vh - m - H)],
  ];
  const [left, top] = spots.reduce((a, s) => (over(s[0], s[1]) < over(a[0], a[1]) ? s : a));
  return { left, top };
}

export function Tutorial(p: { arg?: string | null }) {
  const start = Math.max(0, FIRST[CHAPTERS.findIndex((c) => c.key === p.arg)] ?? 0);
  const [i, setI] = useState(start);
  const [list, setList] = useState(false);
  const [view, setView] = useState<{ box: Box | null; waiting: boolean; ch: number; vw: number; vh: number }>({ box: null, waiting: true, ch: 0, vw: innerWidth, vh: innerHeight });
  const cardRef = useRef<HTMLDivElement>(null);
  const back = useRef(location.pathname + location.search);
  const reached = useRef(new Set<number>());
  const files = useFiles();
  const graphs = useGraphs();
  const cycles = useCycles();
  const stop = STOPS[i];
  const last = i === STOPS.length - 1;
  const chapter = CHAPTERS[stop.ch];
  const inChapter = STOPS.filter((s) => s.ch === stop.ch);
  reached.current.add(stop.ch);

  const ctx: Ctx = {
    file: (files.data?.find((f) => f.items?.length) || files.data?.[0])?.id,
    graph: (graphs.data?.find((g) => g.kind === 'dls_distribution') || graphs.data?.[0])?.id,
    cycle: cycles.data?.[0]?.id,
  };
  const route = stop.route?.(ctx);

  const done = async () => {
    closePanel();
    // The tutorial covered LIGHTSCATTERING once its chapters were reached.
    await markSeen('tutorial', ...(reached.current.has(LS) ? ['ls'] : []));
    navigate(back.current, true);
  };
  const go = (n: number) => n >= 0 && n < STOPS.length && (setI(n), setList(false));

  // Each stop opens its screen; a link to saved content follows once it loads.
  useEffect(() => {
    if (route && route !== location.pathname + location.search) navigate(route, true);
  }, [i, route]);

  useLayoutEffect(() => {
    const t0 = performance.now();
    let prev = '';
    let clicked = !stop.click;
    let scrolled = false;
    const update = () => {
      const now = performance.now() - t0;
      if (!clicked) {
        const el = first(stop.click!);
        if (el) (el.click(), (clicked = true));
        else if (now > 1500) clicked = true;
      }
      const els = find(stop.target) || find(stop.alt);
      if (els && !scrolled) (els[0].scrollIntoView({ block: 'nearest', inline: 'nearest' }), (scrolled = true));
      const box = els && boxOf(els);
      const waiting = !box && !!stop.target && now < 700;
      const ch = cardRef.current?.offsetHeight || 0;
      const k = JSON.stringify([box, waiting, ch, innerWidth, innerHeight]);
      if (k === prev) return;
      prev = k;
      setView({ box, waiting, ch, vw: innerWidth, vh: innerHeight });
    };
    update();
    const raf = requestAnimationFrame(update);
    const iv = setInterval(update, 200);
    addEventListener('resize', update);
    return () => {
      cancelAnimationFrame(raf);
      clearInterval(iv);
      removeEventListener('resize', update);
    };
  }, [i, list]);

  useEffect(() => {
    cardRef.current?.querySelector<HTMLElement>(list ? '.coach-chapters button.on' : '.coach-foot button.primary')?.focus();
  }, [i, list]);

  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        if (list) setList(false);
        else done();
      } else if (e.key === 'ArrowRight' && !list) go(i + 1);
      else if (e.key === 'ArrowLeft' && !list) go(i - 1);
      else if (e.key === 'Tab' && cardRef.current) {
        const f = cardRef.current.querySelectorAll<HTMLElement>('button:not([disabled])');
        const a = f[0],
          b = f[f.length - 1];
        if (e.shiftKey && document.activeElement === a) (e.preventDefault(), b.focus());
        else if (!e.shiftKey && document.activeElement === b) (e.preventDefault(), a.focus());
      }
    };
    document.addEventListener('keydown', k);
    return () => document.removeEventListener('keydown', k);
  }, [i, list]);

  const box = list ? null : view.box;
  const W = Math.min(420, view.vw - 32);
  const spot = place(box, W, view.ch || 320, view.vw, view.vh);
  const label = (l: Legend) => t(l.label, { n: 1 });

  return (
    <div class="coach" role="presentation">
      {box ? <div class="coach-hole" style={{ left: box.x, top: box.y, width: box.w, height: box.h }} /> : <div class="coach-scrim" />}
      <div
        ref={cardRef}
        class={`coach-card ${view.waiting && !list ? 'waiting' : ''}`}
        style={{ left: spot.left, top: spot.top, width: spot.width ?? W }}
        role="dialog"
        aria-modal="true"
        aria-labelledby="coach-title"
        aria-describedby="coach-desc"
        data-stop={stop.key}
        data-expect={stop.target && !stop.optional ? '1' : undefined}
      >
        {spot.side && <span class={`coach-caret ${spot.side}`} style={spot.side === 'top' || spot.side === 'bottom' ? { left: spot.caret } : { top: spot.caret }} />}
        <div class="row gap3">
          <span class="coach-icon">
            <Icon name={list ? 'list' : stop.icon} size="sm" />
          </span>
          <div class="col grow" style={{ gap: 0, minWidth: 0 }}>
            <span class="xs muted">
              {t('tut.chapter', { n: stop.ch + 1, total: CHAPTERS.length })} · {t('tut.progress', { n: i + 1, total: STOPS.length })}
            </span>
            <b class="small">{t('tut.ch.' + chapter.key)}</b>
          </div>
          <Button size="sm" kind="ghost" icon="list" selected={list} onClick={() => setList(!list)}>
            {t('tut.chapters')}
          </Button>
        </div>
        <div class="coach-bar" role="progressbar" aria-label={t('tut.kicker')} aria-valuemin={1} aria-valuemax={STOPS.length} aria-valuenow={i + 1}>
          <span style={{ width: `${((i + 1) / STOPS.length) * 100}%` }} />
        </div>
        <div class="coach-body" key={list ? 'list' : i}>
          {list ? (
            <>
              <h3 id="coach-title">{t('tut.chapters')}</h3>
              <p id="coach-desc" class="muted small">
                {t('tut.chapters.d')}
              </p>
              <div class="coach-chapters">
                {CHAPTERS.map((c, k) => (
                  <button type="button" class={k === stop.ch ? 'on' : ''} aria-current={k === stop.ch ? 'step' : undefined} onClick={() => go(FIRST[k])}>
                    <span class="num faint">{k + 1}</span>
                    <Icon name={c.icon} size="sm" />
                    <span class="grow">{t('tut.ch.' + c.key)}</span>
                    <span class="xs faint">{t('tut.stops', { n: c.stops.length })}</span>
                  </button>
                ))}
              </div>
            </>
          ) : (
            <>
              <div class="col gap2">
                <h3 id="coach-title">{t(stop.key)}</h3>
                <p id="coach-desc" class="muted small">
                  {t(stop.key + '.d')}
                </p>
              </div>
              {stop.flow && (
                <div class="col gap2">
                  <span class="section-title">{t('tut.flow')}</span>
                  <ol class="coach-flow">
                    {FLOW.map((f) => (
                      <li>
                        <Icon name={f.icon} size="sm" />
                        <span>{t(f.k)}</span>
                      </li>
                    ))}
                  </ol>
                </div>
              )}
              {stop.legend && (
                <ul class="coach-legend">
                  {stop.legend.map((l) => (
                    <li>
                      {l.badge ? (
                        <>
                          <Badge kind={l.badge}>{label(l)}</Badge>
                          <span>{t(l.key)}</span>
                        </>
                      ) : (
                        <>
                          <span class="dot" style={{ color: TONE[l.tone || 'ok'] }} />
                          <span>
                            <b>{label(l)}</b> <span class="muted">{t(l.key)}</span>
                          </span>
                        </>
                      )}
                    </li>
                  ))}
                </ul>
              )}
              {stop.points && (
                <ul class="coach-points">
                  {stop.points.map((x) => (
                    <li>
                      <Icon name={x.icon} size="sm" />
                      <span>{t(x.key)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
        <div class="row gap2 coach-foot">
          <Button kind="ghost" size="sm" onClick={done}>
            {t('tour.skip')}
          </Button>
          <span class="spacer" />
          <span class="row gap1" aria-hidden="true">
            {inChapter.map((s) => (
              <span class="dot" style={{ color: s === stop ? 'var(--accent)' : 'var(--border-2)' }} />
            ))}
          </span>
          <span class="spacer" />
          {i > 0 && <Button size="sm" icon="back" label={t('ui.back')} tip={t('ui.back')} onClick={() => go(i - 1)} />}
          <Button kind="primary" size="sm" trail={last ? undefined : 'next'} onClick={() => (last ? done() : go(i + 1))}>
            {last ? t('tour.start') : t('ui.next')}
          </Button>
        </div>
      </div>
    </div>
  );
}
