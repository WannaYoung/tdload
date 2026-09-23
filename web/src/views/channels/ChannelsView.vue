<script setup lang="ts">
import { h, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  NButton,
  NDataTable,
  NEmpty,
  NSpin,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { api } from "../../api/http";
import type { ChannelRow } from "../../api/types";
import ChannelCoverageBar from "./components/ChannelCoverageBar.vue";

const router = useRouter();
const message = useMessage();
const loading = ref(false);
const page = ref(1);
const pageSize = 20;
const total = ref(0);
const rows = ref<ChannelRow[]>([]);

const kindMeta: Record<
  string,
  { label: string; color: { color: string; textColor: string; borderColor: string } }
> = {
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

const statusMeta: Record<string, { label: string; type: "default" | "success" | "info" | "warning" | "error" }> = {
  idle: { label: "可继续", type: "info" },
  running: { label: "下载中", type: "success" },
  caught_up: { label: "已追平", type: "default" },
  has_failed: { label: "有失败", type: "warning" },
};

function openDetail(row: ChannelRow) {
  void router.push({ name: "channel-detail", params: { chatId: String(row.chatId) } });
}

function coveragePct(r: ChannelRow) {
  const l = r.lastMessageId || 0;
  const c = r.scanCursor ?? r.lastDownloadedMessageId ?? 0;
  if (l <= 0) return 0;
  return Math.min(100, Math.round((c / l) * 100));
}

const columns: DataTableColumns<ChannelRow> = [
  {
    title: "名称",
    key: "title",
    ellipsis: { tooltip: true },
    render: (r) => {
      const name = r.title || String(r.chatId);
      const uname = r.username ? `@${r.username}` : "";
      return h(
        "button",
        { class: "name-btn", type: "button", onClick: () => openDetail(r) },
        [
          h("div", { class: "name-title" }, name),
          uname ? h("div", { class: "name-sub" }, uname) : null,
        ],
      );
    },
  },
  {
    title: "类型",
    key: "kind",
    width: 90,
    render: (r) => {
      const meta = kindMeta[r.kind] || {
        label: r.kind || "未知",
        color: { color: "rgba(255,255,255,0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
      };
      return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
    },
  },
  {
    title: "覆盖",
    key: "coverage",
    width: 160,
    render: (r) => {
      const c = r.scanCursor ?? r.lastDownloadedMessageId ?? 0;
      const l = r.lastMessageId || 0;
      return h(ChannelCoverageBar, {
        percent: coveragePct(r),
        text: l > 0 ? `#${c} / #${l}` : `水位 #${c}`,
      });
    },
  },
  {
    title: "已下载",
    key: "downloadedCount",
    width: 88,
    render: (r) => String(r.downloadedCount ?? 0),
  },
  {
    title: "状态",
    key: "status",
    width: 90,
    render: (r) => {
      const meta = statusMeta[r.status || "idle"] || statusMeta.idle;
      return h(NTag, { size: "small", bordered: false, type: meta.type }, { default: () => meta.label });
    },
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
          type: "primary",
          secondary: true,
          onClick: () => openDetail(r),
        },
        { default: () => "下载" },
      ),
  },
];

async function load() {
  loading.value = true;
  try {
    const data = await api<{ items: ChannelRow[]; total: number }>(
      `/api/channels?page=${page.value}&pageSize=${pageSize}`,
    );
    rows.value = data.items || [];
    total.value = data.total || 0;
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

onMounted(() => void load());
</script>

<template>
  <div class="page">
    <header class="head">
      <div>
        <h2>频道</h2>
      </div>
      <n-button secondary :loading="loading" @click="load">刷新</n-button>
    </header>
    <n-spin :show="loading">
      <n-empty v-if="!rows.length && !loading" description="暂无数据，去 Telegram 页同步" />
      <n-data-table v-else :columns="columns" :data="rows" :bordered="false" size="small" />
      <div v-if="total > pageSize" class="pager">
        <n-button size="small" :disabled="page <= 1" @click="page--; load()">上一页</n-button>
        <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">下一页</n-button>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.head h2 {
  margin: 0;
  font-size: 22px;
}
.pager {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
  justify-content: flex-end;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
}
:deep(.name-btn) {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  cursor: pointer;
  min-width: 0;
  color: inherit;
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
</style>
