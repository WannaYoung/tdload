<script setup lang="ts">
import { onMounted, ref } from "vue";
import { NButton, NDataTable, NEmpty, NSpin, useMessage, type DataTableColumns } from "naive-ui";
import { api } from "../api/http";
import type { ChannelRow } from "../api/types";

const message = useMessage();
const loading = ref(false);
const page = ref(1);
const pageSize = 20;
const total = ref(0);
const rows = ref<ChannelRow[]>([]);

const kindLabel: Record<string, string> = {
  channel: "频道",
  supergroup: "超级群",
  group: "群",
};

const columns: DataTableColumns<ChannelRow> = [
  { title: "名称", key: "title", ellipsis: { tooltip: true } },
  {
    title: "类型",
    key: "kind",
    width: 88,
    render: (r) => kindLabel[r.kind] || r.kind,
  },
  {
    title: "消息数",
    key: "messageCount",
    width: 88,
    render: (r) => (r.messageCount >= 0 ? r.messageCount : "—"),
  },
  { title: "已下载", key: "downloadedCount", width: 88 },
  { title: "最新 msg", key: "lastMessageId", width: 96 },
  { title: "已下到 msg", key: "lastDownloadedMessageId", width: 108 },
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
</style>
