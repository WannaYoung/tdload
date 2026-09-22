<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NButton,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NInputNumber,
  NProgress,
  NSpin,
  NTag,
  useMessage,
} from "naive-ui";
import { api, openEventSource } from "../../api/http";
import type { ChannelDownloadInfo, ItemCounts } from "../../api/types";

defineOptions({ name: "ChannelDetailView" });

const route = useRoute();
const router = useRouter();
const message = useMessage();

const loading = ref(false);
const submitting = ref(false);
const batchSize = ref(500);
const info = ref<ChannelDownloadInfo | null>(null);

let es: EventSource | null = null;
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
  queued: "排队",
  paused: "暂停",
  done: "完成",
  failed: "失败",
  cancelled: "取消",
};

function pct(done?: number, total?: number, status?: string) {
  if (!total || total <= 0) return status === "done" ? 100 : 0;
  return Math.min(100, Math.round(((done || 0) / total) * 100));
}

function countsLine(c?: ItemCounts) {
  if (!c) return "";
  return `待${c.pending} · 下${c.downloading} · 完${c.done} · 跳${c.skipped} · 败${c.failed}`;
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

async function load() {
  if (!chatId.value) return;
  loading.value = true;
  try {
    info.value = await api<ChannelDownloadInfo>(`/api/channels/${chatId.value}/download`);
    if (info.value.defaultBatchSize) batchSize.value = info.value.defaultBatchSize;
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function continueDownload() {
  if (!chatId.value) return;
  submitting.value = true;
  try {
    await api(`/api/channels/${chatId.value}/continue`, {
      method: "POST",
      body: JSON.stringify({ count: batchSize.value || 500 }),
    });
    message.success("已开始继续下载");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "入队失败");
  } finally {
    submitting.value = false;
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

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as { type?: string; taskId?: number; kind?: string; done?: number; total?: number; status?: string; itemCounts?: ItemCounts };
    const active = info.value?.activeTask;
    if (!active || !ev.taskId || ev.taskId !== active.id) {
      if (ev.type === "task_status" || ev.status === "done" || ev.status === "failed") void load();
      return;
    }
    active.progressDone = ev.done ?? active.progressDone;
    active.progressTotal = ev.total ?? active.progressTotal;
    if (ev.status) active.status = ev.status;
    if (ev.itemCounts) active.itemCounts = ev.itemCounts;
    if (ev.status === "done" || ev.status === "failed" || ev.status === "cancelled") {
      void load();
    }
  } catch {
    /* ignore */
  }
}

watch(chatId, () => void load());

onMounted(async () => {
  await load();
  try {
    es = await openEventSource("/api/events");
    es.onmessage = (e) => applyEvent(e.data);
  } catch {
    /* poll */
  }
  pollTimer = window.setInterval(() => void load(), 8000);
});

onUnmounted(() => {
  es?.close();
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
      <n-button secondary :loading="loading" @click="load">刷新</n-button>
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
            <n-button
              type="primary"
              size="large"
              :loading="submitting"
              :disabled="!!info.activeTask || info.caughtUp"
              @click="continueDownload"
            >
              {{ info.caughtUp ? "已追平最新" : info.activeTask ? "下载进行中" : "继续下载" }}
            </n-button>
          </div>
          <p class="hint">从水位之后向前扫描，去重后下载；同频道同时只跑一批。</p>
        </section>

        <section v-if="info.activeTask" class="active">
          <div class="sec-title">进行中</div>
          <div class="batch-card">
            <div class="batch-head">
              <span>#{{ info.activeTask.id }} {{ info.activeTask.title }}</span>
              <n-tag size="small" :bordered="false">
                {{ taskStatusLabel[info.activeTask.status] || info.activeTask.status }}
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
              :processing="info.activeTask.status === 'running'"
              indicator-placement="inside"
            />
            <div class="meta">{{ countsLine(info.activeTask.itemCounts) }}</div>
            <div v-if="info.activeTask.error" class="err">{{ info.activeTask.error }}</div>
            <div class="batch-actions">
              <n-button
                v-if="info.activeTask.status === 'running' || info.activeTask.status === 'queued'"
                size="small"
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
                @click="cancel(info.activeTask.id)"
              >
                取消
              </n-button>
              <n-button size="small" @click="retryFailed(info.activeTask.id)">重试失败</n-button>
            </div>
          </div>
        </section>

        <section class="history">
          <n-collapse>
            <n-collapse-item title="历史批次" name="history">
              <n-empty
                v-if="!(info.recentBatches && info.recentBatches.length)"
                description="暂无历史批次"
                size="small"
              />
              <div v-for="t in info.recentBatches" :key="t.id" class="batch-card dim">
                <div class="batch-head">
                  <span>#{{ t.id }} {{ t.title }}</span>
                  <n-tag size="small" :bordered="false">
                    {{ taskStatusLabel[t.status] || t.status }}
                  </n-tag>
                </div>
                <div class="meta">
                  {{ countsLine(t.itemCounts) || `${t.progressDone || t.doneFiles || 0}/${t.progressTotal || t.totalFiles || 0}` }}
                </div>
                <div v-if="t.status === 'failed' || (t.itemCounts && t.itemCounts.failed > 0)" class="batch-actions">
                  <n-button size="tiny" @click="retryFailed(t.id)">重试失败</n-button>
                </div>
              </div>
            </n-collapse-item>
          </n-collapse>
        </section>
      </template>
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
  margin: 8px 0 0;
  font-size: 22px;
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
.hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
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
.meta {
  margin-top: 8px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
.batch-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}
.err {
  color: #f9a8d4;
  font-size: 12px;
  margin-top: 6px;
}
.history {
  margin-top: 8px;
}
</style>
