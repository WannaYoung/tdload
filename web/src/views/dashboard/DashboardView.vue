<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { NAlert, NButton, NIcon, NSpin, useMessage } from "naive-ui";
import {
  AddOutline,
  AlertCircleOutline,
  CheckmarkCircleOutline,
  HourglassOutline,
  PaperPlaneOutline,
  PlayCircleOutline,
  RefreshOutline,
} from "@vicons/ionicons5";
import { api } from "../../api/http";
import type { DashboardStats } from "../../api/types";
import { useAuthStore } from "../../stores/auth";
import { useAppEvents } from "../../composables/useAppEvents";
import ResourceOverview from "./components/ResourceOverview.vue";
import RecentTaskList, { type RecentTask } from "./components/RecentTaskList.vue";

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
  if (s.tasksFailed > 0) return `${s.tasksFailed} 个任务失败，请到对应页面查看`;
  return "系统运行正常";
});

const failureLinks = computed(() => {
  const s = stats.value;
  if (!s || !s.tasksFailed) return [];
  const links: { route: string; label: string; count: number }[] = [];
  if ((s.tasksFailedMessage ?? 0) > 0) {
    links.push({ route: "tasks", label: `消息 ${s.tasksFailedMessage}`, count: s.tasksFailedMessage! });
  }
  if ((s.tasksFailedSaved ?? 0) > 0) {
    links.push({ route: "saved", label: `收藏 ${s.tasksFailedSaved}`, count: s.tasksFailedSaved! });
  }
  if ((s.tasksFailedChannel ?? 0) > 0) {
    links.push({ route: "channels", label: `频道 ${s.tasksFailedChannel}`, count: s.tasksFailedChannel! });
  }
  if ((s.tasksFailedWatch ?? 0) > 0) {
    links.push({ route: "watch", label: `监听 ${s.tasksFailedWatch}`, count: s.tasksFailedWatch! });
  }
  // 兼容旧后端未返回分项时，仍给一个总入口
  if (!links.length && s.tasksFailed > 0) {
    links.push({ route: "tasks", label: "查看任务", count: s.tasksFailed });
  }
  return links;
});

const failureHint = computed(() => {
  const links = failureLinks.value;
  if (!links.length) return "";
  if (links.length === 1) {
    const map: Record<string, string> = {
      tasks: "可在任务页查看原因并重试。",
      saved: "可在收藏页查看同步失败记录。",
      channels: "可在频道页查看下载失败批次。",
      watch: "可在监听页查看相关任务。",
    };
    return map[links[0].route] || "请到对应页面查看。";
  }
  return "请到对应页面查看失败原因。";
});

const kpis = computed(() => {
  const s = stats.value;
  return [
    { key: "running", label: "下载中", value: s?.tasksRunning ?? 0, tone: "tone-running", icon: PlayCircleOutline },
    { key: "queued", label: "排队", value: s?.tasksQueued ?? 0, tone: "tone-queued", icon: HourglassOutline },
    { key: "done", label: "已完成", value: s?.tasksDone ?? 0, tone: "tone-done", icon: CheckmarkCircleOutline },
    { key: "failed", label: "失败", value: s?.tasksFailed ?? 0, tone: "tone-fail", icon: AlertCircleOutline },
  ];
});

const diskPercent = computed(() => {
  const s = stats.value;
  if (!s || !s.diskTotal) return 0;
  const usedFs = Math.max(0, s.diskTotal - s.diskAvailable);
  return Math.min(100, Math.round((usedFs / s.diskTotal) * 100));
});

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

const diskHint = computed(() => {
  const s = stats.value;
  if (!s) return "";
  if (!s.diskTotal) return s.downloadDir || "";
  return `已用 ${formatSize(s.diskUsed)} · 可用 ${formatSize(s.diskAvailable)}`;
});

const dialogsSyncedText = computed(() => {
  const s = stats.value;
  if (!s) return "";
  return s.dialogsSyncedAt ? `同步于 ${fromNow(s.dialogsSyncedAt)}` : "尚未同步，请到 Telegram 页刷新";
});

const recentTasks = computed(() => tasks.value.slice(0, 8));

function go(name: string) {
  void router.push({ name });
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
      const range = ev.done != null && ev.total != null ? `#${ev.done}–#${ev.total}` : "";
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

useAppEvents(applyEvent);

onMounted(async () => {
  await refresh(false);
  pollTimer = window.setInterval(() => void refresh(true), 12000);
});

onUnmounted(() => {
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
        <n-button @click="refresh(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
        <n-button type="primary" ghost @click="go('telegram')">
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
      v-else-if="stats && failureLinks.length"
      type="warning"
      :title="`${stats.tasksFailed} 个任务失败`"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>{{ failureHint }}</span>
        <div class="alert-actions">
          <n-button
            v-for="link in failureLinks"
            :key="link.route"
            size="small"
            @click="go(link.route)"
          >
            {{ link.label }}
          </n-button>
        </div>
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
      <ResourceOverview
        :stats="stats"
        :disk-percent="diskPercent"
        :disk-hint="diskHint"
        :dialogs-synced-text="dialogsSyncedText"
        @navigate="go"
      />
      <RecentTaskList :tasks="recentTasks" @navigate="go" />
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
.alert-actions {
  display: flex;
  flex-wrap: wrap;
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
@media (max-width: 1100px) {
  .main-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 900px) {
  .kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
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
}
</style>
