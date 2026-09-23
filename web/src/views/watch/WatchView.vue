<script setup lang="ts">
import { h, onMounted, onUnmounted, ref } from "vue";
import {
  NButton,
  NDataTable,
  NEmpty,
  NSelect,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { api } from "../../api/http";
import { useMobile } from "../../composables/useMobile";
import type { WatchCandidate, WatchRow } from "../../api/types";

defineOptions({ name: "WatchView" });

const message = useMessage();
const isMobile = useMobile();
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

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    const data = await api<{ items: WatchRow[]; watchIntervalMinutes?: number }>("/api/watch");
    rows.value = data.items || [];
    if (data.watchIntervalMinutes) intervalMinutes.value = data.watchIntervalMinutes;
    await loadCandidates();
  } catch (e) {
    if (!silent) message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    if (!silent) loading.value = false;
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
  timer = window.setInterval(() => void load(true), 30_000);
});

onUnmounted(() => {
  if (timer != null) window.clearInterval(timer);
});
</script>

<template>
  <div class="page list-page" :class="{ pinned: !isMobile }">
    <div class="toolbar">
      <div class="toolbar-left">
        <h2>监听</h2>
        <span class="interval">间隔 {{ intervalMinutes }} 分钟</span>
      </div>
      <div class="toolbar-actions">
        <n-button quaternary :loading="loading" @click="load(false)">刷新</n-button>
      </div>
    </div>

    <div class="panel">
      <div class="composer row">
        <n-select
          v-model:value="selectedChatId"
          class="composer-input"
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

      <div class="table-wrap message-table">
        <n-data-table
          :columns="columns"
          :data="rows"
          :bordered="false"
          size="small"
          :loading="loading"
          :row-key="(r: WatchRow) => r.id"
        >
          <template #empty>
            <n-empty description="暂无监听。可先添加「我的收藏」或已同步的频道。" />
          </template>
        </n-data-table>
      </div>
    </div>
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
  justify-content: space-between;
  gap: 16px;
}
.toolbar-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}
.interval {
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
  white-space: nowrap;
}
.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
.panel {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  width: 100%;
  gap: 12px;
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
.message-table {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
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
@media (max-width: 1000px) {
  .composer.row {
    flex-direction: column;
  }
  .composer.row > .n-button {
    align-self: flex-end;
  }
}
</style>
