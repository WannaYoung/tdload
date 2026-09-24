<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { NButton, NIcon } from "naive-ui";
import { RefreshOutline, SyncOutline, TrashOutline } from "@vicons/ionicons5";
import { useMobile } from "../../composables/useMobile";
import { useAppEvents } from "../../composables/useAppEvents";
import type { ItemCounts } from "../../api/types";
import SavedSyncPanel from "../tasks/components/SavedSyncPanel.vue";

defineOptions({ name: "SavedView" });

type ProgressEv = {
  type?: string;
  kind?: string;
  taskId?: number;
  phase?: string;
  done?: number;
  total?: number;
  status?: string;
  itemCounts?: ItemCounts;
};

type SavedPanelExpose = {
  reload: (silent?: boolean) => Promise<void>;
  applyProgress: (ev: ProgressEv) => boolean;
  clearCompleted: () => Promise<void>;
  createTask: () => Promise<void>;
  clearing: boolean;
  submitting: boolean;
  hasActiveSync: boolean;
  completedCount: number;
};

const isMobile = useMobile();
const savedPanel = ref<SavedPanelExpose | null>(null);

let pollTimer: number | null = null;

async function reload(silent = false) {
  await savedPanel.value?.reload(silent);
}

function applyEvent(raw: string) {
  try {
    const ev = JSON.parse(raw) as ProgressEv;
    if (ev.kind && ev.kind !== "saved") return;
    const terminal =
      ev.status === "done" || ev.status === "failed" || ev.status === "cancelled";
    if (savedPanel.value?.applyProgress(ev)) {
      if (terminal && ev.type !== "task_item_progress") void savedPanel.value.reload(true);
      return;
    }
    void savedPanel.value?.reload(true);
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
});
</script>

<template>
  <div class="page list-page" :class="{ pinned: !isMobile }">
    <div class="toolbar">
      <h2>收藏同步</h2>
      <div class="toolbar-actions">
        <n-button @click="reload(false)">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
        <n-button
          type="error"
          ghost
          :loading="!!savedPanel?.clearing"
          :disabled="(savedPanel?.completedCount ?? 0) <= 0"
          @click="savedPanel?.clearCompleted()"
        >
          <template #icon>
            <n-icon :component="TrashOutline" />
          </template>
          清除
        </n-button>
        <n-button
          type="primary"
          :loading="!!savedPanel?.submitting"
          :disabled="!!savedPanel?.hasActiveSync"
          :title="savedPanel?.hasActiveSync ? '已有进行中的同步' : undefined"
          @click="savedPanel?.createTask()"
        >
          <template #icon>
            <n-icon :component="SyncOutline" />
          </template>
          {{ savedPanel?.hasActiveSync ? "同步中" : "同步" }}
        </n-button>
      </div>
    </div>
    <SavedSyncPanel ref="savedPanel" :active="true" />
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
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
</style>
