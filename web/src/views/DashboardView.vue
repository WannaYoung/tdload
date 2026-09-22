<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { NAlert, NButton, NIcon, NProgress, NSpin, NTag, useMessage } from "naive-ui";
import {
  AddOutline,
  AlertCircleOutline,
  CheckmarkCircleOutline,
  CloudDownloadOutline,
  EllipsisHorizontalOutline,
  FolderOpenOutline,
  HeartOutline,
  HourglassOutline,
  PaperPlaneOutline,
  PeopleOutline,
  PlayCircleOutline,
  RefreshOutline,
  ServerOutline,
} from "@vicons/ionicons5";
import { api, openEventSource } from "../api/http";
import type { DashboardStats } from "../api/types";
import { useAuthStore } from "../stores/auth";

type RecentTask = {
  id: number;
  title: string;
  status: string;
  source?: string;
  kind?: string;
  progressDone: number;
  progressTotal: number;
  error: string;
  createdAt: string;
};

const router = useRouter();
const auth = useAuthStore();
const message = useMessage();
const loading = ref(true);
const error = ref("");
const stats = ref<DashboardStats | null>(null);
const tasks = ref<RecentTask[]>([]);

const greeting = computed(() => {
  const hour = new Date().getHours();
  if (hour < 6) return "夜深了";
  if (hour < 12) return "上午好";
  if (hour < 18) return "下午好";
  return "晚上好";
});

const displayName = computed(() => auth.user?.username || "管理员");

const subtitle = computed(() => {
  const s = stats.value;
  if (!s) return `${greeting.value}，${displayName.value}`;
  if (!s.tgConfigured) return "请先配置 Telegram API 凭证";
  if (s.tgExpired > 0 || (s.tgAccounts > 0 && !s.tgActive)) return "Telegram 会话已失效，请重新登录";
  if (!s.tgActive) return "尚未登录 Telegram，登录后可同步频道与下载";
  if (s.tasksRunning > 0) return `当前有 ${s.tasksRunning} 个任务正在下载`;
  if (s.tasksQueued > 0) return `队列中还有 ${s.tasksQueued} 个任务等待处理`;
  if (s.tasksFailed > 0) return `${s.tasksFailed} 个任务失败，可在任务页重试`;
  return "系统运行正常";
});

const kpis = computed(() => {
  const s = stats.value;
  return [
    {
      key: "running",
      label: "下载中",
      value: s?.tasksRunning ?? 0,
      tone: "tone-running",
      icon: PlayCircleOutline,
    },
    {
      key: "queued",
      label: "排队",
      value: s?.tasksQueued ?? 0,
      tone: "tone-queued",
      icon: HourglassOutline,
    },
    {
      key: "done",
      label: "已完成",
      value: s?.tasksDone ?? 0,
      tone: "tone-done",
      icon: CheckmarkCircleOutline,
    },
    {
      key: "failed",
      label: "失败",
      value: s?.tasksFailed ?? 0,
      tone: "tone-fail",
      icon: AlertCircleOutline,
    },
  ];
});

const diskPercent = computed(() => {
  const s = stats.value;
  if (!s || !s.diskTotal) return 0;
  const usedFs = Math.max(0, s.diskTotal - s.diskAvailable);
  return Math.min(100, Math.round((usedFs / s.diskTotal) * 100));
});

const diskHint = computed(() => {
  const s = stats.value;
  if (!s) return "";
  if (!s.diskTotal) return s.downloadDir || "";
  return `已用 ${formatSize(s.diskUsed)} · 可用 ${formatSize(s.diskAvailable)}`;
});

const recentTasks = computed(() => tasks.value.slice(0, 8));

const statusMeta: Record<string, { label: string; color: string }> = {
  running: { label: "下载中", color: "#f9a8d4" },
  queued: { label: "排队", color: "#93c5fd" },
  done: { label: "完成", color: "#86efac" },
  failed: { label: "失败", color: "#fca5a5" },
  paused: { label: "暂停", color: "#fcd34d" },
  cancelled: { label: "取消", color: "rgba(255,255,255,0.45)" },
};

const kindLabel: Record<string, string> = {
  message: "消息",
  saved: "收藏",
  channel: "频道",
  url: "消息",
  saved_all: "收藏",
  chat_batch: "频道",
  chat_continue: "频道",
};

function formatSize(n: number): string {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function fromNow(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const d = Date.now() - t;
  if (d < 60_000) return "刚刚";
  if (d < 3_600_000) return `${Math.floor(d / 60_000)} 分钟前`;
  if (d < 86_400_000) return `${Math.floor(d / 3_600_000)} 小时前`;
  if (d < 7 * 86_400_000) return `${Math.floor(d / 86_400_000)} 天前`;
  return new Date(t).toLocaleDateString("zh-CN");
}

function go(name: string) {
  void router.push({ name });
}

function taskKind(t: RecentTask) {
  if (t.kind) return kindLabel[t.kind] || t.kind;
  if (t.source) return kindLabel[t.source] || t.source;
  return "任务";
}

function taskPct(t: RecentTask) {
  if (!t.progressTotal) return t.status === "done" ? 100 : 0;
  return Math.min(100, Math.round((t.progressDone / t.progressTotal) * 100));
}

async function refresh(silent = false) {
  if (!silent) loading.value = true;
  try {
    const [dash, page] = await Promise.all([
      api<DashboardStats>("/api/dashboard"),
      api<{ items: RecentTask[] }>("/api/tasks?page=1&pageSize=8").catch(() => ({ items: [] as RecentTask[] })),
    ]);
    stats.value = dash;
    tasks.value = page.items || [];
    error.value = "";
  } catch (err) {
    if (!silent) {
      error.value = err instanceof Error ? err.message : "加载失败";
    }
  } finally {
    if (!silent) loading.value = false;
  }
}

let es: EventSource | null = null;
let pollTimer: number | null = null;
let refreshTimer: number | null = null;

function scheduleRefresh() {
  if (refreshTimer != null) window.clearTimeout(refreshTimer);
  refreshTimer = window.setTimeout(() => {
    refreshTimer = null;
    void refresh(true);
  }, 600);
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as {
      type?: string;
      taskId?: number;
      chatId?: number;
      done?: number;
      total?: number;
      status?: string;
      title?: string;
      error?: string;
    };
    if (ev.type === "watch_hit") {
      const range =
        ev.done != null && ev.total != null ? `#${ev.done}–#${ev.total}` : "";
      const count = ev.error ? `（${ev.error} 条）` : "";
      message.info(`监听入队：${ev.title || ev.chatId || "对话"} ${range}${count}`);
      scheduleRefresh();
      return;
    }
    const idx = tasks.value.findIndex((t) => t.id === ev.taskId);
    if (idx < 0) {
      if (ev.status || ev.type === "task_progress") scheduleRefresh();
      return;
    }
    const row = tasks.value[idx];
    const prev = row.status;
    tasks.value[idx] = {
      ...row,
      status: ev.status || row.status,
      progressDone: ev.done ?? row.progressDone,
      progressTotal: ev.total ?? row.progressTotal,
    };
    if (prev !== tasks.value[idx].status) scheduleRefresh();
  } catch {
    /* ignore */
  }
}

onMounted(async () => {
  await refresh(false);
  try {
    es = await openEventSource("/api/events");
    es.onmessage = (e) => applyEvent(e.data);
  } catch {
    /* poll */
  }
  pollTimer = window.setInterval(() => void refresh(true), 12000);
});

onUnmounted(() => {
  es?.close();
  if (pollTimer != null) window.clearInterval(pollTimer);
  if (refreshTimer != null) window.clearTimeout(refreshTimer);
});
</script>

<template>
  <n-spin class="page dash" :show="loading && !stats">
    <n-alert v-if="error" type="error" style="margin-bottom: 16px">{{ error }}</n-alert>

    <header v-if="stats" class="page-head">
      <div>
        <h1>概览</h1>
        <p class="sub">{{ greeting }}，{{ displayName }} · {{ subtitle }}</p>
      </div>
      <div class="page-head-actions">
        <n-button quaternary @click="refresh(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
        <n-button @click="go('telegram')">
          <template #icon>
            <n-icon :component="PaperPlaneOutline" />
          </template>
          Telegram
        </n-button>
        <n-button type="primary" @click="go('tasks')">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          新建任务
        </n-button>
      </div>
    </header>

    <n-alert
      v-if="stats && !stats.tgActive"
      type="info"
      title="尚未登录 Telegram"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>登录后可同步频道、收藏，并开始下载任务。</span>
        <n-button size="small" type="primary" @click="go('telegram')">去登录</n-button>
      </div>
    </n-alert>
    <n-alert
      v-else-if="stats && stats.tgExpired > 0"
      type="warning"
      title="Telegram 会话已失效"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>请重新登录后再同步或下载。</span>
        <n-button size="small" @click="go('telegram')">去处理</n-button>
      </div>
    </n-alert>
    <n-alert
      v-else-if="stats && stats.tasksFailed > 0"
      type="warning"
      :title="`${stats.tasksFailed} 个任务失败`"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>可在任务页查看原因并重试。</span>
        <n-button size="small" @click="go('tasks')">查看任务</n-button>
      </div>
    </n-alert>

    <section v-if="stats" class="kpi-grid">
      <article v-for="card in kpis" :key="card.key" class="kpi" :class="card.tone">
        <div class="kpi-icon">
          <n-icon :component="card.icon" :size="22" />
        </div>
        <div>
          <div class="kpi-label">{{ card.label }}</div>
          <div class="kpi-value">{{ card.value }}</div>
        </div>
      </article>
    </section>

    <div v-if="stats" class="main-grid">
      <section class="panel">
        <header class="panel-head">
          <h2>资源概况</h2>
        </header>
        <div class="resource-list">
          <button class="resource" type="button" @click="go('telegram')">
            <div class="resource-icon tone-tg">
              <n-icon :component="PaperPlaneOutline" :size="20" />
            </div>
            <div class="resource-body">
              <div class="resource-top">
                <span>Telegram</span>
                <strong>{{ stats.tgActive ? "已登录" : "未登录" }}</strong>
              </div>
              <div class="resource-hint">
                <n-tag size="tiny" :type="stats.tgConfigured ? 'success' : 'warning'">
                  API {{ stats.tgConfigured ? "已配置" : "未配置" }}
                </n-tag>
                <n-tag v-if="stats.proxyConfigured" size="tiny" type="info">代理开</n-tag>
              </div>
            </div>
          </button>

          <button class="resource" type="button" @click="go('channels')">
            <div class="resource-icon tone-ch">
              <n-icon :component="PeopleOutline" :size="20" />
            </div>
            <div class="resource-body">
              <div class="resource-top">
                <span>频道 / 群</span>
                <strong>{{ stats.dialogCount ?? 0 }}</strong>
              </div>
              <div class="resource-hint">
                {{ stats.dialogsSyncedAt ? `同步于 ${fromNow(stats.dialogsSyncedAt)}` : "尚未同步，请到 Telegram 页刷新" }}
              </div>
            </div>
          </button>

          <button class="resource" type="button" @click="go('tasks')">
            <div class="resource-icon tone-fav">
              <n-icon :component="HeartOutline" :size="20" />
            </div>
            <div class="resource-body">
              <div class="resource-top">
                <span>收藏</span>
                <strong>{{ stats.savedDownloaded ?? 0 }}/{{ stats.savedCount ?? 0 }}</strong>
              </div>
              <div class="resource-hint">已下载 / 缓存条目</div>
            </div>
          </button>

          <button class="resource" type="button" @click="go('library')">
            <div class="resource-icon tone-lib">
              <n-icon :component="FolderOpenOutline" :size="20" />
            </div>
            <div class="resource-body">
              <div class="resource-top">
                <span>资源库</span>
                <strong>{{ stats.media }}</strong>
              </div>
              <div class="resource-hint">已索引媒体文件</div>
            </div>
          </button>

          <div class="resource static">
            <div class="resource-icon tone-disk">
              <n-icon :component="ServerOutline" :size="20" />
            </div>
            <div class="resource-body">
              <div class="resource-top">
                <span>磁盘</span>
                <strong>{{ stats.diskTotal ? `${diskPercent}%` : "—" }}</strong>
              </div>
              <n-progress
                v-if="stats.diskTotal"
                type="line"
                :percentage="diskPercent"
                :height="6"
                :show-indicator="false"
                :status="diskPercent > 90 ? 'error' : diskPercent > 75 ? 'warning' : 'success'"
              />
              <div class="resource-hint path">{{ diskHint }}</div>
              <div v-if="stats.downloadDir" class="resource-hint path">{{ stats.downloadDir }}</div>
            </div>
          </div>
        </div>
      </section>

      <section class="panel">
        <header class="panel-head">
          <h2>最近任务</h2>
          <n-button text type="primary" title="更多" @click="go('tasks')">
            <template #icon>
              <n-icon :component="EllipsisHorizontalOutline" />
            </template>
          </n-button>
        </header>
        <ul v-if="recentTasks.length" class="task-list">
          <li v-for="row in recentTasks" :key="row.id" class="task-row" @click="go('tasks')">
            <div class="task-main">
              <n-tag size="tiny" :bordered="false" class="task-kind">{{ taskKind(row) }}</n-tag>
              <div class="task-text">
                <span class="task-title">{{ row.title || `#${row.id}` }}</span>
                <n-progress
                  v-if="row.status === 'running' || row.progressTotal > 0"
                  type="line"
                  :percentage="taskPct(row)"
                  :height="4"
                  :show-indicator="false"
                  :processing="row.status === 'running'"
                />
              </div>
            </div>
            <div class="task-meta">
              <span class="task-time">{{ fromNow(row.createdAt) }}</span>
              <span
                class="status-tag"
                :style="{ color: statusMeta[row.status]?.color || 'rgba(255,255,255,0.55)' }"
              >
                {{ statusMeta[row.status]?.label || row.status }}
              </span>
            </div>
          </li>
        </ul>
        <div v-else class="empty">
          <n-icon :component="CloudDownloadOutline" :size="28" />
          <p>还没有下载任务</p>
          <n-button size="small" type="primary" @click="go('tasks')">创建第一个任务</n-button>
        </div>
      </section>
    </div>
  </n-spin>
</template>

<style scoped>
.dash {
  width: 100%;
  min-width: 0;
  max-width: 1200px;
}
.page-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 20px;
}
.page-head h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: 0.02em;
}
.sub {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.5);
  font-size: 13px;
  line-height: 1.5;
}
.alert-body {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.page-head-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.kpi {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  padding: 16px;
  border-radius: 12px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.045), rgba(255, 255, 255, 0.02));
  border: 1px solid rgba(255, 255, 255, 0.07);
  min-width: 0;
}
.kpi-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.kpi-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
.kpi-value {
  margin-top: 2px;
  font-size: 26px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.03em;
  line-height: 1.2;
}
.tone-running .kpi-icon {
  background: rgba(244, 114, 182, 0.16);
  color: #f9a8d4;
}
.tone-queued .kpi-icon {
  background: rgba(96, 165, 250, 0.14);
  color: #93c5fd;
}
.tone-done .kpi-icon {
  background: rgba(74, 222, 128, 0.14);
  color: #86efac;
}
.tone-fail .kpi-icon {
  background: rgba(248, 113, 113, 0.14);
  color: #fca5a5;
}
.main-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
  gap: 12px;
}
.panel {
  min-width: 0;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid rgba(255, 255, 255, 0.07);
  padding: 16px 16px 8px;
}
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 4px;
}
.panel-head h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.resource-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.resource {
  display: flex;
  gap: 12px;
  align-items: center;
  width: 100%;
  padding: 12px 8px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.resource:hover {
  background: rgba(255, 255, 255, 0.04);
}
.resource-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.tone-tg {
  background: rgba(56, 189, 248, 0.14);
  color: #7dd3fc;
}
.tone-ch {
  background: rgba(167, 139, 250, 0.14);
  color: #c4b5fd;
}
.tone-fav {
  background: rgba(244, 114, 182, 0.14);
  color: #f9a8d4;
}
.tone-lib {
  background: rgba(251, 191, 36, 0.14);
  color: #fcd34d;
}
.tone-disk {
  background: rgba(94, 234, 212, 0.14);
  color: #5eead4;
}
.resource.static {
  cursor: default;
}
.resource.static:hover {
  background: transparent;
}
.resource-body {
  flex: 1;
  min-width: 0;
}
.resource-top {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
}
.resource-top strong {
  font-size: 18px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  color: #fff;
}
.resource-hint {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin-top: 4px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.38);
}
.resource-hint.path {
  word-break: break-all;
}
.task-list {
  list-style: none;
  margin: 0;
  padding: 0 0 8px;
}
.task-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 8px;
  border-radius: 8px;
  cursor: pointer;
}
.task-row:hover {
  background: rgba(255, 255, 255, 0.04);
}
.task-main {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}
.task-kind {
  flex-shrink: 0;
  background: rgba(244, 114, 182, 0.12) !important;
  color: #f9a8d4 !important;
}
.task-text {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.task-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}
.task-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.task-time {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.35);
}
.status-tag {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 28px 12px 32px;
  color: rgba(255, 255, 255, 0.4);
  font-size: 13px;
}
.empty p {
  margin: 0;
}
@media (max-width: 1100px) {
  .kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .main-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 640px) {
  .kpi-grid {
    grid-template-columns: 1fr 1fr;
  }
  .page-head-actions {
    width: 100%;
  }
  .page-head-actions :deep(.n-button) {
    flex: 1;
  }
  .task-time {
    display: none;
  }
}
</style>
