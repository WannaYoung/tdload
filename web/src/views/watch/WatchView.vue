<script setup lang="ts">
import { h, onMounted, onUnmounted, ref } from "vue";
import {
  NButton,
  NDataTable,
  NEmpty,
  NIcon,
  NSelect,
  NTag,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from "naive-ui";
import { AddOutline, RefreshOutline, TrashOutline } from "@vicons/ionicons5";
import { api } from "../../api/http";
import { useMobile } from "../../composables/useMobile";
import type { WatchCandidate, WatchRow } from "../../api/types";

defineOptions({ name: "WatchView" });

type ContentType = "all" | "media" | "image" | "video";

const message = useMessage();
const isMobile = useMobile();
const loading = ref(false);
const adding = ref(false);
const rows = ref<WatchRow[]>([]);
const candidates = ref<WatchCandidate[]>([]);
const selectedChatId = ref<number | null>(null);
const selectedContentType = ref<ContentType>("all");
const intervalMinutes = ref(30);

const contentTypeOptions: SelectOption[] = [
  { label: "全部", value: "all" },
  { label: "媒体", value: "media" },
  { label: "图片", value: "image" },
  { label: "视频", value: "video" },
];

const kindMeta: Record<
  string,
  { label: string; color: { color: string; textColor: string; borderColor: string } }
> = {
  saved: {
    label: "收藏",
    color: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  },
  custom: {
    label: "自定义",
    color: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  },
  channel: {
    label: "频道",
    color: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  },
  supergroup: {
    label: "超级群",
    color: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  },
  group: {
    label: "群组",
    color: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
  },
};

function kindOfCandidate(c: WatchCandidate) {
  if (c.kind === "saved") return "saved";
  if (c.isCustom || c.kind === "custom") return "custom";
  return c.kind || "channel";
}

function kindOfRow(r: WatchRow) {
  if (r.isFavorites) return "saved";
  if (r.isCustom || r.kind === "custom") return "custom";
  return r.kind || "channel";
}

function renderKindTag(kind: string) {
  const meta = kindMeta[kind];
  if (!meta) return null;
  return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
}

const contentTypeMeta: Record<
  string,
  { label: string; color: { color: string; textColor: string; borderColor: string } }
> = {
  all: {
    label: "全部",
    color: {
      color: "rgba(255, 255, 255, 0.1)",
      textColor: "rgba(255, 255, 255, 0.72)",
      borderColor: "transparent",
    },
  },
  media: {
    label: "媒体",
    color: {
      color: "rgba(244, 114, 182, 0.16)",
      textColor: "#f9a8d4",
      borderColor: "transparent",
    },
  },
  image: {
    label: "图片",
    color: {
      color: "rgba(251, 191, 36, 0.16)",
      textColor: "#fbbf24",
      borderColor: "transparent",
    },
  },
  video: {
    label: "视频",
    color: {
      color: "rgba(96, 165, 250, 0.16)",
      textColor: "#93c5fd",
      borderColor: "transparent",
    },
  },
};

function renderContentType(ct?: string) {
  const meta = contentTypeMeta[ct || "all"] || contentTypeMeta.all;
  return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
}

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
    label: c.title,
    value: c.chatId,
    kind: kindOfCandidate(c),
  }));

function renderSelectLabel(option: { label?: string; value?: string | number; kind?: string }) {
  return h("div", { class: "opt-row" }, [
    h("span", { class: "opt-title" }, String(option.label || option.value || "")),
    option.kind ? renderKindTag(option.kind) : null,
  ]);
}

const columns: DataTableColumns<WatchRow> = [
  {
    title: "频道",
    key: "chatTitle",
    ellipsis: { tooltip: true },
    render: (r) =>
      h("div", { class: "name-cell" }, [
        h("div", { class: "name-title-row" }, [
          h("span", { class: "name-title" }, r.chatTitle || String(r.chatId)),
          renderKindTag(kindOfRow(r)),
        ]),
      ]),
  },
  {
    title: "内容类型",
    key: "contentType",
    width: 96,
    render: (r) => renderContentType(r.contentType),
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
    width: 72,
    align: "right",
    render: (r) =>
      h(
        NButton,
        {
          size: "tiny",
          secondary: true,
          type: "error",
          title: "删除",
          onClick: () => void remove(r),
        },
        { icon: () => h(NIcon, { component: TrashOutline }) },
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
      body: JSON.stringify({
        chatId: selectedChatId.value,
        contentType: selectedContentType.value,
      }),
    });
    message.success("已加入监听");
    selectedChatId.value = null;
    selectedContentType.value = "all";
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
        <n-button :loading="loading" @click="load(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
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
          :render-label="renderSelectLabel"
          :disabled="loading || !candidates.length"
        />
        <n-select
          v-model:value="selectedContentType"
          class="composer-type"
          :options="contentTypeOptions"
          :disabled="loading"
        />
        <n-button type="primary" :loading="adding" :disabled="selectedChatId == null" @click="addWatch">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          添加
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
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  flex-shrink: 0;
  width: auto;
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
  flex-wrap: nowrap;
  align-items: flex-start;
}
.composer-input {
  flex: 1 1 auto;
  min-width: 0;
}
.composer-type {
  flex: 0 0 110px;
  width: 110px;
}
.composer.row > .n-button {
  flex-shrink: 0;
  white-space: nowrap;
}
.message-table {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}
:deep(.name-cell) {
  min-width: 0;
}
:deep(.name-title-row),
:deep(.opt-row) {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
:deep(.name-title),
:deep(.opt-title) {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.9);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
