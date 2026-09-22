<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import {
  NAlert,
  NButton,
  NEmpty,
  NInput,
  NInputNumber,
  NProgress,
  NSelect,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from "naive-ui";
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

const message = useMessage();
const isMobile = useMobile();
const tab = ref<"message" | "saved" | "channel">("message");
const loading = ref(false);
const submitting = ref(false);
const text = ref("");
const messageItems = ref<TaskItemRow[]>([]);
const savedItems = ref<TaskItemRow[]>([]);
const channelTasks = ref<Task[]>([]);
const channels = ref<ChannelRow[]>([]);
const channelChatId = ref<number | null>(null);
const fromMsgId = ref<number | null>(null);
const batchCount = ref(50);
let es: EventSource | null = null;
let pollTimer: number | null = null;

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

function pct(done: number, total: number, status: string) {
  if (!total) return status === "done" ? 100 : 0;
  return Math.min(100, Math.round((done / total) * 100));
}

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
    const page = await api<{ items: TaskItemRow[] }>("/api/tasks/items?kind=message&page=1&pageSize=50");
    messageItems.value = page.items || [];
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadSavedItems() {
  loading.value = true;
  try {
    const page = await api<{ items: TaskItemRow[] }>("/api/tasks/items?kind=saved&page=1&pageSize=50");
    savedItems.value = page.items || [];
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
  else if (tab.value === "saved") await loadSavedItems();
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
    message.success("收藏下载已入队");
    await loadSavedItems();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitting.value = false;
  }
}

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

onMounted(() => {
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
      <div>
        <h2>任务</h2>
        <p class="muted">消息/收藏：单条进度 · 频道：任务总进度（每页 20）</p>
      </div>
      <n-button quaternary :disabled="loading" @click="reloadTab">刷新</n-button>
    </div>

    <n-tabs v-model:value="tab" type="line" animated>
      <n-tab-pane name="message" tab="消息下载">
        <div class="composer">
          <n-input v-model:value="text" type="textarea" :rows="3" placeholder="每行一条 t.me 链接" />
          <n-button type="primary" :loading="submitting" :disabled="!text.trim()" @click="createMessageTask">
            开始下载
          </n-button>
        </div>
        <n-empty v-if="!messageItems.length && !loading" description="暂无消息项" />
        <div v-for="it in messageItems" :key="it.id" class="item-row">
          <div class="item-head">
            <span>#{{ it.messageId }} {{ it.fileName || "消息" }}</span>
            <n-tag size="small" :type="statusColor(it.status)">{{ it.status }}</n-tag>
          </div>
          <p v-if="it.error" class="err">{{ it.error }}</p>
        </div>
      </n-tab-pane>

      <n-tab-pane name="saved" tab="收藏下载">
        <n-alert type="info" style="margin-bottom: 12px">落盘目录：downloads/我的收藏</n-alert>
        <n-button type="primary" :loading="submitting" @click="createSavedTask">下载全部收藏（跳过已下）</n-button>
        <n-empty v-if="!savedItems.length && !loading" description="暂无收藏下载记录" style="margin-top: 16px" />
        <div v-for="it in savedItems" :key="it.id" class="item-row">
          <div class="item-head">
            <span>收藏 #{{ it.messageId }} {{ it.fileName }}</span>
            <n-tag size="small" :type="statusColor(it.status)">{{ it.status }}</n-tag>
          </div>
        </div>
      </n-tab-pane>

      <n-tab-pane name="channel" tab="频道下载">
        <div class="channel-form">
          <n-select v-model:value="channelChatId" :options="channelOptions" placeholder="选择频道" filterable />
          <n-input-number v-model:value="fromMsgId" placeholder="起始 message id" :min="1" clearable />
          <n-input-number v-model:value="batchCount" placeholder="数量" :min="1" :max="5000" />
          <n-button type="primary" :loading="submitting" @click="createChannelTask('chat_batch')">按 ID 批量</n-button>
          <n-button secondary :loading="submitting" @click="createChannelTask('chat_continue')">继续未下载</n-button>
        </div>
        <n-alert type="warning" style="margin: 12px 0">频道 Worker 批量逻辑开发中；可先使用消息 Tab 粘贴链接。</n-alert>
        <n-empty v-if="!channelTasks.length && !loading" description="暂无频道任务" />
        <div v-for="t in channelTasks" :key="t.id" class="task-card">
          <div class="task-head">
            <div class="task-title">#{{ t.id }} {{ t.title }}</div>
            <n-tag size="small" :type="statusColor(t.status)">{{ t.status }}</n-tag>
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
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
}
.muted {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
}
.composer {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 16px;
}
.channel-form {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
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
</style>
