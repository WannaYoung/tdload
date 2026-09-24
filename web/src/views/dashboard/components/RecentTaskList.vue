<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NIcon, NProgress, NTag } from "naive-ui";
import { DownloadOutline, EllipsisHorizontalOutline } from "@vicons/ionicons5";
import { formatFromNow } from "../../../i18n";

export type RecentTask = {
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

defineProps<{
  tasks: RecentTask[];
}>();

const emit = defineEmits<{
  navigate: [name: string];
}>();

const { t } = useI18n();

const statusMeta = computed<Record<string, { label: string; color: string }>>(() => ({
  running: { label: t("status.running"), color: "#f9a8d4" },
  queued: { label: t("status.queued"), color: "#93c5fd" },
  done: { label: t("status.done"), color: "#86efac" },
  failed: { label: t("status.failed"), color: "#fca5a5" },
  paused: { label: t("status.paused"), color: "#fcd34d" },
  cancelled: { label: t("status.cancelled"), color: "rgba(255,255,255,0.45)" },
}));

const kindLabel = computed<Record<string, string>>(() => ({
  message: t("kind.message"),
  saved: t("kind.saved"),
  channel: t("kind.channel"),
  url: t("kind.message"),
  saved_all: t("kind.saved"),
  chat_batch: t("kind.channel"),
  chat_continue: t("kind.channel"),
  chat_range: t("kind.channel"),
  watch: t("kind.watch"),
  watch_saved: t("kind.watch"),
}));

function taskKind(row: RecentTask) {
  // 优先 source：watch_saved 等 kind 会被归到 saved，但入口应按来源显示
  if (row.source) return kindLabel.value[row.source] || row.source;
  if (row.kind) return kindLabel.value[row.kind] || row.kind;
  return t("kind.task");
}

/** 失败/任务跳转到对应业务页 */
function routeForTask(row: RecentTask) {
  const src = row.source || row.kind || "";
  switch (src) {
    case "saved":
    case "saved_all":
      return "saved";
    case "channel":
    case "chat_continue":
    case "chat_range":
    case "chat_batch":
      return "channels";
    case "watch":
    case "watch_saved":
      return "watch";
    default:
      return "tasks";
  }
}

function taskPct(row: RecentTask) {
  if (!row.progressTotal) return row.status === "done" ? 100 : 0;
  return Math.min(100, Math.round((row.progressDone / row.progressTotal) * 100));
}
</script>

<template>
  <section class="panel">
    <header class="panel-head">
      <h2>{{ t("recentTasks.title") }}</h2>
      <n-button text type="primary" :title="t('common.more')" @click="emit('navigate', 'tasks')">
        <template #icon>
          <n-icon :component="EllipsisHorizontalOutline" />
        </template>
      </n-button>
    </header>
    <ul v-if="tasks.length" class="task-list">
      <li v-for="row in tasks" :key="row.id" class="task-row" @click="emit('navigate', routeForTask(row))">
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
          <span class="task-time">{{ formatFromNow(row.createdAt) }}</span>
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
      <n-icon :component="DownloadOutline" :size="28" />
      <p>{{ t("recentTasks.empty") }}</p>
      <n-button size="small" type="primary" @click="emit('navigate', 'tasks')">
        {{ t("recentTasks.createFirst") }}
      </n-button>
    </div>
  </section>
</template>

<style scoped>
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
@media (max-width: 640px) {
  .task-time {
    display: none;
  }
}
</style>
