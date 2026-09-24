<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NButton,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NInputNumber,
  NModal,
  NProgress,
  NSpin,
  NTag,
  useDialog,
  useMessage,
} from "naive-ui";
import { api } from "../../api/http";
import type { ChannelDownloadInfo, ItemCounts } from "../../api/types";
import { useAppEvents } from "../../composables/useAppEvents";

defineOptions({ name: "ChannelDetailView" });

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
const info = ref<ChannelDownloadInfo | null>(null);
const cursorModalOpen = ref(false);
const cursorInput = ref<number | null>(null);

let pollTimer: number | null = null;

const chatId = computed(() => {
  const n = Number(route.params.chatId);
  return Number.isFinite(n) ? n : 0;
});

const statusLabel: Record<string, string> = {
  idle: "可继续",
  running: "下载中",
  queued: "排队中",
  paused: "已暂停",
  caught_up: "已追平",
  has_failed: "有失败",
};

const taskStatusLabel: Record<string, string> = {
  running: "下载中",
  listing: "处理中",
  queued: "排队",
  paused: "暂停",
  done: "完成",
  failed: "失败",
  cancelled: "取消",
};

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

function isListingPhase(t?: {
  phase?: string;
  status?: string;
  progressDone?: number;
  doneFiles?: number;
  itemCounts?: ItemCounts | null;
} | null) {
  if (!t) return false;
  if (t.phase === "listing") return true;
  if (t.phase === "downloading") return false;
  return (
    (t.status === "queued" || t.status === "running") &&
    countOr0(t.progressDone) === 0 &&
    countOr0(t.doneFiles) === 0 &&
    !(
      t.itemCounts &&
      (countOr0(t.itemCounts.done) ||
        countOr0(t.itemCounts.skipped) ||
        countOr0(t.itemCounts.failed) ||
        countOr0(t.itemCounts.downloading))
    )
  );
}

function taskDisplayStatus(t: { phase?: string; status?: string }) {
  const st = t.status || "";
  if (st === "done" || st === "failed" || st === "cancelled" || st === "paused") {
    return st;
  }
  if (isListingPhase(t)) return "listing";
  if (st === "running" || t.phase === "downloading") return "running";
  return st || "queued";
}

function taskStatusTag(t: { phase?: string; status?: string }) {
  const key = taskDisplayStatus(t);
  return {
    label: taskStatusLabel[key] || key,
    color: taskStatusTagColor[key] || taskStatusTagColor.queued,
  };
}

function pct(done?: number, total?: number, status?: string) {
  if (!total || total <= 0) return status === "done" ? 100 : 0;
  return Math.min(100, Math.round(((done || 0) / total) * 100));
}

/** 扫描阶段：待下载=已发现数；下载阶段：待下载=剩余未完成 */
function countTags(t?: {
  phase?: string;
  status?: string;
  progressTotal?: number;
  totalFiles?: number;
  progressDone?: number;
  doneFiles?: number;
  itemCounts?: ItemCounts | null;
} | null) {
  const listing = isListingPhase(t);
  const total = Math.max(countOr0(t?.progressTotal), countOr0(t?.totalFiles));
  const c = t?.itemCounts;
  if (listing) {
    return [
      { key: "pending", label: "待下", value: total > 0 ? String(total) : "—" },
      { key: "downloading", label: "下载", value: "—" },
      { key: "done", label: "完成", value: "—" },
      { key: "skipped", label: "跳过", value: "—" },
      { key: "failed", label: "失败", value: "—" },
    ];
  }
  const processed =
    countOr0(c?.done) + countOr0(c?.skipped) + countOr0(c?.failed) + countOr0(c?.downloading);
  const remaining = Math.max(0, total - processed);
  return [
    { key: "pending", label: "待下", value: String(remaining) },
    { key: "downloading", label: "下载", value: String(countOr0(c?.downloading)) },
    { key: "done", label: "完成", value: String(countOr0(c?.done)) },
    { key: "skipped", label: "跳过", value: String(countOr0(c?.skipped)) },
    { key: "failed", label: "失败", value: String(countOr0(c?.failed)) },
  ];
}

function hasFailedItems(t?: { status?: string; itemCounts?: ItemCounts | null } | null) {
  return countOr0(t?.itemCounts?.failed) > 0 || t?.status === "failed";
}

function batchRangeText(t?: { fromMessageId?: number | null; count?: number | null } | null) {
  if (!t) return "";
  const parts: string[] = [];
  if (t.fromMessageId != null && Number.isFinite(t.fromMessageId)) {
    parts.push(`起始：${t.fromMessageId}`);
  }
  if (countOr0(t.count) > 0) parts.push(`数量：${t.count}`);
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
  if (l <= 0) return `水位 #${c || 0} · 最新未知（请先同步对话）`;
  if (info.value.caughtUp) return `已追平最新 #${l}`;
  return `已扫描到 #${c} / 最新 #${l}`;
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
    if (!silent) message.error(e instanceof Error ? e.message : "加载失败");
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
      body: JSON.stringify({ count: batchSize.value || 100 }),
    });
    message.success("已开始继续下载");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "入队失败");
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
    message.warning("请输入有效的消息 ID（最小为 1）");
    return;
  }
  cursorSaving.value = true;
  try {
    await api(`/api/channels/${chatId.value}/scan-cursor`, {
      method: "POST",
      body: JSON.stringify({ mode: "set", messageId: cursorInput.value }),
    });
    message.success(`水位已设为 #${cursorInput.value}`);
    cursorModalOpen.value = false;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "设置失败");
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
    message.success(`已对齐水位 #${res.scanCursor}`);
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "对齐失败");
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
      (t) => t.status === "done" || t.status === "cancelled" || t.status === "failed",
    ).length,
);

function confirmDelete() {
  dialog.warning({
    title: "删除自定义频道",
    content: "确定删除该自定义频道？下载记录不会一并清除。",
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: () => deleteCustom(),
  });
}

async function deleteCustom() {
  if (!chatId.value) return;
  deleting.value = true;
  try {
    await api(`/api/channels/${chatId.value}`, { method: "DELETE" });
    message.success("已删除自定义频道");
    await router.push({ name: "channels" });
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
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
    message.success(res.cleared ? `已清除 ${res.cleared} 条批次` : "没有可清除的已完成批次");
    await load();
  } catch (err) {
    message.error(err instanceof Error ? err.message : "清除失败");
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
      <div>
        <n-button quaternary size="small" @click="router.push({ name: 'channels' })">← 频道列表</n-button>
        <h2>{{ info?.title || `频道 ${chatId}` }}</h2>
        <p v-if="info?.username" class="sub">@{{ info.username }}</p>
      </div>
      <div class="head-actions">
        <n-button
          v-if="isCustom"
          secondary
          type="error"
          :loading="deleting"
          @click="confirmDelete"
        >
          删除
        </n-button>
        <n-button secondary :loading="loading" @click="() => load()">刷新</n-button>
      </div>
    </header>

    <n-spin :show="loading && !info">
      <n-empty v-if="!info && !loading" description="频道不存在" />
      <template v-else-if="info">
        <section class="summary">
          <div class="summary-top">
            <div>
              <div class="label">覆盖进度</div>
              <div class="coverage">{{ coverageText() }}</div>
            </div>
            <n-tag size="small" :bordered="false" type="info">
              {{ statusLabel[info.status] || info.status }}
            </n-tag>
          </div>
          <n-progress type="line" :percentage="coveragePct()" :show-indicator="true" />
          <div class="stats">
            <span>已下载媒体 <b>{{ info.downloadedCount }}</b></span>
            <span v-if="info.failedCount">失败项 <b class="warn">{{ info.failedCount }}</b></span>
          </div>
          <div class="actions">
            <div class="batch-setting">
              <span>每批条数</span>
              <n-input-number v-model:value="batchSize" :min="50" :max="5000" :step="50" size="small" />
            </div>
            <n-button secondary @click="openCursorModal">调整水位</n-button>
            <n-button
              type="primary"
              secondary
              :loading="cursorSaving"
              @click="alignCursor"
            >
              对齐水位
            </n-button>
            <n-button
              type="primary"
              :loading="submitting"
              :disabled="!!info.activeTask || info.caughtUp"
              @click="continueDownload"
            >
              {{ info.caughtUp ? "已追平最新" : info.activeTask ? "下载进行中" : "继续下载" }}
            </n-button>
          </div>
        </section>

        <section v-if="info.activeTask" class="active">
          <div class="sec-title">进行中</div>
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
                  secondary
                  type="warning"
                  @click="pause(info.activeTask.id)"
                >
                  暂停
                </n-button>
                <n-button
                  v-if="info.activeTask.status === 'paused'"
                  size="small"
                  type="primary"
                  @click="resume(info.activeTask.id)"
                >
                  继续
                </n-button>
                <n-button
                  v-if="info.activeTask.status !== 'cancelled' && info.activeTask.status !== 'done'"
                  size="small"
                  secondary
                  type="error"
                  @click="cancel(info.activeTask.id)"
                >
                  取消
                </n-button>
                <n-button
                  v-if="hasFailedItems(info.activeTask)"
                  size="small"
                  secondary
                  type="warning"
                  @click="retryFailed(info.activeTask.id)"
                >
                  重试失败
                </n-button>
              </div>
            </div>
            <div v-if="info.activeTask.error" class="err">{{ info.activeTask.error }}</div>
          </div>
        </section>

        <section class="history">
          <n-collapse>
            <n-collapse-item name="history">
              <template #header>历史批次</template>
              <template #header-extra>
                <n-button
                  size="small"
                  type="primary"
                  secondary
                  :loading="clearing"
                  :disabled="completedBatchCount <= 0"
                  @click="clearCompletedBatches"
                >
                  清除已完成
                </n-button>
              </template>
              <n-empty
                v-if="!(info.recentBatches && info.recentBatches.length)"
                description="暂无历史批次"
                size="small"
              />
              <div v-for="t in info.recentBatches" :key="t.id" class="batch-card dim">
                <div class="batch-head">
                  <div class="batch-head-main">
                    <span class="batch-title">#{{ t.id }} {{ displayText(t.title) }}</span>
                    <span v-if="batchRangeText(t)" class="batch-range">
                      {{ batchRangeText(t) }}
                    </span>
                  </div>
                  <n-tag size="small" :bordered="false" :color="taskStatusTag(t).color">
                    {{ taskStatusTag(t).label }}
                  </n-tag>
                </div>
                <div class="meta counts">
                  <div class="count-texts">
                    <span
                      v-for="tag in countTags(t)"
                      :key="tag.key"
                      class="count-text"
                      :style="{ color: countTagColor[tag.key]?.textColor }"
                    >
                      {{ tag.label }} {{ tag.value }}
                    </span>
                  </div>
                  <div v-if="hasFailedItems(t)" class="batch-actions">
                    <n-button size="tiny" secondary type="warning" @click="retryFailed(t.id)">重试失败</n-button>
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
      title="调整水位"
      style="width: 420px; max-width: 92vw"
      :mask-closable="!cursorSaving"
    >
      <div class="cursor-modal">
        <p class="cursor-meta">
          当前水位 <b>#{{ info?.scanCursor ?? 0 }}</b>
          · 本地最大 <b>#{{ info?.localMaxMessageId ?? 0 }}</b>
          · 对话最新 <b>#{{ info?.lastMessageId || "—" }}</b>
        </p>
        <div class="cursor-row">
          <n-input-number
            v-model:value="cursorInput"
            :min="1"
            :step="1"
            placeholder="消息 ID"
            class="cursor-input"
          />
          <n-button type="primary" :loading="cursorSaving" @click="setCursor">设置水位</n-button>
        </div>
        <p class="cursor-hint">下次「继续下载」从该 id 之后扫描；已存在文件仍会去重跳过。</p>
      </div>
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
  margin: 8px 0 0;
  font-size: 22px;
}
.head-actions {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.sub {
  margin: 4px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
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
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}
.label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  margin-bottom: 4px;
}
.coverage {
  font-size: 16px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.92);
}
.stats {
  display: flex;
  gap: 16px;
  margin: 12px 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
}
.stats b {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 600;
}
.warn {
  color: #fca5a5 !important;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.batch-setting {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.65);
}
.batch-setting :deep(.n-input-number) {
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
