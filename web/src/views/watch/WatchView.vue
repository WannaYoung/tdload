<script setup lang="ts">
import { h, onMounted, onUnmounted, ref } from "vue";
import {
  NButton,
  NDataTable,
  NEmpty,
  NSelect,
  NSpin,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { api } from "../../api/http";
import type { WatchCandidate, WatchRow } from "../../api/types";

defineOptions({ name: "WatchView" });

const message = useMessage();
const loading = ref(false);
const adding = ref(false);
const rows = ref<WatchRow[]>([]);
const candidates = ref<WatchCandidate[]>([]);
const selectedChatId = ref<number | null>(null);
const intervalMinutes = ref(30);

let timer: number | null = null;

function formatTime(v?: string) {
  if (!v) return "—";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

const selectOptions = () =>
  candidates.value.map((c) => ({
    label: c.username ? `${c.title} (@${c.username})` : c.title,
    value: c.chatId,
  }));

const columns: DataTableColumns<WatchRow> = [
  {
    title: "频道",
    key: "chatTitle",
    ellipsis: { tooltip: true },
    render: (r) =>
      h("div", { class: "name-cell" }, [
        h("div", { class: "name-title" }, r.chatTitle || String(r.chatId)),
        r.isFavorites ? h("div", { class: "name-sub" }, "收藏") : null,
      ]),
  },
  {
    title: "已下载",
    key: "downloadedCount",
    width: 90,
    render: (r) => String(r.downloadedCount ?? 0),
  },
  {
    title: "最新消息",
    key: "lastMessageId",
    width: 110,
    render: (r) => (r.lastMessageId > 0 ? String(r.lastMessageId) : "—"),
  },
  {
    title: "上次运行",
    key: "lastRunAt",
    width: 160,
    render: (r) => formatTime(r.lastRunAt),
  },
  {
    title: "下次运行",
    key: "nextRunAt",
    width: 160,
    render: (r) => formatTime(r.nextRunAt),
  },
  {
    title: "操作",
    key: "actions",
    width: 90,
    align: "right",
    render: (r) =>
      h(
        NButton,
        {
          size: "small",
          type: "error",
          secondary: true,
          onClick: () => void remove(r),
        },
        { default: () => "删除" },
      ),
  },
];

async function loadCandidates() {
  try {
    const data = await api<{ items: WatchCandidate[] }>("/api/watch/candidates");
    candidates.value = data.items || [];
    if (
      selectedChatId.value != null &&
      !candidates.value.some((c) => c.chatId === selectedChatId.value)
    ) {
      selectedChatId.value = null;
    }
  } catch {
    candidates.value = [];
  }
}

async function load() {
  loading.value = true;
  try {
    const data = await api<{ items: WatchRow[]; watchIntervalMinutes?: number }>("/api/watch");
    rows.value = data.items || [];
    if (data.watchIntervalMinutes) intervalMinutes.value = data.watchIntervalMinutes;
    await loadCandidates();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function addWatch() {
  if (selectedChatId.value == null) {
    message.warning("请先选择频道");
    return;
  }
  adding.value = true;
  try {
    await api("/api/watch", {
      method: "POST",
      body: JSON.stringify({ chatId: selectedChatId.value }),
    });
    message.success("已加入监听");
    selectedChatId.value = null;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "添加失败");
  } finally {
    adding.value = false;
  }
}

async function remove(row: WatchRow) {
  try {
    await api(`/api/watch/${row.id}`, { method: "DELETE" });
    message.success("已移除监听");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
  }
}

onMounted(() => {
  void load();
  timer = window.setInterval(() => void load(), 30_000);
});

onUnmounted(() => {
  if (timer != null) window.clearInterval(timer);
});
</script>

<template>
  <div class="page list-page pinned">
    <div class="toolbar">
      <div>
        <h2>监听</h2>
        <p>按设置间隔扫描增量消息并自动入队（当前间隔 {{ intervalMinutes }} 分钟）。</p>
      </div>
      <n-button quaternary :loading="loading" @click="load">刷新</n-button>
    </div>

    <div class="add-row">
      <n-select
        v-model:value="selectedChatId"
        class="chat-select"
        filterable
        clearable
        placeholder="选择要监听的频道"
        :options="selectOptions()"
        :disabled="loading || !candidates.length"
      />
      <n-button type="primary" :loading="adding" :disabled="selectedChatId == null" @click="addWatch">
        添加监听
      </n-button>
    </div>

    <div class="table-wrap">
      <n-spin :show="loading" class="spin-fill">
        <n-empty
          v-if="!rows.length && !loading"
          description="暂无监听。可先添加「我的收藏」或已同步的频道。"
        />
        <n-data-table v-else :columns="columns" :data="rows" :bordered="false" size="small" />
      </n-spin>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
}
.toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.toolbar p {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
}
.add-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: nowrap;
}
.chat-select {
  flex: 1 1 auto;
  min-width: 180px;
  max-width: 480px;
}
.spin-fill {
  min-height: 120px;
}
:deep(.name-cell) {
  min-width: 0;
}
:deep(.name-title) {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.9);
}
:deep(.name-sub) {
  margin-top: 2px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
}
@media (max-width: 640px) {
  .add-row {
    flex-wrap: wrap;
  }
  .chat-select {
    max-width: none;
    width: 100%;
  }
}
</style>
