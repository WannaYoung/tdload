<script setup lang="ts">
import { NButton, NIcon, NProgress, NTag } from "naive-ui";
import { CloudDownloadOutline, EllipsisHorizontalOutline } from "@vicons/ionicons5";

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
  chat_range: "频道",
  watch: "监听",
  watch_saved: "监听",
};

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

function taskKind(t: RecentTask) {
  // 优先 source：watch_saved 等 kind 会被归到 saved，但入口应按来源显示
  if (t.source) return kindLabel[t.source] || t.source;
  if (t.kind) return kindLabel[t.kind] || t.kind;
  return "任务";
}

/** 失败/任务跳转到对应业务页 */
function routeForTask(t: RecentTask) {
  const src = t.source || t.kind || "";
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

function taskPct(t: RecentTask) {
  if (!t.progressTotal) return t.status === "done" ? 100 : 0;
  return Math.min(100, Math.round((t.progressDone / t.progressTotal) * 100));
}
</script>

<template>
  <section class="panel">
    <header class="panel-head">
      <h2>最近任务</h2>
      <n-button text type="primary" title="更多" @click="emit('navigate', 'tasks')">
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
      <n-button size="small" type="primary" @click="emit('navigate', 'tasks')">创建第一个任务</n-button>
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
