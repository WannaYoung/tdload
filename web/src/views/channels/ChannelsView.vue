<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import {
  NButton,
  NDataTable,
  NEmpty,
  NIcon,
  NInput,
  NModal,
  NSpin,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { AddOutline, DownloadOutline, RefreshOutline, SyncOutline } from "@vicons/ionicons5";
import { api } from "../../api/http";
import type { ChannelRow } from "../../api/types";
import ChannelCoverageBar from "./components/ChannelCoverageBar.vue";

const { t } = useI18n();
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

const kindMeta = computed<
  Record<string, { label: string; color: { color: string; textColor: string; borderColor: string } }>
>(() => ({
  channel: {
    label: t("kind.channel"),
    color: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  },
  custom: {
    label: t("kind.custom"),
    color: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  },
  supergroup: {
    label: t("kind.supergroup"),
    color: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  },
  group: {
    label: t("kind.group"),
    color: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
  },
}));

const statusMeta = computed<Record<string, { label: string; color: string }>>(() => ({
  idle: { label: t("status.idle"), color: "#93c5fd" },
  running: { label: t("status.running"), color: "var(--td-pink)" },
  caught_up: { label: t("status.caughtUp"), color: "#86efac" },
  has_failed: { label: t("status.hasFailed"), color: "#f87171" },
}));

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
    message.warning(t("channels.needChat"));
    return;
  }
  adding.value = true;
  try {
    await api("/api/channels", {
      method: "POST",
      body: JSON.stringify({ chat }),
    });
    message.success(t("channels.addSuccess"));
    addOpen.value = false;
    page.value = 1;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.addFailed"));
  } finally {
    adding.value = false;
  }
}

async function syncChannel(row: ChannelRow) {
  syncingId.value = row.chatId;
  try {
    await api(`/api/channels/${row.chatId}/sync`, { method: "POST", body: "{}" });
    message.success(t("channels.syncSuccess"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.syncFailed"));
  } finally {
    syncingId.value = null;
  }
}

const columns = computed<DataTableColumns<ChannelRow>>(() => [
  {
    title: t("channels.name"),
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
    title: t("channels.kind"),
    key: "kind",
    width: 90,
    render: (r) => {
      const kind = isCustomRow(r) ? "custom" : r.kind;
      const meta = kindMeta.value[kind] || {
        label: r.kind || t("kind.unknown"),
        color: { color: "rgba(255,255,255,0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
      };
      return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
    },
  },
  {
    title: t("channels.coverage"),
    key: "coverage",
    width: 160,
    render: (r) => {
      const c = r.scanCursor ?? r.lastDownloadedMessageId ?? 0;
      const l = r.lastMessageId || 0;
      return h(ChannelCoverageBar, {
        percent: coveragePct(r),
        text:
          l > 0
            ? t("channels.coverageProgress", { cursor: c, last: l })
            : t("channels.coverageWatermark", { cursor: c }),
      });
    },
  },
  {
    title: t("channels.downloaded"),
    key: "downloadedCount",
    width: 88,
    render: (r) => String(r.downloadedCount ?? 0),
  },
  {
    title: t("channels.statusCol"),
    key: "status",
    width: 90,
    render: (r) => {
      const meta = statusMeta.value[r.status || "idle"] || statusMeta.value.idle;
      return h("span", { class: "status-text", style: { color: meta.color } }, meta.label);
    },
  },
  {
    title: t("channels.actions"),
    key: "actions",
    width: 112,
    align: "right",
    render: (r) =>
      h("div", { class: "row-actions" }, [
        h(
          NButton,
          {
            size: "small",
            class: "icon-square",
            loading: syncingId.value === r.chatId,
            title: t("channels.syncTitle"),
            onClick: (e: MouseEvent) => {
              e.stopPropagation();
              void syncChannel(r);
            },
          },
          { icon: () => h(NIcon, { component: SyncOutline }) },
        ),
        h(
          NButton,
          {
            size: "small",
            class: "icon-square",
            type: "primary",
            ghost: true,
            title: t("channels.downloadTitle"),
            onClick: () => openDetail(r),
          },
          { icon: () => h(NIcon, { component: DownloadOutline }) },
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
    message.error(e instanceof Error ? e.message : t("common.loadFailed"));
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
        <h2>{{ t("channels.title") }}</h2>
      </div>
      <div class="head-actions">
        <n-button :loading="loading" @click="load">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          {{ t("common.refresh") }}
        </n-button>
        <n-button type="primary" @click="openAddModal">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          {{ t("channels.add") }}
        </n-button>
      </div>
    </header>
    <n-spin :show="loading">
      <n-empty v-if="!rows.length && !loading" :description="t('channels.empty')" />
      <n-data-table v-else :columns="columns" :data="rows" :bordered="false" size="small" />
      <div v-if="total > pageSize" class="pager">
        <n-button size="small" :disabled="page <= 1" @click="page--; load()">{{ t("common.prevPage") }}</n-button>
        <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">{{ t("common.nextPage") }}</n-button>
      </div>
    </n-spin>

    <n-modal
      v-model:show="addOpen"
      preset="card"
      :title="t('channels.addTitle')"
      style="width: min(420px, 92vw)"
      :mask-closable="!adding"
      :closable="!adding"
    >
      <p class="add-hint">{{ t("channels.addHint") }}</p>
      <n-input
        v-model:value="addChat"
        :placeholder="t('channels.addPlaceholder')"
        :disabled="adding"
        @keyup.enter="submitAdd"
      />
      <template #footer>
        <div class="modal-actions">
          <n-button :disabled="adding" @click="addOpen = false">{{ t("common.cancel") }}</n-button>
          <n-button type="primary" :loading="adding" @click="submitAdd">{{ t("common.add") }}</n-button>
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
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
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
:deep(.status-text) {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
:deep(.row-actions) {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}
:deep(.icon-square) {
  width: 32px;
  height: 32px;
  padding: 0;
}
:deep(.icon-square .n-button__icon) {
  margin: 0;
}
:deep(.icon-square .n-icon) {
  font-size: 18px;
}
</style>
