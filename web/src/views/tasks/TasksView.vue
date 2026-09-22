<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NButton, useMessage } from "naive-ui";
import { api, openEventSource } from "../../api/http";
import { useMobile } from "../../composables/useMobile";
import type { ItemCounts } from "../../api/types";
import MessageDownloadPanel from "./components/MessageDownloadPanel.vue";
import SavedSyncPanel from "./components/SavedSyncPanel.vue";

defineOptions({ name: "TasksView" });

const route = useRoute();
const router = useRouter();
const message = useMessage();
const isMobile = useMobile();
const tab = ref<"message" | "saved">("message");
const busy = ref(false);

const messagePanel = ref<{ reload: () => Promise<void> } | null>(null);
const savedPanel = ref<{
  reload: () => Promise<void>;
  applyProgress: (ev: {
    taskId?: number;
    done?: number;
    total?: number;
    status?: string;
    itemCounts?: ItemCounts;
  }) => boolean;
} | null>(null);

const tabOptions = [
  { name: "message" as const, label: "消息下载" },
  { name: "saved" as const, label: "收藏同步" },
];

let es: EventSource | null = null;
let pollTimer: number | null = null;

async function reloadTab() {
  if (tab.value === "saved") {
    await savedPanel.value?.reload();
  } else {
    await messagePanel.value?.reload();
  }
}

async function pauseAll() {
  busy.value = true;
  try {
    const data = await api<{ paused: number }>("/api/tasks/pause-all", { method: "POST", body: "{}" });
    message.success(`已暂停 ${data.paused} 个任务`);
    await reloadTab();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "暂停失败");
  } finally {
    busy.value = false;
  }
}

async function startAll() {
  busy.value = true;
  try {
    const data = await api<{ started: number }>("/api/tasks/start-all", { method: "POST", body: "{}" });
    message.success(`已恢复 ${data.started} 个任务`);
    await reloadTab();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "启动失败");
  } finally {
    busy.value = false;
  }
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as {
      type?: string;
      kind?: string;
      taskId?: number;
      done?: number;
      total?: number;
      status?: string;
      itemCounts?: ItemCounts;
    };
    if (ev.type === "task_item_progress") {
      void reloadTab();
      return;
    }
    if (savedPanel.value?.applyProgress(ev)) return;
    void reloadTab();
  } catch {
    /* ignore */
  }
}

function applyRoutePrefill() {
  const q = route.query;
  if (q.tab === "channel") {
    void router.replace({ name: "channels" });
    return;
  }
  if (q.tab === "saved" || q.tab === "message") {
    tab.value = q.tab;
  }
  if (q.chatId != null && String(q.chatId) !== "") {
    const id = Number(q.chatId);
    if (Number.isFinite(id)) {
      void router.replace({ name: "channel-detail", params: { chatId: String(id) } });
    }
  }
}

watch(
  () => [route.query.tab, route.query.chatId] as const,
  () => {
    applyRoutePrefill();
  },
);

onMounted(() => {
  applyRoutePrefill();
  void (async () => {
    try {
      es = await openEventSource("/api/events");
      es.onmessage = (e) => applyEvent(e.data);
    } catch {
      /* poll */
    }
  })();
  pollTimer = window.setInterval(() => void reloadTab(), 8000);
});

onUnmounted(() => {
  es?.close();
  if (pollTimer != null) window.clearInterval(pollTimer);
});
</script>

<template>
  <div class="page list-page" :class="{ pinned: !isMobile }">
    <div class="toolbar">
      <h2>任务</h2>
      <div class="tab-bar">
        <button
          v-for="opt in tabOptions"
          :key="opt.name"
          type="button"
          class="tab-btn"
          :class="{ active: tab === opt.name }"
          @click="tab = opt.name"
        >
          {{ opt.label }}
        </button>
      </div>
      <n-button quaternary :disabled="busy" @click="pauseAll">全部暂停</n-button>
      <n-button quaternary :disabled="busy" @click="startAll">全部开始</n-button>
      <n-button quaternary :disabled="busy" @click="reloadTab">刷新</n-button>
    </div>

    <MessageDownloadPanel v-show="tab === 'message'" ref="messagePanel" :active="tab === 'message'" />
    <SavedSyncPanel v-show="tab === 'saved'" ref="savedPanel" :active="tab === 'saved'" />
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
  gap: 16px;
}
.tab-bar {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
}
.tab-btn {
  appearance: none;
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
  font-size: 14px;
  font-weight: 500;
  padding: 6px 12px;
  border-radius: 8px;
  cursor: pointer;
  white-space: nowrap;
}
.tab-btn:hover {
  color: rgba(255, 255, 255, 0.85);
  background: rgba(255, 255, 255, 0.04);
}
.tab-btn.active {
  color: #f9a8d4;
  background: rgba(244, 114, 182, 0.12);
}
@media (max-width: 1000px) {
  .toolbar {
    flex-wrap: wrap;
  }
  .tab-bar {
    order: 3;
    width: 100%;
  }
}
</style>
