<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { NIcon, NProgress, NTag } from "naive-ui";
import {
  FolderOpenOutline,
  HeartOutline,
  PaperPlaneOutline,
  PeopleOutline,
  ServerOutline,
} from "@vicons/ionicons5";
import type { DashboardStats } from "../../../api/types";

defineProps<{
  stats: DashboardStats;
  diskPercent: number;
  diskHint: string;
  dialogsSyncedText: string;
}>();

const emit = defineEmits<{
  navigate: [name: string];
}>();

const { t } = useI18n();
</script>

<template>
  <section class="panel">
    <header class="panel-head">
      <h2>{{ t("resource.title") }}</h2>
    </header>
    <div class="resource-list">
      <button class="resource" type="button" @click="emit('navigate', 'telegram')">
        <div class="resource-icon tone-tg">
          <n-icon :component="PaperPlaneOutline" :size="20" />
        </div>
        <div class="resource-body">
          <div class="resource-top">
            <span>{{ t("layout.telegram") }}</span>
            <strong>{{ stats.tgActive ? t("common.loggedIn") : t("common.notLoggedIn") }}</strong>
          </div>
          <div class="resource-hint">
            <n-tag size="tiny" :type="stats.tgConfigured ? 'success' : 'warning'">
              API {{ stats.tgConfigured ? t("resource.apiConfigured") : t("resource.apiNotConfigured") }}
            </n-tag>
            <n-tag v-if="stats.proxyConfigured" size="tiny" type="info">{{ t("resource.proxyOn") }}</n-tag>
          </div>
        </div>
      </button>

      <button class="resource" type="button" @click="emit('navigate', 'channels')">
        <div class="resource-icon tone-ch">
          <n-icon :component="PeopleOutline" :size="20" />
        </div>
        <div class="resource-body">
          <div class="resource-top">
            <span>{{ t("resource.channelsGroups") }}</span>
            <strong>{{ stats.dialogCount ?? 0 }}</strong>
          </div>
          <div class="resource-hint">{{ dialogsSyncedText }}</div>
        </div>
      </button>

      <button class="resource" type="button" @click="emit('navigate', 'saved')">
        <div class="resource-icon tone-fav">
          <n-icon :component="HeartOutline" :size="20" />
        </div>
        <div class="resource-body">
          <div class="resource-top">
            <span>{{ t("resource.saved") }}</span>
            <strong>{{ stats.savedDownloaded ?? 0 }}/{{ stats.savedCount ?? 0 }}</strong>
          </div>
          <div class="resource-hint">{{ t("resource.savedHint") }}</div>
        </div>
      </button>

      <button class="resource" type="button" @click="emit('navigate', 'library')">
        <div class="resource-icon tone-lib">
          <n-icon :component="FolderOpenOutline" :size="20" />
        </div>
        <div class="resource-body">
          <div class="resource-top">
            <span>{{ t("resource.library") }}</span>
            <strong>{{ stats.media }}</strong>
          </div>
          <div class="resource-hint">{{ t("resource.libraryHint") }}</div>
        </div>
      </button>

      <div class="resource static">
        <div class="resource-icon tone-disk">
          <n-icon :component="ServerOutline" :size="20" />
        </div>
        <div class="resource-body">
          <div class="resource-top">
            <span>{{ t("resource.disk") }}</span>
            <strong>{{ stats.diskTotal ? `${diskPercent}%` : t("common.dash") }}</strong>
          </div>
          <n-progress
            v-if="stats.diskTotal"
            type="line"
            :percentage="diskPercent"
            :height="6"
            :show-indicator="false"
            :status="diskPercent > 90 ? 'error' : diskPercent > 75 ? 'warning' : 'success'"
          />
          <div class="resource-hint path">{{ diskHint }}</div>
          <div v-if="stats.downloadDir" class="resource-hint path">{{ stats.downloadDir }}</div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.panel {
  min-width: 0;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid rgba(255, 255, 255, 0.07);
  padding: 16px 16px 8px;
}
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 4px;
}
.panel-head h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.resource-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.resource {
  display: flex;
  gap: 12px;
  align-items: center;
  width: 100%;
  padding: 12px 8px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.resource:hover {
  background: rgba(255, 255, 255, 0.04);
}
.resource-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.tone-tg {
  background: rgba(56, 189, 248, 0.14);
  color: #7dd3fc;
}
.tone-ch {
  background: rgba(167, 139, 250, 0.14);
  color: #c4b5fd;
}
.tone-fav {
  background: rgba(244, 114, 182, 0.14);
  color: #f9a8d4;
}
.tone-lib {
  background: rgba(251, 191, 36, 0.14);
  color: #fcd34d;
}
.tone-disk {
  background: rgba(94, 234, 212, 0.14);
  color: #5eead4;
}
.resource.static {
  cursor: default;
}
.resource.static:hover {
  background: transparent;
}
.resource-body {
  flex: 1;
  min-width: 0;
}
.resource-top {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
}
.resource-top strong {
  font-size: 18px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  color: #fff;
}
.resource-hint {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin-top: 4px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.38);
}
.resource-hint.path {
  word-break: break-all;
}
</style>
