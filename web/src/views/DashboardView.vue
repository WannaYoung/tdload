<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { NAlert, NButton, NIcon, NSpin, NTag } from "naive-ui";
import {
  CloudDownloadOutline,
  EyeOutline,
  FolderOpenOutline,
  PaperPlaneOutline,
  RefreshOutline,
  SettingsOutline,
} from "@vicons/ionicons5";
import { api } from "../api/http";
import type { DashboardStats } from "../api/types";
import { useAuthStore } from "../stores/auth";

const router = useRouter();
const auth = useAuthStore();
const loading = ref(true);
const error = ref("");
const stats = ref<DashboardStats | null>(null);

const greeting = computed(() => {
  const hour = new Date().getHours();
  if (hour < 6) return "夜深了";
  if (hour < 12) return "上午好";
  if (hour < 18) return "下午好";
  return "晚上好";
});

const subtitle = computed(() => {
  const s = stats.value;
  if (!s) return `${greeting.value}，${auth.user?.username || "管理员"}`;
  if (!s.tgConfigured) return "请先在设置中配置 Telegram API，再到 Telegram 页登录";
  if (s.tgExpired > 0) return "Telegram 会话已失效，请重新登录";
  if (s.tasksRunning > 0) return `当前有 ${s.tasksRunning} 个任务正在下载`;
  if (s.tasksQueued > 0) return `队列中还有 ${s.tasksQueued} 个任务等待处理`;
  if (s.tasksFailed > 0) return `${s.tasksFailed} 个任务失败，可在任务页重试`;
  return "系统运行正常";
});

const kpis = computed(() => {
  const s = stats.value;
  return [
    { label: "排队", value: s?.tasksQueued ?? 0, tone: "muted" },
    { label: "下载中", value: s?.tasksRunning ?? 0, tone: "pink" },
    { label: "失败", value: s?.tasksFailed ?? 0, tone: "warn" },
    { label: "已完成", value: s?.tasksDone ?? 0, tone: "muted" },
    { label: "资料库", value: s?.media ?? 0, tone: "muted" },
  ];
});

async function load() {
  loading.value = true;
  error.value = "";
  try {
    stats.value = await api<DashboardStats>("/api/dashboard");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    loading.value = false;
  }
}

function go(name: string) {
  void router.push({ name });
}

onMounted(() => {
  void load();
});

let timer: number | null = null;
onMounted(() => {
  timer = window.setInterval(() => void load(), 8000);
});
onUnmounted(() => {
  if (timer != null) window.clearInterval(timer);
});
</script>

<template>
  <div class="page dash">
    <header class="hero">
      <div>
        <h1>{{ greeting }}，{{ auth.user?.username || "管理员" }}</h1>
        <p>{{ subtitle }}</p>
      </div>
      <n-button quaternary circle :disabled="loading" @click="load">
        <template #icon>
          <n-icon :component="RefreshOutline" />
        </template>
      </n-button>
    </header>

    <n-alert v-if="error" type="error" style="margin-bottom: 16px">{{ error }}</n-alert>

    <n-spin :show="loading && !stats">
      <div class="kpi-grid">
        <div v-for="k in kpis" :key="k.label" class="kpi" :class="k.tone">
          <div class="kpi-label">{{ k.label }}</div>
          <div class="kpi-value">{{ k.value }}</div>
        </div>
      </div>

      <div class="status-row">
        <n-tag :type="stats?.tgConfigured ? 'success' : 'warning'" size="small">
          API {{ stats?.tgConfigured ? "已配置" : "未配置" }}
        </n-tag>
        <n-tag :type="stats?.tgActive ? 'success' : 'default'" size="small">
          TG {{ stats?.tgActive ? "已登录" : "未登录" }}
        </n-tag>
        <span class="path">{{ stats?.downloadDir }}</span>
      </div>

      <div class="quick">
        <n-button secondary @click="go('telegram')">
          <template #icon><n-icon :component="PaperPlaneOutline" /></template>
          Telegram
        </n-button>
        <n-button type="primary" @click="go('tasks')">
          <template #icon><n-icon :component="CloudDownloadOutline" /></template>
          任务
        </n-button>
        <n-button secondary @click="go('library')">
          <template #icon><n-icon :component="FolderOpenOutline" /></template>
          资料库
        </n-button>
        <n-button secondary @click="go('watch')">
          <template #icon><n-icon :component="EyeOutline" /></template>
          监听
        </n-button>
        <n-button secondary @click="go('settings')">
          <template #icon><n-icon :component="SettingsOutline" /></template>
          设置
        </n-button>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.dash {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1100px;
}
.hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.hero h1 {
  margin: 0;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.hero p {
  margin: 8px 0 0;
  color: rgba(255, 255, 255, 0.55);
  font-size: 14px;
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
}
.kpi {
  padding: 16px 18px;
  border-radius: 12px;
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.kpi.pink {
  border-color: rgba(244, 114, 182, 0.35);
  background: linear-gradient(160deg, rgba(244, 114, 182, 0.12), #18181c 60%);
}
.kpi.warn {
  border-color: rgba(250, 173, 20, 0.35);
}
.kpi-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
}
.kpi-value {
  margin-top: 8px;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.kpi.pink .kpi-value {
  color: #f9a8d4;
}
.status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 16px;
}
.path {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
  word-break: break-all;
}
.quick {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 20px;
}
@media (max-width: 1000px) {
  .kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .hero h1 {
    font-size: 22px;
  }
}
</style>
