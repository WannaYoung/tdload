<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { NButton, useMessage } from "naive-ui";
import { api } from "../../api/http";
import { useMobile } from "../../composables/useMobile";
import { useAppEvents } from "../../composables/useAppEvents";
import type { ItemCounts } from "../../api/types";
import MessageDownloadPanel from "./components/MessageDownloadPanel.vue";

defineOptions({ name: "TasksView" });

type ProgressEv = {
  type?: string;
  kind?: string;
  taskId?: number;
  chatId?: number;
  messageId?: number;
  done?: number;
  total?: number;
  status?: string;
  title?: string;
  itemCounts?: ItemCounts;
};

const message = useMessage();
const isMobile = useMobile();
const busy = ref(false);

const messagePanel = ref<{
  reload: (silent?: boolean) => Promise<void>;
  applyItemProgress: (ev: ProgressEv) => boolean;
} | null>(null);

let pollTimer: number | null = null;
let messageReloadTimer: number | null = null;

async function reload(silent = false) {
  await messagePanel.value?.reload(silent);
}

function scheduleMessageSilentReload() {
  if (messageReloadTimer != null) window.clearTimeout(messageReloadTimer);
  messageReloadTimer = window.setTimeout(() => {
    messageReloadTimer = null;
    void messagePanel.value?.reload(true);
  }, 400);
}

async function pauseAll() {
  busy.value = true;
  try {
    const data = await api<{ paused: number }>("/api/tasks/pause-all", { method: "POST", body: "{}" });
    message.success(`已暂停 ${data.paused} 个任务`);
    await reload();
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
    await reload();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "启动失败");
  } finally {
    busy.value = false;
  }
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as ProgressEv;
    if (ev.kind === "channel" || ev.kind === "saved") return;

    const terminal =
      ev.status === "done" || ev.status === "failed" || ev.status === "cancelled";

    if (ev.type === "task_item_progress") {
      if (messagePanel.value?.applyItemProgress(ev)) {
        if (terminal) scheduleMessageSilentReload();
        return;
      }
      scheduleMessageSilentReload();
      return;
    }

    if (ev.kind === "message" || !ev.kind) {
      if (terminal || ev.type === "task_progress") {
        scheduleMessageSilentReload();
      }
    }
  } catch {
    /* ignore */
  }
}

useAppEvents(applyEvent);

onMounted(() => {
  pollTimer = window.setInterval(() => void reload(true), 12000);
});

onUnmounted(() => {
  if (pollTimer != null) window.clearInterval(pollTimer);
  if (messageReloadTimer != null) window.clearTimeout(messageReloadTimer);
});
</script>

<template>
  <div class="page list-page" :class="{ pinned: !isMobile }">
    <div class="toolbar">
      <h2>任务</h2>
      <div class="toolbar-actions">
        <n-button quaternary :disabled="busy" @click="pauseAll">全部暂停</n-button>
        <n-button quaternary :disabled="busy" @click="startAll">全部开始</n-button>
        <n-button quaternary :disabled="busy" @click="reload(false)">刷新</n-button>
      </div>
    </div>
    <MessageDownloadPanel ref="messagePanel" :active="true" />
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
.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
</style>
