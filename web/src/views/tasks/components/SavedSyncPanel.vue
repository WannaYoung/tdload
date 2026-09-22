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

async function load() {
  loading.value = true;
  try {
    const page = await api<{ items: SavedTask[] }>("/api/tasks?kind=saved&page=1&pageSize=50");
    tasks.value = page.items || [];
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function createTask() {
  submitting.value = true;
  try {
    await api("/api/tasks", { method: "POST", body: JSON.stringify({ source: "saved_all" }) });
    message.success("已开始同步");
    await load();
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
  done?: number;
  total?: number;
  status?: string;
  itemCounts?: ItemCounts;
}) {
  const saved = tasks.value.find((x) => x.id === ev.taskId);
  if (!saved) return false;
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

defineExpose({ reload: load, loading, applyProgress });
</script>

<template>
  <div class="panel">
    <div class="list-meta">
      <span class="section-title">同步记录</span>
      <div class="meta-actions">
        <n-button
          size="small"
          type="primary"
          secondary
          :loading="clearing"
          :disabled="completedCount <= 0"
          @click="clearCompleted"
        >
          清除完成
        </n-button>
        <n-button size="small" type="primary" secondary :loading="submitting" @click="createTask">
          开始同步
        </n-button>
      </div>
    </div>
    <div class="table-wrap message-table">
      <n-empty v-if="!tasks.length && !loading" description="暂无同步记录" />
      <n-data-table
        v-else
        flex-height
        :columns="columns"
        :data="tasks"
        :bordered="false"
        size="small"
        :row-key="(r: SavedTask) => r.id"
      />
    </div>
  </div>
</template>

<style scoped>
.panel {
  display: contents;
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
.meta-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
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
</style>
