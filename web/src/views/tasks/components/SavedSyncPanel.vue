<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
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

export type SavedTask = {
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

const props = defineProps<{
  active: boolean;
}>();

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

function savedTotal(t: SavedTask) {
  const c = t.itemCounts;
  if (c) {
    const n = c.pending + c.downloading + c.done + c.skipped + c.failed;
    if (n > 0) return n;
  }
  return t.progressTotal || t.totalFiles || 0;
}

function savedDone(t: SavedTask) {
  if (t.itemCounts) return t.itemCounts.done + t.itemCounts.skipped;
  return t.progressDone || t.doneFiles || 0;
}

function savedFailed(t: SavedTask) {
  return t.itemCounts?.failed ?? 0;
}

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    const page = await api<{ items: SavedTask[] }>("/api/tasks?kind=saved&page=1&pageSize=50");
    tasks.value = page.items || [];
  } catch (e) {
    if (!silent) message.error(e instanceof Error ? e.message : "加载失败");
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
    message.success("已开始同步");
    const rest = tasks.value.filter((t) => t.id !== task.id);
    tasks.value = [task, ...rest];
  } catch (e) {
    message.error(e instanceof Error ? e.message : "创建失败");
  } finally {
    submitting.value = false;
  }
}

async function deleteTask(id: number) {
  try {
    await api(`/api/tasks/${id}/cancel`, { method: "POST", body: "{}" }).catch(() => undefined);
    await api(`/api/tasks/${id}`, { method: "DELETE" });
    message.success("已删除");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
  }
}

async function clearCompleted() {
  clearing.value = true;
  try {
    const res = await api<{ cleared: number }>("/api/tasks/completed?kind=saved", { method: "DELETE" });
    message.success(res.cleared ? `已清除 ${res.cleared} 条记录` : "没有可清除的已结束记录");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "清除失败");
  } finally {
    clearing.value = false;
  }
}

function applyProgress(ev: {
  taskId?: number;
  kind?: string;
  type?: string;
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
  if (ev.done != null) {
    row.progressDone = ev.done;
    row.doneFiles = ev.done;
  }
  if (ev.total != null) {
    row.progressTotal = ev.total;
    row.totalFiles = ev.total;
  }
  if (ev.status && ev.type !== "task_item_progress") {
    row.status = ev.status;
  }
  if (ev.itemCounts) row.itemCounts = { ...ev.itemCounts };
  tasks.value.splice(idx, 1, row);
  return true;
}

const columns = computed<DataTableColumns<SavedTask>>(() => [
  { title: "收藏数", key: "total", width: 100, render: (r) => String(savedTotal(r)) },
  { title: "已同步", key: "done", width: 100, render: (r) => String(savedDone(r)) },
  { title: "同步失败", key: "failed", width: 100, render: (r) => String(savedFailed(r)) },
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
          <n-empty description="暂无同步记录" />
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
