export type ApiOk<T> = {
  ok: boolean;
  data?: T;
  message?: string;
};

export type User = {
  id: number;
  username: string;
  role: string;
};

export type LoginData = {
  token: string;
  user: User;
};

export type DashboardStats = {
  tgAccounts: number;
  tgActive: number;
  tgExpired: number;
  tasksQueued: number;
  tasksRunning: number;
  tasksFailed: number;
  tasksPaused: number;
  tasksDone: number;
  media: number;
  downloadDir: string;
  diskUsed: number;
  diskAvailable: number;
  diskTotal: number;
  tgConfigured: boolean;
  watchEnabled: boolean;
  dialogCount?: number;
  savedCount?: number;
  savedDownloaded?: number;
  dialogsSyncedAt?: string;
  savedSyncedAt?: string;
  proxyConfigured?: boolean;
  threads?: number;
  concurrency?: number;
};

export type Settings = {
  bind: string;
  downloadDir: string;
  webDir: string;
  dbPath: string;
  sessionDir: string;
  appId: number;
  appHashSet: boolean;
  usingDesktopPreset?: boolean;
  threads: number;
  concurrency: number;
  skipSame: boolean;
  groupAlbum: boolean;
  rewriteExt: boolean;
  takeout: boolean;
  noImage: boolean;
  watchIntervalMinutes: number;
  template: string;
  proxy: string;
};

export type TgUser = {
  id: number;
  username: string;
  firstName: string;
  phone: string;
};

export type TgStatus = {
  configured: boolean;
  loggedIn: boolean;
  status: string;
  message: string;
  user?: TgUser | null;
  usingDesktopPreset?: boolean;
};

export type TGSummary = {
  dialogCount: number;
  savedCount: number;
  savedDownloaded?: number;
  dialogsSyncedAt?: string;
  savedSyncedAt?: string;
};

export type ItemCounts = {
  pending: number;
  downloading: number;
  done: number;
  skipped: number;
  failed: number;
};

export type ChannelRow = {
  chatId: number;
  title: string;
  username: string;
  kind: string;
  messageCount: number;
  downloadedCount: number;
  lastMessageId: number;
  lastDownloadedMessageId: number;
  scanCursor?: number;
  caughtUp?: boolean;
  status?: string;
  syncedAt: string;
};

export type ChannelDownloadTask = {
  id: number;
  title: string;
  status: string;
  progressDone?: number;
  progressTotal?: number;
  doneFiles?: number;
  totalFiles?: number;
  itemCounts?: ItemCounts;
  error?: string;
  createdAt?: string;
};

export type ChannelDownloadInfo = {
  chatId: number;
  title: string;
  username: string;
  kind: string;
  downloadedCount: number;
  lastMessageId: number;
  scanCursor: number;
  caughtUp: boolean;
  failedCount: number;
  status: string;
  syncedAt?: string;
  activeTask?: ChannelDownloadTask | null;
  recentBatches?: ChannelDownloadTask[];
  defaultBatchSize?: number;
};

export type TaskItemRow = {
  id: number;
  taskId: number;
  chatId: number;
  messageId: number;
  fileName: string;
  size: number;
  status: string;
  localPath: string;
  error: string;
  chatTitle?: string;
  mediaKind?: string;
};

export type LibraryItem = {
  id: number;
  chatId: number;
  messageId: number;
  fileName: string;
  size: number;
  mime: string;
  mediaKind: string;
  localPath: string;
};

export type WatchRow = {
  id: number;
  chatId: number;
  chatTitle: string;
  isFavorites?: boolean;
  enabled?: boolean;
  lastMessageId: number;
  cursorMessageId?: number;
  downloadedCount: number;
  lastRunAt?: string;
  nextRunAt?: string;
  createdAt?: string;
};

export type WatchCandidate = {
  chatId: number;
  title: string;
  kind: string;
  username?: string;
};
