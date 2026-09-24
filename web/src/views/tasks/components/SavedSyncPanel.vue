<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  NButton,
  NDataTable,
  NEmpty,
  NIcon,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { TrashOutline } from "@vicons/ionicons5";
import { api } from "../../../api/http";
import type { ItemCounts } from "../../../api/types";
import { dateLocaleTag } from "../../../i18n";

export type SavedTask = {
  id: number;
  title: string;
  status: string;
  source?: string;
  kind?: string;
  chatId?: number;
  phase?: string;
  doneFiles: number;
  totalFiles: number;
  progressDone: number;
  progressTotal: number;
  itemCounts?: ItemCounts;
  error: string;
  createdAt: string;
};

const props = defineProps<{
  active: boolean;
}>();

const { t } = useI18n();
const message = useMessage();
const loading = ref(false);
const submitting = ref(false);
const clearing = ref(false);
const tasks = ref<SavedTask[]>([]);

const completedCount = computed(
  () => tasks.value.filter((t) => t.status === "done" || t.status === "cancelled" || t.status === "failed").length,
);

const hasActiveSync = computed(() =>
  tasks.value.some((t) => t.status === "queued" || t.status === "running" || t.status === "paused"),
);

function formatSyncTime(iso: string) {
  if (!iso) return t("common.dash");
  const ts = Date.parse(iso);
  if (!Number.isFinite(ts)) return iso;
  return new Date(ts).toLocaleString(dateLocaleTag(), {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

/** 拉取收藏列表阶段：收藏数可刷新，已同步显示 — */
function isListing(row: SavedTask) {
  if (row.phase === "listing") return true;
  if (row.phase === "downloading") return false;
  // 兼容：刚创建 / 初始 phase=running，尚未进入 downloading
  return (
    (row.status === "queued" || row.status === "running") &&
    countOr0(row.progressDone) === 0 &&
    countOr0(row.doneFiles) === 0 &&
    !(
      row.itemCounts &&
      (countOr0(row.itemCounts.done) ||
        countOr0(row.itemCounts.skipped) ||
        countOr0(row.itemCounts.failed) ||
        countOr0(row.itemCounts.downloading))
    )
  );
}

function countOr0(n: number | undefined | null) {
  return typeof n === "number" && Number.isFinite(n) ? n : 0;
}

function savedTotal(row: SavedTask) {
  // 收藏数 = 拉取得到的总数；不要用 itemCounts 合计（下载时会随已处理条数一起涨）
  return Math.max(countOr0(row.progressTotal), countOr0(row.totalFiles));
}

function savedDone(row: SavedTask) {
  if (isListing(row)) return null;
  if (row.itemCounts) return countOr0(row.itemCounts.done) + countOr0(row.itemCounts.skipped);
  return countOr0(row.progressDone) || countOr0(row.doneFiles) || 0;
}

function savedFailed(row: SavedTask) {
  if (isListing(row)) return null;
  return countOr0(row.itemCounts?.failed);
}

function displayCount(n: number | null) {
  if (n == null || !Number.isFinite(n)) return t("common.dash");
  return String(n);
}

function coloredCount(n: number | null, color: string) {
  return h("span", { style: { color } }, displayCount(n));
}

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    const page = await api<{ items: SavedTask[] }>("/api/tasks?kind=saved&page=1&pageSize=50");
    tasks.value = page.items || [];
  } catch (e) {
    if (!silent) message.error(e instanceof Error ? e.message : t("common.loadFailed"));
  } finally {
    if (!silent) loading.value = false;
  }
}

async function createTask() {
  submitting.value = true;
  try {
    const task = await api<SavedTask>("/api/tasks", {
      method: "POST",
      body: JSON.stringify({ source: "saved_all" }),
    });
    message.success(t("savedSync.syncStarted"));
    const rest = tasks.value.filter((row) => row.id !== task.id);
    tasks.value = [task, ...rest];
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.createFailed"));
  } finally {
    submitting.value = false;
  }
}

async function deleteTask(id: number) {
  try {
    await api(`/api/tasks/${id}/cancel`, { method: "POST", body: "{}" }).catch(() => undefined);
    await api(`/api/tasks/${id}`, { method: "DELETE" });
    message.success(t("savedSync.deleted"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.deleteFailed"));
  }
}

async function clearCompleted() {
  clearing.value = true;
  try {
    const res = await api<{ cleared: number }>("/api/tasks/completed?kind=saved", { method: "DELETE" });
    message.success(
      res.cleared ? t("savedSync.cleared", { n: res.cleared }) : t("savedSync.nothingToClear"),
    );
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.clearFailed"));
  } finally {
    clearing.value = false;
  }
}

function applyProgress(ev: {
  taskId?: number;
  kind?: string;
  type?: string;
  phase?: string;
  done?: number;
  total?: number;
  status?: string;
  itemCounts?: ItemCounts;
}) {
  if (ev.kind && ev.kind !== "saved") return false;
  // 单条媒体进度事件不要覆盖任务状态（其 status 是 downloading/skipped/done 等条目态）
  if (ev.type === "task_item_progress" && !ev.itemCounts && ev.done == null && ev.total == null) {
    return tasks.value.some((x) => x.id === ev.taskId);
  }
  const idx = tasks.value.findIndex((x) => x.id === ev.taskId);
  if (idx < 0) return false;
  const row = { ...tasks.value[idx] };
  if (ev.phase) {
    row.phase = ev.phase;
    if (ev.phase === "listing") {
      // 拉取阶段不应展示已同步条目进度
      row.itemCounts = undefined;
      row.progressDone = 0;
      row.doneFiles = 0;
    }
  }
  if (ev.done != null && row.phase !== "listing") {
    row.progressDone = ev.done;
    row.doneFiles = ev.done;
  }
  if (ev.total != null) {
    // 总数只增不减，避免下载进度把收藏数改小后再一起涨
    row.progressTotal = Math.max(countOr0(row.progressTotal), ev.total);
    row.totalFiles = Math.max(countOr0(row.totalFiles), ev.total);
  }
  if (ev.status && ev.type !== "task_item_progress") {
    row.status = ev.status;
  }
  if (ev.itemCounts && row.phase !== "listing") {
    row.itemCounts = { ...ev.itemCounts };
  }
  tasks.value.splice(idx, 1, row);
  return true;
}

const columns = computed<DataTableColumns<SavedTask>>(() => [
  { title: t("savedSync.total"), key: "total", width: 100, render: (r) => String(savedTotal(r)) },
  { title: t("savedSync.done"), key: "done", width: 100, render: (r) => coloredCount(savedDone(r), "#86efac") },
  { title: t("savedSync.failed"), key: "failed", width: 100, render: (r) => coloredCount(savedFailed(r), "#fca5a5") },
  {
    title: t("savedSync.createdAt"),
    key: "createdAt",
    ellipsis: { tooltip: true },
    render: (r) => formatSyncTime(r.createdAt),
  },
  {
    title: t("savedSync.actions"),
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
          title: t("savedSync.stopDelete"),
          onClick: () => void deleteTask(r.id),
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
  applyProgress,
  clearCompleted,
  createTask,
  clearing,
  submitting,
  hasActiveSync,
  completedCount,
});
</script>

<template>
  <div class="panel">
    <div class="table-wrap message-table">
      <n-data-table
        :columns="columns"
        :data="tasks"
        :bordered="false"
        size="small"
        :loading="loading"
        :row-key="(r: SavedTask) => r.id"
      >
        <template #empty>
          <n-empty :description="t('savedSync.empty')" />
        </template>
      </n-data-table>
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
}
.message-table {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}
</style>
