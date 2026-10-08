// The interface's icon set: one family (outline, 1.75 stroke, 24-unit grid),
// sized by the design system (.icon, .icon.sm, .icon.lg).
import type { ComponentType } from 'preact';
import {
  Activity, Archive, ArrowLeft, ArrowRight, ArrowUpRight, Ban, BookOpen, Calendar, CalendarClock, Camera, ChartLine, ChartSpline,
  Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert, CircleCheck, CircleDot, CircleQuestionMark, Clock, ClockArrowDown, Cloud,
  CloudCheck, CloudOff, CloudUpload, Command, Copy, Database, Download, Ellipsis, Eye, EyeOff, FileBraces, FileSpreadsheet,
  FileText, FileUp, Files, FlaskConical, Folder, FolderOpen, Funnel, Gauge, Gift, Globe, HardDrive, Hash, Image, ImageUp, Info,
  Keyboard, KeyRound, Laptop, Layers, LayoutGrid, LifeBuoy, Link2, List, ListChecks, Lock, LockOpen, LogOut, Maximize2, Megaphone,
  Microscope, Minimize2, Monitor, MonitorSmartphone, Moon, Package, Palette, PanelRight, Pause, Pencil, Pin, Play, Plus, Power,
  Presentation, QrCode, RefreshCw, Repeat2, Rocket, RotateCcw, Ruler, Save, ScanLine, Search, Server, Settings, Shield,
  ShieldCheck, Sigma, SkipBack, SkipForward, SlidersHorizontal, Smartphone, Sparkles, Star, Sun, Table2, Tag, Trash, TriangleAlert,
  Type, Undo2, Upload, Usb, User, Users, Wifi, WifiOff, Workflow, X, ZoomIn,
} from 'lucide-preact';

const set = {
  activity: Activity, archive: Archive, back: ArrowLeft, next: ArrowRight, external: ArrowUpRight, ban: Ban, book: BookOpen,
  calendar: Calendar, calendarClock: CalendarClock, camera: Camera, chart: ChartLine, spline: ChartSpline, check: Check,
  down: ChevronDown, left: ChevronLeft, right: ChevronRight, alert: CircleAlert, ok: CircleCheck, dot: CircleDot,
  help: CircleQuestionMark, clock: Clock, history: ClockArrowDown, cloud: Cloud, cloudOk: CloudCheck, cloudOff: CloudOff,
  cloudUp: CloudUpload, command: Command, copy: Copy, database: Database, download: Download, more: Ellipsis, eye: Eye,
  eyeOff: EyeOff, json: FileBraces, sheet: FileSpreadsheet, file: FileText, fileUp: FileUp, files: Files, flask: FlaskConical,
  folder: Folder, folderOpen: FolderOpen, filter: Funnel, gauge: Gauge, gift: Gift, globe: Globe, drive: HardDrive, hash: Hash,
  image: Image, imageUp: ImageUp, info: Info, keyboard: Keyboard, key: KeyRound, laptop: Laptop, layers: Layers, grid: LayoutGrid,
  lifebuoy: LifeBuoy, link: Link2, list: List, listChecks: ListChecks, lock: Lock, unlock: LockOpen, logout: LogOut,
  maximize: Maximize2, megaphone: Megaphone, microscope: Microscope, minimize: Minimize2, monitor: Monitor,
  devices: MonitorSmartphone, moon: Moon, package: Package, palette: Palette, panel: PanelRight, pause: Pause, pencil: Pencil,
  pin: Pin, play: Play, plus: Plus, power: Power, present: Presentation, qr: QrCode, refresh: RefreshCw, cycle: Repeat2,
  rocket: Rocket, reset: RotateCcw, ruler: Ruler, save: Save, scan: ScanLine, search: Search, server: Server, settings: Settings,
  shield: Shield, shieldOk: ShieldCheck, sigma: Sigma, prev: SkipBack, skip: SkipForward, sliders: SlidersHorizontal,
  phone: Smartphone, sparkles: Sparkles, star: Star, sun: Sun, table: Table2, tag: Tag, trash: Trash, warning: TriangleAlert,
  type: Type, undo: Undo2, upload: Upload, usb: Usb, user: User, users: Users, wifi: Wifi, wifiOff: WifiOff, workflow: Workflow,
  x: X, zoom: ZoomIn,
} satisfies Record<string, ComponentType<any>>;

export type IconName = keyof typeof set;

export function Icon({ name, size, class: cls, title }: { name: IconName; size?: 'sm' | 'lg'; class?: string; title?: string }) {
  const C = set[name];
  return <C class={`icon ${size || ''} ${cls || ''}`} strokeWidth={1.75} aria-hidden={title ? undefined : 'true'} aria-label={title} />;
}
