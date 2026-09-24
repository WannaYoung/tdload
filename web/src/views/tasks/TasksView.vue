<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NIcon } from "naive-ui";
import { RefreshOutline } from "@vicons/ionicons5";
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

const { t } = useI18n();
const isMobile = useMobile();

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

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as ProgressEv;
    const terminal =
      ev.status === "done" || ev.status === "failed" || ev.status === "cancelled";

    if (ev.kind === "saved" || ev.kind === "channel") return;

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
      <h2>{{ t("tasks.title") }}</h2>
      <div class="toolbar-actions">
        <n-button @click="reload(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          {{ t("common.refresh") }}
        </n-button>
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
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  flex-shrink: 0;
  width: auto;
}
</style>
