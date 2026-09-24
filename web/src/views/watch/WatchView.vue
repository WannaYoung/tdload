<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
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

const { t } = useI18n();
const message = useMessage();
const isMobile = useMobile();
const loading = ref(false);
const adding = ref(false);
const rows = ref<WatchRow[]>([]);
const candidates = ref<WatchCandidate[]>([]);
const selectedChatId = ref<number | null>(null);
const selectedContentType = ref<ContentType>("all");
const intervalMinutes = ref(30);

const contentTypeOptions = computed<SelectOption[]>(() => [
  { label: t("contentType.all"), value: "all" },
  { label: t("contentType.media"), value: "media" },
  { label: t("contentType.image"), value: "image" },
  { label: t("contentType.video"), value: "video" },
]);

const kindMeta = computed<
  Record<string, { label: string; color: { color: string; textColor: string; borderColor: string } }>
>(() => ({
  saved: {
    label: t("kind.saved"),
    color: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  },
  custom: {
    label: t("kind.custom"),
    color: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  },
  channel: {
    label: t("kind.channel"),
    color: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
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
  const meta = kindMeta.value[kind];
  if (!meta) return null;
  return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
}

const contentTypeMeta = computed<
  Record<string, { label: string; color: { color: string; textColor: string; borderColor: string } }>
>(() => ({
  all: {
    label: t("contentType.all"),
    color: {
      color: "rgba(255, 255, 255, 0.1)",
      textColor: "rgba(255, 255, 255, 0.72)",
      borderColor: "transparent",
    },
  },
  media: {
    label: t("contentType.media"),
    color: {
      color: "rgba(244, 114, 182, 0.16)",
      textColor: "#f9a8d4",
      borderColor: "transparent",
    },
  },
  image: {
    label: t("contentType.image"),
    color: {
      color: "rgba(251, 191, 36, 0.16)",
      textColor: "#fbbf24",
      borderColor: "transparent",
    },
  },
  video: {
    label: t("contentType.video"),
    color: {
      color: "rgba(96, 165, 250, 0.16)",
      textColor: "#93c5fd",
      borderColor: "transparent",
    },
  },
}));

function renderContentType(ct?: string) {
  const meta = contentTypeMeta.value[ct || "all"] || contentTypeMeta.value.all;
  return h(NTag, { size: "small", bordered: false, color: meta.color }, { default: () => meta.label });
}

let timer: number | null = null;

function formatTime(v?: string) {
  if (!v) return t("common.dash");
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

const columns = computed<DataTableColumns<WatchRow>>(() => [
  {
    title: t("watch.channel"),
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
    title: t("watch.contentType"),
    key: "contentType",
    width: 96,
    render: (r) => renderContentType(r.contentType),
  },
  {
    title: t("watch.downloaded"),
    key: "downloadedCount",
    width: 90,
    render: (r) => String(r.downloadedCount ?? 0),
  },
  {
    title: t("watch.lastMessage"),
    key: "lastMessageId",
    width: 110,
    render: (r) => (r.lastMessageId > 0 ? String(r.lastMessageId) : t("common.dash")),
  },
  {
    title: t("watch.lastRun"),
    key: "lastRunAt",
    width: 160,
    render: (r) => formatTime(r.lastRunAt),
  },
  {
    title: t("watch.nextRun"),
    key: "nextRunAt",
    width: 160,
    render: (r) => formatTime(r.nextRunAt),
  },
  {
    title: t("watch.actions"),
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
          onClick: () => void remove(r),
        },
        { icon: () => h(NIcon, { component: TrashOutline }) },
      ),
  },
]);

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
    if (!silent) message.error(e instanceof Error ? e.message : t("common.loadFailed"));
  } finally {
    if (!silent) loading.value = false;
  }
}

async function addWatch() {
  if (selectedChatId.value == null) {
    message.warning(t("watch.selectChannel"));
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
    message.success(t("watch.addSuccess"));
    selectedChatId.value = null;
    selectedContentType.value = "all";
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.addFailed"));
  } finally {
    adding.value = false;
  }
}

async function remove(row: WatchRow) {
  try {
    await api(`/api/watch/${row.id}`, { method: "DELETE" });
    message.success(t("watch.removeSuccess"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.deleteFailed"));
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
        <h2>{{ t("watch.title") }}</h2>
        <span class="interval">{{ t("watch.interval", { n: intervalMinutes }) }}</span>
      </div>
      <div class="toolbar-actions">
        <n-button :loading="loading" @click="load(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          {{ t("common.refresh") }}
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
          :placeholder="t('watch.selectPlaceholder')"
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
          {{ t("common.add") }}
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
            <n-empty :description="t('watch.empty')" />
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
