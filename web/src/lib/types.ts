import type { Quantity, Timestamp } from './format';

export type Mode = 'portable' | 'temporary';

export type ProfileEntry = {
  id: string;
  username: string;
  avatarType?: string;
  avatarHash?: string;
  hasPassword: boolean;
  storageMode: 'usb_cloud' | 'usb_only' | 'cloud_only';
  provider?: string;
  created: string;
  color: number;
  avatar: boolean;
};

export type AccountView = { id: string; name: string; remembered: boolean; profiles: ProfileEntry[] | null; maxReached: boolean };

export type SyncStatus = {
  state: 'local' | 'synced' | 'pending' | 'syncing' | 'offline' | 'reconnect' | 'not_connected' | 'error' | 'conflict';
  pending: number;
  conflicts: number;
  lastSync?: string;
  lastError?: string;
  storageMode: string;
  provider?: string;
  transport?: string;
  account?: { displayName?: string; maskedEmail?: string };
};

export type ExportPrefs = { preset?: string; formats?: Record<string, string>; figure?: Record<string, unknown> | null; csvDecimal?: string; destination?: string };

export type ProfileSettings = {
  language?: string;
  theme?: string;
  onboarding?: Record<string, boolean>;
  whatsNewSeen?: string;
  export?: ExportPrefs;
  decimal?: string;
};

export type Choice = {
  version: string;
  build: number;
  channel: string;
  date: string;
  notes?: Record<string, string>;
  schema: number;
  security?: boolean;
  current: boolean;
  latest: boolean;
  newer: boolean;
  compatible: boolean;
  problem?: string;
  migration?: boolean;
};

export type UpdateStatus = {
  configured: boolean;
  portable: boolean;
  auto: boolean;
  locked: boolean;
  lockedAt?: string;
  current: string;
  channel: string;
  buildDate?: string;
  latest?: string;
  newer: boolean;
  staged?: string;
  applying?: boolean;
  lastCheck?: string;
  error?: string;
  choices?: Choice[];
  dataSchema: number;
  platform: string;
  online: boolean;
  manifestError?: string;
};

export type MobileDevice = { id: string; label: string; paired: string; lastSeen: string };
export type MobileInfo = { active: boolean; url?: string; qr?: string; address?: string; expires?: string; devices: MobileDevice[] };
export type Presentation = { active: boolean; graphs: string[] | null; index: number; noLegend?: boolean; hidden?: string[] | null };
export type Activity = { id: string; kind: string; started: string };
export type ProviderOption = { id: string; api: boolean; folder: string; hasFolder: boolean; configured: boolean };
export type Recovery = { account: string; created: string; source: string };

export type StateView = {
  version: string;
  channel: string;
  buildDate?: string;
  mode: Mode;
  lang: string;
  systemLang: string;
  languages: string[];
  decimal: string;
  settings: { language?: string; turbo: boolean; theme?: string; lastAccount?: string; onboarded?: boolean };
  crashed: boolean;
  exiting: boolean;
  accounts: { id: string; created: string; remembered: boolean }[];
  account?: AccountView;
  profile?: ProfileEntry & { settings: ProfileSettings; sync: SyncStatus };
  storageModes: string[];
  providers: ProviderOption[];
  recoveries?: Recovery[] | null;
  update: UpdateStatus;
  mobile: MobileInfo;
  present: Presentation;
  activities: Activity[];
  exports: string;
  science: Record<string, string>;
};

// ---- LIGHTSCATTERING ----

export type SourceInfo = {vendor:string;container:string;support:string;scientificValidation:string;details?:unknown};
export type SourceFile = {
  sourceInfo?: SourceInfo;
  importSelection?: ImportSelection;
  id: string;
  blobId: string;
  name: string;
  format: string;
  size: number;
  sha256: string;
  importedAt: string;
  module: string;
  parser: string;
  spec: string;
  status: 'parsed' | 'partial' | 'failed';
  measurements: string[] | null;
  encoding?: string;
  delimiter?: string;
  decimal?: string;
  warnings?: string[];
  error?: string;
  experiment?: string;
  group?: string;
  tags?: string[];
  notes?: string;
  origin: string;
  trashed?: boolean;
};

export type MeasurementSummary = {
  sourceSheet?: string; sourceRange?: string;
  id: string;
  fileId: string;
  index: number;
  sampleId?: string;
  experimentalUnitId?: string;
  measuredAt?: Timestamp;
  params: Record<string, Quantity>;
  weightings?: string[];
  bins: number;
  distributionId?: string;
  label?: string;
  replicate?: number;
  condition?: string;
  experiment?: string;
  group?: string;
  tags?: string[];
  notes?: string;
};

export type FileView = SourceFile & { items: MeasurementSummary[] | null };

export type Field = { key?: string; label: string; text: string; unit?: string; num?: number; line: number };
export type Column = { key?: string; label: string; unit?: string; values: number[]; raw: string[] };
export type Distribution = { id?: string; method?: string; format?: string; columns: Column[]; firstLine: number; lastLine: number; sourceLines?: number[]; auxiliary?: Field[] };
export type Measurement = MeasurementSummary & {
  fields: Field[] | null;
  distribution?: Distribution;
  distributions?: Distribution[];
  distributionSelectedAt?: string;
  parser: string;
  spec: string;
};

export type ImportResult = {
  name: string;
  status: 'parsed' | 'partial' | 'failed' | 'duplicate';
  fileId?: string;
  existing?: string;
  measurements: number;
  recognized?: string[];
  warnings?: string[];
  error?: string;
  needsDate?: boolean;
};

export type SheetSelection = { name: string; range?: string };
export type ImportSelection = { sheets: SheetSelection[] };
export type ImportProfile = { id: string; schema: number; name: string; format: string; selection?: ImportSelection; parser: string; spec: string; createdAt: string };
export type ImportInspection = {
  tabular?: { schema: number; format: string; delimiter?: string; truncated: boolean; error?: string; tables?: { sheet?: string; range?: string; rowCount: number; columnCount: number; columns?: { index: number; label: string }[]; rows?: { line: number; lastLine: number; cells: { column: number; line: number; byteColumn?: number; address?: string; value: string }[] }[] }[] };
  selection?: ImportSelection;
  name: string; sha256: string; format: string; module?: string;
  measurements: number; previewTruncated: boolean; needsDate: boolean;
  existing?: string; accountId: string; profileId: string; receipt?: string;
  result: { sourceInfo?:SourceInfo; sheets?: { name: string; rows: number; columns: number; headers?: string[]; selected: boolean; compatible: boolean }[]; status: 'parsed' | 'partial' | 'failed'; measurements: Measurement[];
    encoding: string; delimiter?: string; decimal?: string; recognized: string[];
    warnings?: string[]; error?: string; parser: string; spec: string };
};

export type SeriesStyle = { label?: string; color?: string; hidden?: boolean };
export type Visual = { legend: boolean; metadata: boolean; grid: boolean; points: boolean; lineWidth: number; fontScale: number; view?: { XMin: number; XMax: number; YMin: number; YMax: number } | null };

export type GraphDef = {
  schema?: number;
  id?: string;
  title: string;
  kind: 'dls_distribution' | 'parameter_time' | 'dls_by_time' | 'statistical_groups';
  analysisId?: string;
  errorBars?: 'sd'|'sem'|'ci';
  annotations?: string[];
  annotationStyle?: 'exact'|'stars';
  measurements: string[];
  distributions?: Record<string, string>;
  weighting?: string;
  xScale?: string;
  cycleId?: string;
  param?: string;
  series?: Record<string, SeriesStyle>;
  visual: Visual;
  tags?: string[];
  notes?: string;
  created?: string;
  updated?: string;
  trashed?: boolean;
};

export type Op = {
  k: string;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
  x2?: number;
  y2?: number;
  r?: number;
  pts?: number[];
  fill?: string;
  stroke?: string;
  sw?: number;
  dash?: number[];
  op?: number;
  t?: string;
  size?: number;
  font?: string;
  anchor?: string;
  rot?: number;
  role?: string;
};

export type Figure = {
  width: number;
  height: number;
  background?: string;
  ops: Op[];
  hits?: { s: number; i: number; x: number; y: number }[];
  map: { plot: { X: number; Y: number; W: number; H: number }; xMin: number; xMax: number; yMin: number; yMax: number; xScale: string; yScale: string };
  title: string;
  desc?: string;
};

export type Series = {
  id: string;
  label: string;
  measurementId?: string;
  x: number[];
  y: number[];
  err?: number[];
  n?: number[];
  kind: string;
  meta?: Record<string, string>;
  hidden?: boolean;
  color?: string;
};

export type CycleConfig = {
  name: string;
  experiment?: string;
  sampleId?: string;
  start: string;
  startTzKnown: boolean;
  interval: number;
  unit: 'hours' | 'days' | 'weeks' | 'months' | 'years';
  duration: number;
  replicates: number;
  params: string[];
};

export type CycleDoc = {
  schema?: number;
  id?: string;
  config: CycleConfig;
  measurements: string[];
  overrides?: Record<string, { point: number; replicate?: number; at: string }>;
  tags?: string[];
  notes?: string;
  created?: string;
  updated?: string;
  trashed?: boolean;
};

export type Point = { index: number; offset: number; unit: string; target: string };
export type Assignment = { measurementId: string; point: number; status: string; reason?: string; replicate?: number };
export type Summary = { n: number; values: number[] | null; mean?: number; sd?: number; min?: number; max?: number; definition: string };
export type PointResult = { point: number; offset: number; target: string; missing: boolean; summary: Summary; sources: string[] | null };

export type CycleView = CycleDoc & {
  points: Point[] | null;
  assignments: Assignment[] | null;
  items: MeasurementSummary[] | null;
  series: Record<string, PointResult[]> | null;
  files: string[] | null;
};

export type Relations = {
  files: Record<string, { graphs: string[] | null; cycles: string[] | null; analyses?:string[] | null }>;
  graphs: Record<string, { files: string[] | null; cycle?: string }>;
  cycles: Record<string, { files: string[] | null; graphs: string[] | null; analyses?:string[] | null }>;
};
