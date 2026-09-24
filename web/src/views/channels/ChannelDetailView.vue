<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import {
  NButton,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NIcon,
  NInputNumber,
  NModal,
  NProgress,
  NSelect,
  NSpin,
  NTag,
  useDialog,
  useMessage,
  type SelectOption,
} from "naive-ui";
import {
  CloseOutline,
  DownloadOutline,
  PauseOutline,
  PlayOutline,
  PulseOutline,
  RefreshOutline,
  ReloadOutline,
  SyncOutline,
  TrashOutline,
} from "@vicons/ionicons5";
import { api } from "../../api/http";
import type { ChannelDownloadInfo, ItemCounts } from "../../api/types";
import { useAppEvents } from "../../composables/useAppEvents";

defineOptions({ name: "ChannelDetailView" });

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();

const loading = ref(false);
const submitting = ref(false);
const clearing = ref(false);
const deleting = ref(false);
const cursorSaving = ref(false);
const batchSize = ref(100);
type ContentType = "all" | "media" | "image" | "video";
const contentType = ref<ContentType>("all");
const contentTypeOptions = computed<SelectOption[]>(() => [
  { label: t("contentType.all"), value: "all" },
  { label: t("contentType.media"), value: "media" },
  { label: t("contentType.image"), value: "image" },
  { label: t("contentType.video"), value: "video" },
]);
const info = ref<ChannelDownloadInfo | null>(null);
const cursorModalOpen = ref(false);
const cursorInput = ref<number | null>(null);

let pollTimer: number | null = null;

const chatId = computed(() => {
  const n = Number(route.params.chatId);
  return Number.isFinite(n) ? n : 0;
});

const statusLabel = computed<Record<string, string>>(() => ({
  idle: t("status.idle"),
  running: t("status.running"),
  queued: t("status.queuedLong"),
  paused: t("status.pausedLong"),
  caught_up: t("status.caughtUp"),
  has_failed: t("status.hasFailed"),
}));

/** 与频道列表状态色一致，标签底色用同色半透明 */
const statusTagColor: Record<string, { color: string; textColor: string; borderColor: string }> = {
  idle: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  running: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f472b6", borderColor: "transparent" },
  queued: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  paused: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
  caught_up: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  has_failed: { color: "rgba(248, 113, 113, 0.16)", textColor: "#f87171", borderColor: "transparent" },
};

const taskStatusLabel = computed<Record<string, string>>(() => ({
  running: t("status.running"),
  listing: t("status.listing"),
  queued: t("status.queued"),
  paused: t("status.paused"),
  done: t("status.done"),
  failed: t("status.failed"),
  cancelled: t("status.cancelled"),
}));

const taskStatusTagColor: Record<string, { color: string; textColor: string; borderColor: string }> = {
  listing: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  running: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  downloading: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  queued: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  paused: { color: "rgba(251, 191, 36, 0.16)", textColor: "#fcd34d", borderColor: "transparent" },
  done: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  failed: { color: "rgba(248, 113, 113, 0.16)", textColor: "#fca5a5", borderColor: "transparent" },
  cancelled: { color: "rgba(255, 255, 255, 0.08)", textColor: "rgba(255,255,255,0.45)", borderColor: "transparent" },
};

const countTagColor: Record<string, { color: string; textColor: string; borderColor: string }> = {
  pending: { color: "rgba(96, 165, 250, 0.16)", textColor: "#93c5fd", borderColor: "transparent" },
  downloading: { color: "rgba(244, 114, 182, 0.16)", textColor: "#f9a8d4", borderColor: "transparent" },
  done: { color: "rgba(74, 222, 128, 0.14)", textColor: "#86efac", borderColor: "transparent" },
  skipped: { color: "rgba(255, 255, 255, 0.08)", textColor: "rgba(255,255,255,0.55)", borderColor: "transparent" },
  failed: { color: "rgba(248, 113, 113, 0.16)", textColor: "#fca5a5", borderColor: "transparent" },
};

function countOr0(n?: number | null) {
  return typeof n === "number" && Number.isFinite(n) ? n : 0;
}

function isListingPhase(task?: {
  phase?: string;
  status?: string;
  progressDone?: number;
  doneFiles?: number;
  itemCounts?: ItemCounts | null;
} | null) {
  if (!task) return false;
  if (task.phase === "listing") return true;
  if (task.phase === "downloading") return false;
  return (
    (task.status === "queued" || task.status === "running") &&
    countOr0(task.progressDone) === 0 &&
    countOr0(task.doneFiles) === 0 &&
    !(
      task.itemCounts &&
      (countOr0(task.itemCounts.done) ||
        countOr0(task.itemCounts.skipped) ||
        countOr0(task.itemCounts.failed) ||
        countOr0(task.itemCounts.downloading))
    )
  );
}

function taskDisplayStatus(task: { phase?: string; status?: string }) {
  const st = task.status || "";
  if (st === "done" || st === "failed" || st === "cancelled" || st === "paused") {
    return st;
  }
  if (isListingPhase(task)) return "listing";
  if (st === "running" || task.phase === "downloading") return "running";
  return st || "queued";
}

function taskStatusTag(task: { phase?: string; status?: string }) {
  const key = taskDisplayStatus(task);
  return {
    label: taskStatusLabel.value[key] || key,
    color: taskStatusTagColor[key] || taskStatusTagColor.queued,
  };
}

function pct(done?: number, total?: number, status?: string) {
  if (!total || total <= 0) return status === "done" ? 100 : 0;
  return Math.min(100, Math.round(((done || 0) / total) * 100));
}

/** 扫描阶段：待下载=已发现数；下载阶段：待下载=剩余未完成 */
function countTags(task?: {
  phase?: string;
  status?: string;
  progressTotal?: number;
  totalFiles?: number;
  progressDone?: number;
  doneFiles?: number;
  itemCounts?: ItemCounts | null;
} | null) {
  const listing = isListingPhase(task);
  const total = Math.max(countOr0(task?.progressTotal), countOr0(task?.totalFiles));
  const c = task?.itemCounts;
  if (listing) {
    return [
      { key: "pending", label: t("channelDetail.pending"), value: total > 0 ? String(total) : t("common.dash") },
      { key: "downloading", label: t("channelDetail.downloading"), value: t("common.dash") },
      { key: "done", label: t("channelDetail.done"), value: t("common.dash") },
      { key: "skipped", label: t("channelDetail.skipped"), value: t("common.dash") },
      { key: "failed", label: t("channelDetail.failed"), value: t("common.dash") },
    ];
  }
  const processed =
    countOr0(c?.done) + countOr0(c?.skipped) + countOr0(c?.failed) + countOr0(c?.downloading);
  const remaining = Math.max(0, total - processed);
  return [
    { key: "pending", label: t("channelDetail.pending"), value: String(remaining) },
    { key: "downloading", label: t("channelDetail.downloading"), value: String(countOr0(c?.downloading)) },
    { key: "done", label: t("channelDetail.done"), value: String(countOr0(c?.done)) },
    { key: "skipped", label: t("channelDetail.skipped"), value: String(countOr0(c?.skipped)) },
    { key: "failed", label: t("channelDetail.failed"), value: String(countOr0(c?.failed)) },
  ];
}

function hasFailedItems(task?: { status?: string; itemCounts?: ItemCounts | null } | null) {
  return countOr0(task?.itemCounts?.failed) > 0 || task?.status === "failed";
}

function batchRangeText(task?: { fromMessageId?: number | null; count?: number | null } | null) {
  if (!task) return "";
  const parts: string[] = [];
  if (task.fromMessageId != null && Number.isFinite(task.fromMessageId)) {
    parts.push(t("channelDetail.batchFrom", { id: task.fromMessageId }));
  }
  if (countOr0(task.count) > 0) parts.push(t("channelDetail.batchCount", { n: task.count }));
  return parts.join("｜");
}

function displayText(v: unknown, fallback = "-") {
  if (v == null || v === "") return fallback;
  return String(v);
}

function coverageText() {
  if (!info.value) return "";
  const c = info.value.scanCursor || 0;
  const l = info.value.lastMessageId || 0;
  if (l <= 0) return t("channelDetail.coverageUnknown", { cursor: c || 0 });
  if (info.value.caughtUp) return t("channelDetail.caughtUpLatest", { last: l });
  return t("channelDetail.scanned", { cursor: c, last: l });
}

function coveragePct() {
  if (!info.value) return 0;
  const l = info.value.lastMessageId || 0;
  const c = info.value.scanCursor || 0;
  if (l <= 0) return 0;
  return Math.min(100, Math.round((c / l) * 100));
}

async function load(silent = false) {
  if (!chatId.value) return;
  if (!silent) loading.value = true;
  try {
    info.value = await api<ChannelDownloadInfo>(`/api/channels/${chatId.value}/download`);
    if (info.value.defaultBatchSize) batchSize.value = info.value.defaultBatchSize;
  } catch (e) {
    if (!silent) message.error(e instanceof Error ? e.message : t("common.loadFailed"));
  } finally {
    if (!silent) loading.value = false;
  }
}

async function continueDownload() {
  if (!chatId.value) return;
  submitting.value = true;
  try {
    await api(`/api/channels/${chatId.value}/continue`, {
      method: "POST",
      body: JSON.stringify({ count: batchSize.value || 100, contentType: contentType.value }),
    });
    message.success(t("channelDetail.continueSuccess"));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.enqueueFailed"));
  } finally {
    submitting.value = false;
  }
}

function openCursorModal() {
  const cur = info.value?.scanCursor ?? 0;
  cursorInput.value = cur > 0 ? cur : 1;
  cursorModalOpen.value = true;
}

async function setCursor() {
  if (!chatId.value || cursorInput.value == null || cursorInput.value < 1) {
    message.warning(t("channelDetail.invalidMessageId"));
    return;
  }
  cursorSaving.value = true;
  try {
    await api(`/api/channels/${chatId.value}/scan-cursor`, {
      method: "POST",
      body: JSON.stringify({ mode: "set", messageId: cursorInput.value }),
    });
    message.success(t("channelDetail.cursorSet", { id: cursorInput.value }));
    cursorModalOpen.value = false;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.setFailed"));
  } finally {
    cursorSaving.value = false;
  }
}

async function alignCursor() {
  if (!chatId.value) return;
  cursorSaving.value = true;
  try {
    const res = await api<{ scanCursor: number }>(`/api/channels/${chatId.value}/scan-cursor`, {
      method: "POST",
      body: JSON.stringify({ mode: "align" }),
    });
    message.success(t("channelDetail.cursorAligned", { id: res.scanCursor }));
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("channelDetail.alignFailed"));
  } finally {
    cursorSaving.value = false;
  }
}

async function pause(id: number) {
  await api(`/api/tasks/${id}/pause`, { method: "POST", body: "{}" });
  await load();
}

async function resume(id: number) {
  await api(`/api/tasks/${id}/resume`, { method: "POST", body: "{}" });
  await load();
}

async function cancel(id: number) {
  await api(`/api/tasks/${id}/cancel`, { method: "POST", body: "{}" });
  await load();
}

async function retryFailed(id: number) {
  await api(`/api/tasks/${id}/retry-failed`, { method: "POST", body: "{}" });
  await load();
}

const isCustom = computed(() => Boolean(info.value?.isCustom || info.value?.custom || info.value?.kind === "custom"));

const completedBatchCount = computed(
  () =>
    (info.value?.recentBatches || []).filter(
      (task) => task.status === "done" || task.status === "cancelled" || task.status === "failed",
    ).length,
);

function confirmDelete() {
  dialog.warning({
    title: t("channelDetail.deleteTitle"),
    content: t("channelDetail.deleteContent"),
    positiveText: t("common.delete"),
    negativeText: t("common.cancel"),
    onPositiveClick: () => deleteCustom(),
  });
}

async function deleteCustom() {
  if (!chatId.value) return;
  deleting.value = true;
  try {
    await api(`/api/channels/${chatId.value}`, { method: "DELETE" });
    message.success(t("channelDetail.deleteSuccess"));
    await router.push({ name: "channels" });
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.deleteFailed"));
  } finally {
    deleting.value = false;
  }
}

async function clearCompletedBatches(e?: Event) {
  e?.stopPropagation();
  if (!chatId.value) return;
  clearing.value = true;
  try {
    const res = await api<{ cleared: number }>(`/api/channels/${chatId.value}/batches/completed`, {
      method: "DELETE",
    });
    message.success(
      res.cleared
        ? t("channelDetail.clearedBatches", { n: res.cleared })
        : t("channelDetail.nothingToClear"),
    );
    await load();
  } catch (err) {
    message.error(err instanceof Error ? err.message : t("common.clearFailed"));
  } finally {
    clearing.value = false;
  }
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as {
      type?: string;
      taskId?: number;
      kind?: string;
      phase?: string;
      done?: number;
      total?: number;
      status?: string;
      itemCounts?: ItemCounts;
    };
    const active = info.value?.activeTask;
    if (!active || !ev.taskId || ev.taskId !== active.id) {
      if (ev.type === "task_status" || ev.status === "done" || ev.status === "failed") void load(true);
      return;
    }
    if (ev.phase) {
      active.phase = ev.phase;
      if (ev.phase === "listing") {
        active.itemCounts = undefined;
        active.progressDone = 0;
        active.doneFiles = 0;
      }
      // 进入下载阶段时用扫描定稿总数覆盖（避免扫描窗口冲高后锁死）
      if (ev.phase === "downloading" && ev.total != null) {
        active.progressTotal = ev.total;
        active.totalFiles = ev.total;
      }
    }
    if (ev.done != null && active.phase !== "listing") {
      active.progressDone = ev.done;
      active.doneFiles = ev.done;
    }
    if (ev.total != null) {
      if (active.phase === "listing") {
        active.progressTotal = Math.max(countOr0(active.progressTotal), ev.total);
        active.totalFiles = active.progressTotal;
      } else if (ev.phase === "downloading") {
        // 已在上方覆盖
      } else {
        active.progressTotal = Math.max(countOr0(active.progressTotal), ev.total);
        active.totalFiles = Math.max(countOr0(active.totalFiles), ev.total);
      }
    }
    if (ev.status) active.status = ev.status;
    if (ev.itemCounts && active.phase !== "listing") {
      active.itemCounts = { ...ev.itemCounts };
    } else if (ev.itemCounts && active.phase === "listing") {
      // 扫描阶段只用 pending/total 刷新待下载数（不超过本批上限由后端保证）
      active.progressTotal = Math.max(countOr0(active.progressTotal), countOr0(ev.itemCounts.pending), countOr0(ev.total));
      active.totalFiles = active.progressTotal;
    }
    if (ev.status === "done" || ev.status === "failed" || ev.status === "cancelled") {
      void load(true);
    }
  } catch {
    /* ignore */
  }
}

useAppEvents(applyEvent);

watch(chatId, () => void load());

onMounted(async () => {
  await load();
  pollTimer = window.setInterval(() => void load(true), 8000);
});

onUnmounted(() => {
  if (pollTimer != null) window.clearInterval(pollTimer);
});
</script>

<template>
  <div class="page">
    <header class="head">
      <div class="head-top">
        <n-button quaternary size="small" @click="router.push({ name: 'channels' })">{{ t("channelDetail.back") }}</n-button>
        <n-button
          v-if="isCustom"
          size="small"
          type="error"
          secondary
          :loading="deleting"
          @click="confirmDelete"
        >
          <template #icon>
            <n-icon :component="TrashOutline" />
          </template>
          {{ t("common.delete") }}
        </n-button>
      </div>
      <div class="title-row">
        <h2>
          {{ info?.title || t("channelDetail.fallbackTitle", { chatId }) }}
          <span v-if="info?.username" class="title-at">@{{ info.username }}</span>
        </h2>
        <div class="title-actions">
          <n-button size="small" :loading="loading" @click="() => load()">
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            {{ t("common.refresh") }}
          </n-button>
          <template v-if="info">
            <n-button size="small" @click="openCursorModal">
              <template #icon>
                <n-icon :component="PulseOutline" />
              </template>
              {{ t("channelDetail.adjustCursor") }}
            </n-button>
            <n-button size="small" type="primary" ghost :loading="cursorSaving" @click="alignCursor">
              <template #icon>
                <n-icon :component="SyncOutline" />
              </template>
              {{ t("channelDetail.alignCursor") }}
            </n-button>
          </template>
        </div>
      </div>
    </header>

    <n-spin :show="loading && !info">
      <n-empty v-if="!info && !loading" :description="t('channelDetail.notFound')" />
      <template v-else-if="info">
        <section class="summary">
          <div class="summary-top">
            <div class="summary-meta">
              <span class="coverage">{{ coverageText() }}</span>
              <span class="stat">{{ t("channelDetail.downloaded") }} <b>{{ info.downloadedCount }}</b></span>
              <span v-if="info.failedCount" class="stat">{{ t("channelDetail.failedItems") }} <b class="warn">{{ info.failedCount }}</b></span>
            </div>
            <n-tag
              size="small"
              :bordered="false"
              :color="statusTagColor[info.status] || statusTagColor.idle"
            >
              {{ statusLabel[info.status] || info.status }}
            </n-tag>
          </div>
          <n-progress type="line" :percentage="coveragePct()" :show-indicator="true" />
          <div class="download-row">
            <div class="batch-params">
              <div class="field">
                <span class="field-label">{{ t("channelDetail.batchSize") }}</span>
                <n-input-number v-model:value="batchSize" :min="50" :max="5000" :step="50" size="small" />
              </div>
              <div class="field">
                <span class="field-label">{{ t("channelDetail.downloadContent") }}</span>
                <n-select
                  v-model:value="contentType"
                  class="content-type"
                  size="small"
                  :options="contentTypeOptions"
                />
              </div>
            </div>
            <n-button
              type="primary"
              :loading="submitting"
              :disabled="!!info.activeTask || info.caughtUp"
              @click="continueDownload"
            >
              <template #icon>
                <n-icon :component="DownloadOutline" />
              </template>
              {{
                info.caughtUp
                  ? t("channelDetail.caughtUpBtn")
                  : info.activeTask
                    ? t("status.running")
                    : t("channelDetail.continueDownload")
              }}
            </n-button>
          </div>
        </section>

        <section v-if="info.activeTask" class="active">
          <div class="sec-title">{{ t("channelDetail.activeTitle") }}</div>
          <div class="batch-card">
            <div class="batch-head">
              <div class="batch-head-main">
                <span class="batch-title">#{{ info.activeTask.id }} {{ displayText(info.activeTask.title) }}</span>
                <span v-if="batchRangeText(info.activeTask)" class="batch-range">
                  {{ batchRangeText(info.activeTask) }}
                </span>
              </div>
              <n-tag size="small" :bordered="false" :color="taskStatusTag(info.activeTask).color">
                {{ taskStatusTag(info.activeTask).label }}
              </n-tag>
            </div>
            <n-progress
              type="line"
              :percentage="
                pct(
                  info.activeTask.progressDone || info.activeTask.doneFiles,
                  info.activeTask.progressTotal || info.activeTask.totalFiles,
                  info.activeTask.status,
                )
              "
              :processing="info.activeTask.status === 'running' || info.activeTask.status === 'queued'"
              indicator-placement="inside"
            />
            <div class="meta counts">
              <div class="count-texts">
                <span
                  v-for="tag in countTags(info.activeTask)"
                  :key="tag.key"
                  class="count-text"
                  :style="{ color: countTagColor[tag.key]?.textColor }"
                >
                  {{ tag.label }} {{ tag.value }}
                </span>
              </div>
              <div class="batch-actions">
                <n-button
                  v-if="info.activeTask.status === 'running' || info.activeTask.status === 'queued'"
                  size="small"
                  ghost
                  type="warning"
                  @click="pause(info.activeTask.id)"
                >
                  <template #icon>
                    <n-icon :component="PauseOutline" />
                  </template>
                  {{ t("channelDetail.pause") }}
                </n-button>
                <n-button
                  v-if="info.activeTask.status === 'paused'"
                  size="small"
                  type="primary"
                  ghost
                  @click="resume(info.activeTask.id)"
                >
                  <template #icon>
                    <n-icon :component="PlayOutline" />
                  </template>
                  {{ t("channelDetail.resume") }}
                </n-button>
                <n-button
                  v-if="info.activeTask.status !== 'cancelled' && info.activeTask.status !== 'done'"
                  size="small"
                  ghost
                  type="error"
                  @click="cancel(info.activeTask.id)"
                >
                  <template #icon>
                    <n-icon :component="CloseOutline" />
                  </template>
                  {{ t("common.cancel") }}
                </n-button>
                <n-button
                  v-if="hasFailedItems(info.activeTask)"
                  size="small"
                  ghost
                  type="warning"
                  @click="retryFailed(info.activeTask.id)"
                >
                  <template #icon>
                    <n-icon :component="ReloadOutline" />
                  </template>
                  {{ t("channelDetail.retry") }}
                </n-button>
              </div>
            </div>
            <div v-if="info.activeTask.error" class="err">{{ info.activeTask.error }}</div>
          </div>
        </section>

        <section class="history">
          <n-collapse>
            <n-collapse-item name="history">
              <template #header>{{ t("channelDetail.historyTitle") }}</template>
              <template #header-extra>
                <n-button
                  size="small"
                  type="error"
                  secondary
                  :loading="clearing"
                  :disabled="completedBatchCount <= 0"
                  @click="clearCompletedBatches"
                >
                  {{ t("channelDetail.clearCompleted") }}
                </n-button>
              </template>
              <n-empty
                v-if="!(info.recentBatches && info.recentBatches.length)"
                :description="t('channelDetail.historyEmpty')"
                size="small"
              />
              <div v-for="task in info.recentBatches" :key="task.id" class="batch-card dim">
                <div class="batch-head">
                  <div class="batch-head-main">
                    <span class="batch-title">#{{ task.id }} {{ displayText(task.title) }}</span>
                    <span v-if="batchRangeText(task)" class="batch-range">
                      {{ batchRangeText(task) }}
                    </span>
                  </div>
                  <n-tag size="small" :bordered="false" :color="taskStatusTag(task).color">
                    {{ taskStatusTag(task).label }}
                  </n-tag>
                </div>
                <div class="meta counts">
                  <div class="count-texts">
                    <span
                      v-for="tag in countTags(task)"
                      :key="tag.key"
                      class="count-text"
                      :style="{ color: countTagColor[tag.key]?.textColor }"
                    >
                      {{ tag.label }} {{ tag.value }}
                    </span>
                  </div>
                  <div v-if="hasFailedItems(task)" class="batch-actions">
                    <n-button size="tiny" ghost type="warning" @click="retryFailed(task.id)">
                      <template #icon>
                        <n-icon :component="ReloadOutline" />
                      </template>
                      {{ t("channelDetail.retry") }}
                    </n-button>
                  </div>
                </div>
              </div>
            </n-collapse-item>
          </n-collapse>
        </section>
      </template>
    </n-spin>

    <n-modal
      v-model:show="cursorModalOpen"
      preset="card"
      :title="t('channelDetail.cursorTitle')"
      style="width: 420px; max-width: 92vw"
      :mask-closable="!cursorSaving"
    >
      <div class="cursor-modal">
        <p class="cursor-meta">
          {{
            t("channelDetail.cursorMeta", {
              scan: info?.scanCursor ?? 0,
              local: info?.localMaxMessageId ?? 0,
              last: info?.lastMessageId || t("common.dash"),
            })
          }}
        </p>
        <div class="cursor-row">
          <n-input-number
            v-model:value="cursorInput"
            :min="1"
            :step="1"
            :placeholder="t('channelDetail.messageId')"
            class="cursor-input"
          />
          <n-button type="primary" :loading="cursorSaving" @click="setCursor">{{ t("channelDetail.setCursor") }}</n-button>
        </div>
        <p class="cursor-hint">{{ t("channelDetail.cursorHint") }}</p>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.head {
  margin-bottom: 16px;
}
.head h2 {
  margin: 0;
  font-size: 22px;
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}
.head-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px 12px;
  margin-top: 8px;
}
.title-at {
  font-size: 14px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.45);
}
.title-actions {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-shrink: 0;
  margin-left: auto;
}
.summary {
  padding: 16px;
  border-radius: 12px;
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 16px;
}
.summary-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 17px;
}
.summary-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 12px 16px;
  min-width: 0;
}
.coverage {
  font-size: 16px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.92);
}
.stat {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
}
.stat b {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 600;
}
.warn {
  color: #fca5a5 !important;
}
.download-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
}
.batch-params {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px 16px;
}
.field {
  display: inline-flex;
  flex-direction: column;
  gap: 6px;
}
.field-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
}
.field :deep(.n-input-number) {
  width: 120px;
}
.content-type {
  width: 120px;
}
.sec-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 10px;
  color: rgba(255, 255, 255, 0.85);
}
.active {
  margin-bottom: 16px;
}
.batch-card {
  padding: 12px 14px;
  border-radius: 10px;
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 8px;
}
.batch-card.dim {
  opacity: 0.85;
}
.batch-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  font-weight: 600;
  font-size: 13px;
}
.batch-head-main {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px 12px;
  min-width: 0;
  flex: 1;
}
.batch-title {
  min-width: 0;
}
.meta {
  margin-top: 8px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
.counts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
}
.count-texts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  min-width: 0;
}
.count-text {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.batch-range {
  font-size: 12px;
  font-weight: 400;
  color: rgba(255, 255, 255, 0.45);
  white-space: nowrap;
}
.batch-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
  align-items: center;
}
.err {
  color: #f9a8d4;
  font-size: 12px;
  margin-top: 6px;
}
.history {
  margin-top: 8px;
}
.cursor-modal {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cursor-meta {
  margin: 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
  line-height: 1.5;
}
.cursor-meta b {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 600;
}
.cursor-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.cursor-input {
  flex: 1 1 auto;
  min-width: 0;
}
.cursor-hint {
  margin: 0;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.5;
}
</style>
