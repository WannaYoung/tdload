<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  NButton,
  NDataTable,
  NEmpty,
  NIcon,
  NInput,
  NInputNumber,
  NProgress,
  NSelect,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import {
  DocumentOutline,
  ImageOutline,
  MusicalNotesOutline,
  TrashOutline,
  VideocamOutline,
} from "@vicons/ionicons5";
import { api, openEventSource } from "../api/http";
import { useMobile } from "../composables/useMobile";
import type { ChannelRow, ItemCounts, TaskItemRow } from "../api/types";

defineOptions({ name: "TasksView" });

type Task = {
  id: number;
  title: string;
  status: string;
  source?: string;
  kind?: string;
  chatId?: number;
  doneFiles: number;
  totalFiles: number;
  progressDone: number;
  progressTotal: number;
  itemCounts?: ItemCounts;
  error: string;
  createdAt: string;
};

const route = useRoute();
const message = useMessage();
const isMobile = useMobile();
const tab = ref<"message" | "saved" | "channel">("message");
const loading = ref(false);
const submitting = ref(false);
const clearing = ref(false);
const text = ref("");
const messageItems = ref<TaskItemRow[]>([]);
const messageTotal = ref(0);
const messageDone = ref(0);
const messagePage = ref(1);
const messagePageSize = 50;
const savedTasks = ref<Task[]>([]);
const channelTasks = ref<Task[]>([]);
const channels = ref<ChannelRow[]>([]);
const channelChatId = ref<number | null>(null);
const fromMsgId = ref<number | null>(null);
const batchCount = ref(50);
let es: EventSource | null = null;
let pollTimer: number | null = null;

const tabOptions = [
  { name: "message" as const, label: "消息下载" },
  { name: "saved" as const, label: "收藏同步" },
  { name: "channel" as const, label: "频道下载" },
];

const statusLabel: Record<string, string> = {
  pending: "等待",
  downloading: "下载中",
  running: "下载中",
  queued: "排队",
  done: "完成",
  skipped: "跳过",
  failed: "失败",
  paused: "暂停",
  cancelled: "取消",
};

const statusColor = computed(() => (s: string) => {
  switch (s) {
    case "running":
    case "downloading":
      return "success";
    case "queued":
    case "pending":
      return "info";
    case "failed":
      return "error";
    case "paused":
      return "warning";
    case "done":
    case "skipped":
      return "default";
    default:
      return "default";
  }
});

const statusTagColor: Record<string, { color: string; textColor: string; borderColor: string }> = {
  downloading: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  running: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  pending: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  queued: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  done: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  skipped: { color: "rgba(255, 255, 255, 0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
  failed: { color: "rgba(248, 113, 113, 0.16)", textColor: "#fca5a5", borderColor: "transparent" },
  paused: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
};

const pinkTag = {
  color: "rgba(244, 114, 182, 0.16)",
  textColor: "#f9a8d4",
  borderColor: "transparent",
};

function pct(done: number, total: number, status: string) {
  if (!total) return status === "done" ? 100 : 0;
  return Math.min(100, Math.round((done / total) * 100));
}

function renderStatus(status: string) {
  const color = statusTagColor[status] || {
    color: "rgba(255,255,255,0.08)",
    textColor: "rgba(255,255,255,0.55)",
    borderColor: "transparent",
  };
  return h(NTag, { size: "small", bordered: false, color }, { default: () => statusLabel[status] || status });
}

function mediaIcon(kind?: string) {
  switch (kind) {
    case "图片":
      return ImageOutline;
    case "视频":
      return VideocamOutline;
    case "音频":
      return MusicalNotesOutline;
    default:
      return DocumentOutline;
  }
}

function renderMediaKind(kind?: string) {
  const label = kind || "文件";
  return h(
    NTag,
    {
      size: "small",
      bordered: false,
      color: pinkTag,
      title: label,
    },
    {
      default: () => h(NIcon, { size: 16, component: mediaIcon(label) }),
    },
  );
}

async function deleteMessageItem(id: number) {
  try {
    await api(`/api/tasks/items/${id}`, { method: "DELETE" });
    message.success("已删除");
    await loadMessageItems();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
  }
}

async function clearCompletedMessages() {
  clearing.value = true;
  try {
    const res = await api<{ cleared: number }>("/api/tasks/items/completed?kind=message", { method: "DELETE" });
    message.success(res.cleared ? `已清除 ${res.cleared} 条` : "没有可清除的已完成项");
    messagePage.value = 1;
    await loadMessageItems();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "清除失败");
  } finally {
    clearing.value = false;
  }
}

const messageColumns = computed<DataTableColumns<TaskItemRow>>(() => [
  {
    title: "频道",
    key: "chatTitle",
    ellipsis: { tooltip: true },
    render: (r) => r.chatTitle || String(r.chatId || "—"),
  },
  {
    title: "消息 ID",
    key: "messageId",
    width: 100,
  },
  {
    title: "名称",
    key: "fileName",
    ellipsis: { tooltip: true },
    render: (r) => r.fileName || "—",
  },
  {
    title: "类型",
    key: "mediaKind",
    width: 64,
    align: "center",
    render: (r) => renderMediaKind(r.mediaKind),
  },
  {
    title: "状态",
    key: "status",
    width: 96,
    render: (r) => renderStatus(r.status),
  },
  {
    title: "操作",
    key: "actions",
    width: 72,
    align: "right",
    render: (r) =>
      h(
        NButton,
        {
          size: "tiny",
          quaternary: true,
          type: "error",
          title: "删除",
          onClick: () => void deleteMessageItem(r.id),
        },
        { icon: () => h(NIcon, { component: TrashOutline }) },
      ),
  },
]);

async function loadChannels() {
  try {
    const data = await api<{ items: ChannelRow[] }>("/api/channels?page=1&pageSize=500");
    channels.value = data.items || [];
  } catch {
    /* ignore */
  }
}

async function loadMessageItems() {
  loading.value = true;
  try {
    const page = await api<{ items: TaskItemRow[]; total: number; doneCount: number }>(
      `/api/tasks/items?kind=message&page=${messagePage.value}&pageSize=${messagePageSize}`,
    );
    messageItems.value = page.items || [];
    messageTotal.value = page.total || 0;
    messageDone.value = page.doneCount || 0;
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadSavedTasks() {
  loading.value = true;
  try {
    const page = await api<{ items: Task[] }>("/api/tasks?kind=saved&page=1&pageSize=50");
    savedTasks.value = page.items || [];
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadChannelTasks() {
  loading.value = true;
  try {
    const page = await api<{ items: Task[] }>("/api/tasks?kind=channel&page=1&pageSize=20");
    channelTasks.value = page.items || [];
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function reloadTab() {
  if (tab.value === "message") await loadMessageItems();
  else if (tab.value === "saved") await loadSavedTasks();
  else await loadChannelTasks();
}

async function createMessageTask() {
  submitting.value = true;
  try {
    await api("/api/tasks", { method: "POST", body: JSON.stringify({ text: text.value, source: "url" }) });
    text.value = "";
    message.success("已入队");
    await loadMessageItems();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitting.value = false;
  }
}

async function createSavedTask() {
  submitting.value = true;
  try {
    await api("/api/tasks", { method: "POST", body: JSON.stringify({ source: "saved_all" }) });
    message.success("已开始同步");
    await loadSavedTasks();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitting.value = false;
  }
}

async function deleteSavedTask(id: number) {
  try {
    await api(`/api/tasks/${id}/cancel`, { method: "POST", body: "{}" }).catch(() => undefined);
    await api(`/api/tasks/${id}`, { method: "DELETE" });
    message.success("已删除");
    await loadSavedTasks();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
  }
}

function formatSyncTime(iso: string) {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  return new Date(t).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function savedTotal(t: Task) {
  const c = t.itemCounts;
  if (c) {
    const n = c.pending + c.downloading + c.done + c.skipped + c.failed;
    if (n > 0) return n;
  }
  return t.progressTotal || t.totalFiles || 0;
}

function savedDone(t: Task) {
  if (t.itemCounts) return t.itemCounts.done + t.itemCounts.skipped;
  return t.progressDone || t.doneFiles || 0;
}

function savedFailed(t: Task) {
  return t.itemCounts?.failed ?? 0;
}

const savedColumns = computed<DataTableColumns<Task>>(() => [
  {
    title: "收藏数",
    key: "total",
    width: 100,
    render: (r) => String(savedTotal(r)),
  },
  {
    title: "已同步",
    key: "done",
    width: 100,
    render: (r) => String(savedDone(r)),
  },
  {
    title: "同步失败",
    key: "failed",
    width: 100,
    render: (r) => String(savedFailed(r)),
  },
  {
    title: "同步时间",
    key: "createdAt",
    ellipsis: { tooltip: true },
    render: (r) => formatSyncTime(r.createdAt),
  },
  {
    title: "操作",
    key: "actions",
    width: 72,
    align: "right",
    render: (r) =>
      h(
        NButton,
        {
          size: "tiny",
          quaternary: true,
          type: "error",
          title: "停止并删除",
          onClick: () => void deleteSavedTask(r.id),
        },
        { icon: () => h(NIcon, { component: TrashOutline }) },
      ),
  },
]);

async function createChannelTask(source: "chat_batch" | "chat_continue") {
  if (!channelChatId.value) {
    message.warning("请选择频道");
    return;
  }
  submitting.value = true;
  try {
    await api("/api/tasks", {
      method: "POST",
      body: JSON.stringify({
        source,
        chatId: channelChatId.value,
        fromMessageId: fromMsgId.value || 0,
        count: batchCount.value,
      }),
    });
    message.success("频道任务已入队");
    await loadChannelTasks();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitting.value = false;
  }
}

async function pause(id: number) {
  await api(`/api/tasks/${id}/pause`, { method: "POST", body: "{}" });
  await reloadTab();
}

async function resume(id: number) {
  await api(`/api/tasks/${id}/resume`, { method: "POST", body: "{}" });
  await reloadTab();
}

async function cancel(id: number) {
  await api(`/api/tasks/${id}/cancel`, { method: "POST", body: "{}" });
  await reloadTab();
}

async function retryFailed(id: number) {
  await api(`/api/tasks/${id}/retry-failed`, { method: "POST", body: "{}" });
  await reloadTab();
}

async function pauseAll() {
  try {
    const data = await api<{ paused: number }>("/api/tasks/pause-all", { method: "POST", body: "{}" });
    message.success(`已暂停 ${data.paused} 个任务`);
    await reloadTab();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "暂停失败");
  }
}

async function startAll() {
  try {
    const data = await api<{ started: number }>("/api/tasks/start-all", { method: "POST", body: "{}" });
    message.success(`已恢复 ${data.started} 个任务`);
    await reloadTab();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "启动失败");
  }
}

async function remove(id: number) {
  await api(`/api/tasks/${id}`, { method: "DELETE" });
  await reloadTab();
}

function countsLine(c?: ItemCounts) {
  if (!c) return "";
  return `待${c.pending} · 下${c.downloading} · 完${c.done} · 跳${c.skipped} · 败${c.failed}`;
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as {
      type?: string;
      kind?: string;
      taskId?: number;
      done?: number;
      total?: number;
      status?: string;
      itemCounts?: ItemCounts;
    };
    if (ev.type === "task_item_progress") {
      void reloadTab();
      return;
    }
    const saved = savedTasks.value.find((x) => x.id === ev.taskId);
    if (saved) {
      if (ev.done != null) {
        saved.progressDone = ev.done;
        saved.doneFiles = ev.done;
      }
      if (ev.total != null) {
        saved.progressTotal = ev.total;
        saved.totalFiles = ev.total;
      }
      if (ev.status) saved.status = ev.status;
      if (ev.itemCounts) saved.itemCounts = ev.itemCounts;
      return;
    }
    const t = channelTasks.value.find((x) => x.id === ev.taskId);
    if (t) {
      if (ev.done != null) {
        t.progressDone = ev.done;
        t.doneFiles = ev.done;
      }
      if (ev.total != null) {
        t.progressTotal = ev.total;
        t.totalFiles = ev.total;
      }
      if (ev.status) t.status = ev.status;
      if (ev.itemCounts) t.itemCounts = ev.itemCounts;
    } else {
      void reloadTab();
    }
  } catch {
    /* ignore */
  }
}

watch(tab, () => void reloadTab());

function applyRoutePrefill() {
  const q = route.query;
  if (q.tab === "channel" || q.tab === "saved" || q.tab === "message") {
    tab.value = q.tab;
  }
  if (q.chatId != null && String(q.chatId) !== "") {
    const id = Number(q.chatId);
    if (Number.isFinite(id)) {
      channelChatId.value = id;
      tab.value = "channel";
    }
  }
}

watch(
  () => [route.query.tab, route.query.chatId] as const,
  () => {
    applyRoutePrefill();
  },
);

onMounted(() => {
  applyRoutePrefill();
  void loadChannels();
  void reloadTab();
  void (async () => {
    try {
      es = await openEventSource("/api/events");
      es.onmessage = (e) => applyEvent(e.data);
    } catch {
      /* poll */
    }
  })();
  pollTimer = window.setInterval(() => void reloadTab(), 8000);
});

onUnmounted(() => {
  es?.close();
  if (pollTimer != null) window.clearInterval(pollTimer);
});

const channelOptions = computed(() =>
  channels.value.map((c) => ({
    label: c.title || String(c.chatId),
    value: c.chatId,
  })),
);
</script>

<template>
  <div class="page list-page" :class="{ pinned: !isMobile }">
    <div class="toolbar">
      <h2>任务</h2>
      <div class="tab-bar">
        <button
          v-for="opt in tabOptions"
          :key="opt.name"
          type="button"
          class="tab-btn"
          :class="{ active: tab === opt.name }"
          @click="tab = opt.name"
        >
          {{ opt.label }}
        </button>
      </div>
      <n-button quaternary :disabled="loading" @click="pauseAll">全部暂停</n-button>
      <n-button quaternary :disabled="loading" @click="startAll">全部开始</n-button>
      <n-button quaternary :disabled="loading" @click="reloadTab">刷新</n-button>
    </div>

    <template v-if="tab === 'message'">
      <div class="composer row">
        <n-input
          v-model:value="text"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 8 }"
          placeholder="每行一条 t.me 链接"
          class="composer-input"
        />
        <n-button type="primary" :loading="submitting" :disabled="!text.trim()" @click="createMessageTask">
          下载
        </n-button>
      </div>
      <div class="list-meta">
        <span>共 {{ messageTotal }} 条消息，{{ messageDone }} 条已下载</span>
        <n-button
          size="small"
          type="primary"
          secondary
          :loading="clearing"
          :disabled="messageDone <= 0"
          @click="clearCompletedMessages"
        >
          清除已完成
        </n-button>
      </div>
      <div class="table-wrap message-table">
        <n-empty v-if="!messageItems.length && !loading" description="暂无消息项" />
        <n-data-table
          v-else
          flex-height
          :columns="messageColumns"
          :data="messageItems"
          :bordered="false"
          size="small"
          :row-key="(r: TaskItemRow) => r.id"
        />
      </div>
      <div v-if="messageTotal > messagePageSize" class="pager">
        <n-button size="small" :disabled="messagePage <= 1" @click="messagePage--; loadMessageItems()">上一页</n-button>
        <span>{{ messagePage }} / {{ Math.ceil(messageTotal / messagePageSize) }}</span>
        <n-button
          size="small"
          :disabled="messagePage * messagePageSize >= messageTotal"
          @click="messagePage++; loadMessageItems()"
        >
          下一页
        </n-button>
      </div>
    </template>

    <template v-else-if="tab === 'saved'">
      <div class="list-meta">
        <span class="section-title">同步记录</span>
        <n-button size="small" type="primary" secondary :loading="submitting" @click="createSavedTask">
          开始同步
        </n-button>
      </div>
      <div class="table-wrap message-table">
        <n-empty v-if="!savedTasks.length && !loading" description="暂无同步记录" />
        <n-data-table
          v-else
          flex-height
          :columns="savedColumns"
          :data="savedTasks"
          :bordered="false"
          size="small"
          :row-key="(r: Task) => r.id"
        />
      </div>
    </template>

    <template v-else>
      <div class="channel-form">
        <n-select v-model:value="channelChatId" :options="channelOptions" placeholder="选择频道" filterable />
        <n-input-number v-model:value="fromMsgId" placeholder="起始 message id" :min="1" clearable />
        <n-input-number v-model:value="batchCount" placeholder="数量" :min="1" :max="5000" />
        <n-button type="primary" :loading="submitting" @click="createChannelTask('chat_batch')">按 ID 批量</n-button>
        <n-button secondary :loading="submitting" @click="createChannelTask('chat_continue')">继续未下载</n-button>
      </div>
      <div class="table-wrap">
        <n-empty v-if="!channelTasks.length && !loading" description="暂无频道任务" />
        <div v-for="t in channelTasks" :key="t.id" class="task-card">
          <div class="task-head">
            <div class="task-title">#{{ t.id }} {{ t.title }}</div>
            <n-tag size="small" :type="statusColor(t.status)" :bordered="false">
              {{ statusLabel[t.status] || t.status }}
            </n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="pct(t.progressDone || t.doneFiles, t.progressTotal || t.totalFiles, t.status)"
            indicator-placement="inside"
            :processing="t.status === 'running'"
          />
          <div class="task-meta">{{ countsLine(t.itemCounts) }}</div>
          <div v-if="t.error" class="err">{{ t.error }}</div>
          <div class="task-actions">
            <n-button v-if="t.status === 'running' || t.status === 'queued'" size="tiny" @click="pause(t.id)">暂停</n-button>
            <n-button v-if="t.status === 'paused'" size="tiny" type="primary" @click="resume(t.id)">开始</n-button>
            <n-button v-if="t.status !== 'cancelled' && t.status !== 'done'" size="tiny" @click="cancel(t.id)">取消</n-button>
            <n-button size="tiny" @click="retryFailed(t.id)">重试失败</n-button>
            <n-button size="tiny" quaternary @click="remove(t.id)">删除</n-button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
  flex-shrink: 0;
}
.toolbar {
  flex-wrap: nowrap;
  align-items: center;
  gap: 16px;
}
.tab-bar {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
}
.tab-btn {
  appearance: none;
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
  font-size: 14px;
  font-weight: 500;
  padding: 6px 12px;
  border-radius: 8px;
  cursor: pointer;
  white-space: nowrap;
}
.tab-btn:hover {
  color: rgba(255, 255, 255, 0.85);
  background: rgba(255, 255, 255, 0.04);
}
.tab-btn.active {
  color: #f9a8d4;
  background: rgba(244, 114, 182, 0.12);
}
.composer {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex-shrink: 0;
}
.composer.row {
  flex-direction: row;
  align-items: flex-start;
}
.composer-input {
  flex: 1 1 auto;
  min-width: 0;
}
.list-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-shrink: 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.85);
}
.message-table {
  overflow: hidden;
}
.message-table :deep(.n-data-table) {
  height: 100%;
}
.message-table :deep(.n-data-table-base-table) {
  height: 100%;
}
.channel-form {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  flex-shrink: 0;
}
.item-row,
.task-card {
  padding: 12px 14px;
  margin-bottom: 8px;
  border-radius: 10px;
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.item-head,
.task-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
}
.task-title {
  font-weight: 600;
}
.task-meta {
  margin-top: 8px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
.task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}
.err {
  color: #f9a8d4;
  font-size: 12px;
  margin-top: 6px;
}
:deep(.kind-tag) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.pager {
  flex-shrink: 0;
}
@media (max-width: 1000px) {
  .toolbar {
    flex-wrap: wrap;
  }
  .tab-bar {
    order: 3;
    width: 100%;
  }
  .composer.row {
    flex-direction: column;
  }
  .composer.row > .n-button {
    align-self: flex-end;
  }
}
</style>
