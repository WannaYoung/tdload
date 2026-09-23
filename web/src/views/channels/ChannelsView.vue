<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  NButton,
  NDataTable,
  NEmpty,
  NInput,
  NModal,
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
const adding = ref(false);
const addOpen = ref(false);
const addChat = ref("");
const syncingId = ref<number | null>(null);
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
  custom: {
    label: "自定义",
    color: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
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

function isCustomRow(r: ChannelRow) {
  return Boolean(r.isCustom || r.custom || r.kind === "custom");
}

function openDetail(row: ChannelRow) {
  void router.push({ name: "channel-detail", params: { chatId: String(row.chatId) } });
}

function coveragePct(r: ChannelRow) {
  const l = r.lastMessageId || 0;
  const c = r.scanCursor ?? r.lastDownloadedMessageId ?? 0;
  if (l <= 0) return 0;
  return Math.min(100, Math.round((c / l) * 100));
}

function openAddModal() {
  addChat.value = "";
  addOpen.value = true;
}

async function submitAdd() {
  const chat = addChat.value.trim();
  if (!chat) {
    message.warning("请输入 @名称 或频道 ID");
    return;
  }
  adding.value = true;
  try {
    await api("/api/channels", {
      method: "POST",
      body: JSON.stringify({ chat }),
    });
    message.success("已添加自定义频道");
    addOpen.value = false;
    page.value = 1;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "添加失败");
  } finally {
    adding.value = false;
  }
}

async function syncChannel(row: ChannelRow) {
  syncingId.value = row.chatId;
  try {
    await api(`/api/channels/${row.chatId}/sync`, { method: "POST", body: "{}" });
    message.success("已同步最新消息");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "同步失败");
  } finally {
    syncingId.value = null;
  }
}

const columns = computed<DataTableColumns<ChannelRow>>(() => [
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
      const kind = isCustomRow(r) ? "custom" : r.kind;
      const meta = kindMeta[kind] || {
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
    width: 148,
    align: "right",
    render: (r) =>
      h("div", { class: "row-actions" }, [
        h(
          NButton,
          {
            size: "small",
            secondary: true,
            loading: syncingId.value === r.chatId,
            onClick: (e: MouseEvent) => {
              e.stopPropagation();
              void syncChannel(r);
            },
          },
          { default: () => "同步" },
        ),
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
      ]),
  },
]);

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
      <div class="head-actions">
        <n-button type="primary" secondary @click="openAddModal">新增</n-button>
        <n-button secondary :loading="loading" @click="load">刷新</n-button>
      </div>
    </header>
    <n-spin :show="loading">
      <n-empty v-if="!rows.length && !loading" description="暂无数据，去 Telegram 页同步或新增自定义频道" />
      <n-data-table v-else :columns="columns" :data="rows" :bordered="false" size="small" />
      <div v-if="total > pageSize" class="pager">
        <n-button size="small" :disabled="page <= 1" @click="page--; load()">上一页</n-button>
        <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">下一页</n-button>
      </div>
    </n-spin>

    <n-modal
      v-model:show="addOpen"
      preset="card"
      title="新增频道"
      style="width: min(420px, 92vw)"
      :mask-closable="!adding"
      :closable="!adding"
    >
      <p class="add-hint">输入公开频道的 @名称 或频道 ID，解析后会出现在已加入频道之后。</p>
      <n-input
        v-model:value="addChat"
        placeholder="@channel 或 100…"
        :disabled="adding"
        @keyup.enter="submitAdd"
      />
      <template #footer>
        <div class="modal-actions">
          <n-button :disabled="adding" @click="addOpen = false">取消</n-button>
          <n-button type="primary" :loading="adding" @click="submitAdd">添加</n-button>
        </div>
      </template>
    </n-modal>
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
.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
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
.add-hint {
  margin: 0 0 12px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
  line-height: 1.5;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
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
:deep(.row-actions) {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}
</style>
