<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
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
import { formatFromNow } from "../../i18n";
import ResourceOverview from "./components/ResourceOverview.vue";
import RecentTaskList, { type RecentTask } from "./components/RecentTaskList.vue";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const message = useMessage();
const loading = ref(true);
const error = ref("");
const stats = ref<DashboardStats | null>(null);
const tasks = ref<RecentTask[]>([]);

const greeting = computed(() => {
  const hour = new Date().getHours();
  if (hour < 6) return t("dashboard.greetingLate");
  if (hour < 12) return t("dashboard.greetingMorning");
  if (hour < 18) return t("dashboard.greetingAfternoon");
  return t("dashboard.greetingEvening");
});

const displayName = computed(() => auth.user?.username || t("dashboard.admin"));

const subtitle = computed(() => {
  const s = stats.value;
  if (!s) return t("dashboard.greetingWithName", { greeting: greeting.value, name: displayName.value });
  if (!s.tgConfigured) return t("dashboard.needApi");
  if (s.tgExpired > 0 || (s.tgAccounts > 0 && !s.tgActive)) return t("dashboard.sessionExpired");
  if (!s.tgActive) return t("dashboard.notLoggedInTg");
  if (s.tasksRunning > 0) return t("dashboard.tasksRunning", { n: s.tasksRunning });
  if (s.tasksQueued > 0) return t("dashboard.tasksQueued", { n: s.tasksQueued });
  if (s.tasksFailed > 0) return t("dashboard.tasksFailed", { n: s.tasksFailed });
  return t("dashboard.systemOk");
});

const failureLinks = computed(() => {
  const s = stats.value;
  if (!s || !s.tasksFailed) return [];
  const links: { route: string; label: string; count: number }[] = [];
  if ((s.tasksFailedMessage ?? 0) > 0) {
    links.push({
      route: "tasks",
      label: t("dashboard.failMessage", { n: s.tasksFailedMessage }),
      count: s.tasksFailedMessage!,
    });
  }
  if ((s.tasksFailedSaved ?? 0) > 0) {
    links.push({
      route: "saved",
      label: t("dashboard.failSaved", { n: s.tasksFailedSaved }),
      count: s.tasksFailedSaved!,
    });
  }
  if ((s.tasksFailedChannel ?? 0) > 0) {
    links.push({
      route: "channels",
      label: t("dashboard.failChannel", { n: s.tasksFailedChannel }),
      count: s.tasksFailedChannel!,
    });
  }
  if ((s.tasksFailedWatch ?? 0) > 0) {
    links.push({
      route: "watch",
      label: t("dashboard.failWatch", { n: s.tasksFailedWatch }),
      count: s.tasksFailedWatch!,
    });
  }
  // 兼容旧后端未返回分项时，仍给一个总入口
  if (!links.length && s.tasksFailed > 0) {
    links.push({ route: "tasks", label: t("dashboard.viewTasks"), count: s.tasksFailed });
  }
  return links;
});

const failureHint = computed(() => {
  const links = failureLinks.value;
  if (!links.length) return "";
  if (links.length === 1) {
    const map: Record<string, string> = {
      tasks: t("dashboard.hintTasks"),
      saved: t("dashboard.hintSaved"),
      channels: t("dashboard.hintChannels"),
      watch: t("dashboard.hintWatch"),
    };
    return map[links[0].route] || t("dashboard.hintGeneric");
  }
  return t("dashboard.hintMulti");
});

const kpis = computed(() => {
  const s = stats.value;
  return [
    {
      key: "running",
      label: t("dashboard.kpiRunning"),
      value: s?.tasksRunning ?? 0,
      tone: "tone-running",
      icon: PlayCircleOutline,
    },
    {
      key: "queued",
      label: t("dashboard.kpiQueued"),
      value: s?.tasksQueued ?? 0,
      tone: "tone-queued",
      icon: HourglassOutline,
    },
    {
      key: "done",
      label: t("dashboard.kpiDone"),
      value: s?.tasksDone ?? 0,
      tone: "tone-done",
      icon: CheckmarkCircleOutline,
    },
    {
      key: "failed",
      label: t("dashboard.kpiFailed"),
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

function formatSize(n: number): string {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

const diskHint = computed(() => {
  const s = stats.value;
  if (!s) return "";
  if (!s.diskTotal) return s.downloadDir || "";
  return t("dashboard.diskUsed", {
    used: formatSize(s.diskUsed),
    available: formatSize(s.diskAvailable),
  });
});

const dialogsSyncedText = computed(() => {
  const s = stats.value;
  if (!s) return "";
  return s.dialogsSyncedAt
    ? t("dashboard.dialogsSynced", { time: formatFromNow(s.dialogsSyncedAt) })
    : t("dashboard.dialogsNotSynced");
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
      error.value = err instanceof Error ? err.message : t("common.loadFailed");
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
      const count = ev.error ? t("dashboard.watchHitCount", { n: ev.error }) : "";
      message.info(
        t("dashboard.watchHit", {
          title: ev.title || ev.chatId || t("dashboard.dialogFallback"),
          range,
          count,
        }),
      );
      scheduleRefresh();
      return;
    }
    const idx = tasks.value.findIndex((row) => row.id === ev.taskId);
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
        <h1>{{ t("dashboard.title") }}</h1>
        <p class="sub">
          {{
            t("dashboard.headSubtitle", {
              greeting,
              name: displayName,
              detail: subtitle,
            })
          }}
        </p>
      </div>
      <div class="page-head-actions">
        <n-button @click="refresh(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          {{ t("dashboard.refresh") }}
        </n-button>
        <n-button type="primary" ghost @click="go('telegram')">
          <template #icon>
            <n-icon :component="PaperPlaneOutline" />
          </template>
          {{ t("layout.telegram") }}
        </n-button>
        <n-button type="primary" @click="go('tasks')">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          {{ t("dashboard.newTask") }}
        </n-button>
      </div>
    </header>

    <n-alert
      v-if="stats && !stats.tgActive"
      type="info"
      :title="t('dashboard.notLoggedInTitle')"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>{{ t("dashboard.notLoggedInBody") }}</span>
        <n-button size="small" type="primary" @click="go('telegram')">{{ t("dashboard.goLogin") }}</n-button>
      </div>
    </n-alert>
    <n-alert
      v-else-if="stats && stats.tgExpired > 0"
      type="warning"
      :title="t('dashboard.sessionExpiredTitle')"
      style="margin-bottom: 16px"
    >
      <div class="alert-body">
        <span>{{ t("dashboard.sessionExpiredBody") }}</span>
        <n-button size="small" @click="go('telegram')">{{ t("dashboard.goHandle") }}</n-button>
      </div>
    </n-alert>
    <n-alert
      v-else-if="stats && failureLinks.length"
      type="warning"
      :title="t('dashboard.tasksFailedTitle', { n: stats.tasksFailed })"
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
