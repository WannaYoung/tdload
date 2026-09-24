<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  NButton,
  NDataTable,
  NEmpty,
  NIcon,
  NInput,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import {
  DocumentOutline,
  DownloadOutline,
  ImageOutline,
  MusicalNotesOutline,
  TrashOutline,
  VideocamOutline,
} from "@vicons/ionicons5";
import { api } from "../../../api/http";
import type { TaskItemRow } from "../../../api/types";

const props = defineProps<{
  active: boolean;
}>();

const { t } = useI18n();
const message = useMessage();
const loading = ref(false);
const submitting = ref(false);
const clearing = ref(false);
const text = ref("");
const items = ref<TaskItemRow[]>([]);
const total = ref(0);
const doneCount = ref(0);
const page = ref(1);
const pageSize = 50;

const statusLabel = computed<Record<string, string>>(() => ({
  pending: t("status.pending"),
  downloading: t("status.downloading"),
  running: t("status.running"),
  queued: t("status.queued"),
  done: t("status.done"),
  skipped: t("status.skipped"),
  failed: t("status.failed"),
  paused: t("status.paused"),
  cancelled: t("status.cancelled"),
}));

const statusTagColor: Record<string, { color: string; textColor: string; borderColor: string }> = {
  downloading: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  running: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  pending: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  queued: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  done: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  skipped: { color: "rgba(255, 255, 255, 0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
  failed: { color: "rgba(248, 113, 113, 0.16)", textColor: "#fca5a5", borderColor: "transparent" },
  paused: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
  cancelled: { color: "rgba(255, 255, 255, 0.08)", textColor: "rgba(255,255,255,0.45)", borderColor: "transparent" },
};

const pinkTag = {
  color: "rgba(244, 114, 182, 0.16)",
  textColor: "#f9a8d4",
  borderColor: "transparent",
};

function renderStatus(status: string) {
  const color = statusTagColor[status] || {
    color: "rgba(255,255,255,0.08)",
    textColor: "rgba(255,255,255,0.55)",
    borderColor: "transparent",
  };
  return h(
    NTag,
    { size: "small", bordered: false, color },
    { default: () => statusLabel.value[status] || status },
  );
}

/** Normalize backend Chinese labels or English codes to a stable key. */
function resolveMediaKindKey(kind?: string): "image" | "video" | "audio" | "file" | null {
  if (!kind || kind === "—" || kind === "-") return null;
  const lower = kind.toLowerCase();
  if (lower === "image" || kind === "图片") return "image";
  if (lower === "video" || kind === "视频") return "video";
  if (lower === "audio" || kind === "音频") return "audio";
  if (lower === "file" || kind === "文件") return "file";
  return "file";
}

function mediaIcon(kind?: string) {
  switch (resolveMediaKindKey(kind)) {
    case "image":
      return ImageOutline;
    case "video":
      return VideocamOutline;
    case "audio":
      return MusicalNotesOutline;
    default:
      return DocumentOutline;
  }
}

function mediaKindDisplay(kind?: string) {
  const key = resolveMediaKindKey(kind);
  if (key == null) return t("common.dash");
  return t(`mediaKind.${key}`);
}

function renderMediaKind(kind?: string) {
  const label = mediaKindDisplay(kind);
  return h(
    NTag,
    { size: "small", bordered: false, color: pinkTag, title: label },
    { default: () => h(NIcon, { size: 16, component: mediaIcon(kind) }) },
  );
}

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    const data = await api<{ items: TaskItemRow[]; total: number; doneCount: number }>(
      `/api/tasks/items?kind=message&page=${page.value}&pageSize=${pageSize}`,
    );
    items.value = data.items || [];
    total.value = data.total || 0;
    doneCount.value = data.doneCount || 0;
  } catch (e) {
    if (!silent) message.error(e instanceof Error ? e.message : t("common.loadFailed"));
  } finally {
    if (!silent) loading.value = false;
  }
}

/** 就地更新单条消息项状态，避免整表刷新 */
function applyItemProgress(ev: {
  kind?: string;
  taskId?: number;
  chatId?: number;
  messageId?: number;
  status?: string;
  title?: string;
}) {
  if (ev.kind && ev.kind !== "message") return false;
  if (ev.messageId == null || ev.messageId <= 0) return false;
  const idx = items.value.findIndex(
    (it) =>
      it.messageId === ev.messageId &&
      (ev.chatId == null || ev.chatId === 0 || it.chatId === ev.chatId) &&
      (ev.taskId == null || it.taskId === ev.taskId),
  );
  if (idx < 0) return false;
  const row = { ...items.value[idx] };
  if (ev.status) row.status = ev.status;
  if (ev.title) row.fileName = ev.title;
  items.value.splice(idx, 1, row);
  return true;
}

async function deleteItem(id: number) {
  try {
    await api(`/api/tasks/items/${id}`, { method: "DELETE" });
    message.success(t("messageDownload.deleted"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.deleteFailed"));
  }
}

async function clearCompleted() {
  clearing.value = true;
  try {
    const res = await api<{ cleared: number }>("/api/tasks/items/completed?kind=message", { method: "DELETE" });
    message.success(
      res.cleared
        ? t("messageDownload.cleared", { n: res.cleared })
        : t("messageDownload.nothingToClear"),
    );
    page.value = 1;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.clearFailed"));
  } finally {
    clearing.value = false;
  }
}

async function createTask() {
  submitting.value = true;
  try {
    await api("/api/tasks", { method: "POST", body: JSON.stringify({ text: text.value, source: "url" }) });
    text.value = "";
    message.success(t("messageDownload.enqueued"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.createFailed"));
  } finally {
    submitting.value = false;
  }
}

const columns = computed<DataTableColumns<TaskItemRow>>(() => [
  {
    title: t("messageDownload.channel"),
    key: "chatTitle",
    ellipsis: { tooltip: true },
    render: (r) => r.chatTitle || String(r.chatId || t("common.dash")),
  },
  { title: t("messageDownload.messageId"), key: "messageId", width: 100 },
  {
    title: t("messageDownload.name"),
    key: "fileName",
    ellipsis: { tooltip: true },
    render: (r) => r.fileName || t("common.dash"),
  },
  {
    title: t("messageDownload.type"),
    key: "mediaKind",
    width: 64,
    align: "center",
    render: (r) => renderMediaKind(r.mediaKind),
  },
  {
    title: t("messageDownload.statusCol"),
    key: "status",
    width: 96,
    render: (r) => renderStatus(r.status),
  },
  {
    title: t("messageDownload.actions"),
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
          title: t("common.delete"),
          onClick: () => void deleteItem(r.id),
        },
        { icon: () => h(NIcon, { component: TrashOutline }) },
      ),
  },
]);

watch(
  () => props.active,
  (v) => {
    if (v) void load();
  },
  { immediate: true },
);

defineExpose({
  reload: (silent?: boolean) => load(!!silent),
  loading,
  applyItemProgress,
});
</script>

<template>
  <div class="panel">
    <div class="composer row">
      <n-input
        v-model:value="text"
        type="textarea"
        :autosize="{ minRows: 1, maxRows: 8 }"
        :placeholder="t('messageDownload.placeholder')"
        class="composer-input"
      />
      <n-button type="primary" :loading="submitting" :disabled="!text.trim()" @click="createTask">
        <template #icon>
          <n-icon :component="DownloadOutline" />
        </template>
        {{ t("messageDownload.download") }}
      </n-button>
    </div>
    <div class="list-meta">
      <span>{{ t("messageDownload.listMeta", { total, done: doneCount }) }}</span>
      <n-button
        size="small"
        type="error"
        secondary
        :loading="clearing"
        :disabled="doneCount <= 0"
        @click="clearCompleted"
      >
        {{ t("messageDownload.clearCompleted") }}
      </n-button>
    </div>
    <div class="table-wrap message-table">
      <n-data-table
        :columns="columns"
        :data="items"
        :bordered="false"
        size="small"
        :loading="loading"
        :row-key="(r: TaskItemRow) => r.id"
      >
        <template #empty>
          <n-empty :description="t('messageDownload.empty')" />
        </template>
      </n-data-table>
    </div>
    <div v-if="total > pageSize" class="pager">
      <n-button size="small" :disabled="page <= 1" @click="page--; load()">{{ t("common.prevPage") }}</n-button>
      <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
      <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">{{ t("common.nextPage") }}</n-button>
    </div>
  </div>
</template>

<style scoped>
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
.composer.row > .n-button {
  flex-shrink: 0;
  white-space: nowrap;
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
.message-table {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}
.pager {
  flex-shrink: 0;
}
</style>
