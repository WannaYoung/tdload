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
import { api } from "../api/http";
import type { ChannelRow } from "../api/types";

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

function createTask(row: ChannelRow) {
  void router.push({
    name: "tasks",
    query: { tab: "channel", chatId: String(row.chatId) },
  });
}

const columns: DataTableColumns<ChannelRow> = [
  {
    title: "名称",
    key: "title",
    ellipsis: { tooltip: true },
    render: (r) => {
      const name = r.title || String(r.chatId);
      const uname = r.username ? `@${r.username}` : "";
      return h("div", { class: "name-cell" }, [
        h("div", { class: "name-title" }, name),
        uname ? h("div", { class: "name-sub" }, uname) : null,
      ]);
    },
  },
  {
    title: "类型",
    key: "kind",
    width: 100,
    render: (r) => {
      const meta = kindMeta[r.kind] || {
        label: r.kind || "未知",
        color: { color: "rgba(255,255,255,0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
      };
      return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
    },
  },
  {
    title: "最新消息",
    key: "lastMessageId",
    width: 110,
    render: (r) => (r.lastMessageId > 0 ? String(r.lastMessageId) : "—"),
  },
  {
    title: "已下载",
    key: "downloadedCount",
    width: 96,
    render: (r) => String(r.downloadedCount ?? 0),
  },
  {
    title: "操作",
    key: "actions",
    width: 110,
    align: "right",
    render: (r) =>
      h(
        NButton,
        { size: "small", type: "primary", secondary: true, onClick: () => createTask(r) },
        { default: () => "新增任务" },
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
        <p>已加入的频道与群组。请先在 Telegram 页点击「同步对话」。</p>
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
.head p {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
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
</style>
