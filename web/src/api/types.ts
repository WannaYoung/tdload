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
};

export type Settings = {
  bind: string;
  downloadDir: string;
  webDir: string;
  dbPath: string;
  sessionDir: string;
  appId: number;
  appHashSet: boolean;
  threads: number;
  concurrency: number;
  skipSame: boolean;
  groupAlbum: boolean;
  rewriteExt: boolean;
  takeout: boolean;
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

export type ChannelRow = {
  chatId: number;
  title: string;
  username: string;
  kind: string;
  messageCount: number;
  downloadedCount: number;
  lastMessageId: number;
  lastDownloadedMessageId: number;
  syncedAt: string;
};

export type ItemCounts = {
  pending: number;
  downloading: number;
  done: number;
  skipped: number;
  failed: number;
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
