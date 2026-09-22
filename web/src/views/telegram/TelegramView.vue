<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NSpin,
  useDialog,
  useMessage,
} from "naive-ui";
import { api } from "../../api/http";
import type { TgStatus, TGSummary } from "../../api/types";

const message = useMessage();
const dialog = useDialog();
const loading = ref(true);
const busy = ref(false);
const status = ref<TgStatus | null>(null);
const summary = ref<TGSummary | null>(null);
const syncing = ref(false);
const step = ref<"idle" | "code" | "password">("idle");
const loginId = ref("");

const form = reactive({
  phone: "",
  code: "",
  password: "",
});

const loggedIn = computed(() => Boolean(status.value?.loggedIn));

const syncedAtText = computed(() => {
  const a = summary.value?.dialogsSyncedAt || "";
  const b = summary.value?.savedSyncedAt || "";
  const raw = [a, b].filter(Boolean).sort().at(-1) || "";
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
});

async function loadSummary() {
  try {
    summary.value = await api<TGSummary>("/api/tg/summary");
  } catch {
    summary.value = null;
  }
}

async function load() {
  loading.value = true;
  try {
    status.value = await api<TgStatus>("/api/tg/status");
    await loadSummary();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function syncAll() {
  if (!loggedIn.value) {
    message.warning("请先登录 Telegram");
    return;
  }
  syncing.value = true;
  try {
    await api("/api/tg/sync", { method: "POST", body: JSON.stringify({ scope: "all" }) });
    message.success("已同步频道/群与收藏");
    await loadSummary();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "同步失败");
  } finally {
    syncing.value = false;
  }
}

async function sendCode() {
  busy.value = true;
  try {
    const res = await api<{ loginId: string; message: string }>("/api/tg/login/send_code", {
      method: "POST",
      body: JSON.stringify({ phone: form.phone }),
    });
    loginId.value = res.loginId;
    step.value = "code";
    form.code = "";
    form.password = "";
    message.success(res.message || "验证码已发送");
  } catch (e) {
    message.error(e instanceof Error ? e.message : "发送失败");
  } finally {
    busy.value = false;
  }
}

async function signIn() {
  busy.value = true;
  try {
    const res = await api<{ needPassword: boolean; user?: { id: number } }>(
      "/api/tg/login/sign_in",
      {
        method: "POST",
        body: JSON.stringify({
          loginId: loginId.value,
          code: form.code,
          password: form.password,
        }),
      },
    );
    if (res.needPassword) {
      step.value = "password";
      message.info("该账号开启了两步验证，请输入密码");
      return;
    }
    message.success("登录成功");
    step.value = "idle";
    form.code = "";
    form.password = "";
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "登录失败");
  } finally {
    busy.value = false;
  }
}

function confirmLogout() {
  dialog.warning({
    title: "退出 Telegram",
    content: "确定退出 Telegram 会话？之后下载需要重新登录。",
    positiveText: "退出",
    negativeText: "取消",
    onPositiveClick: () => logout(),
  });
}

async function logout() {
  busy.value = true;
  try {
    await api("/api/tg/logout", { method: "POST", body: "{}" });
    message.success("已退出");
    step.value = "idle";
    summary.value = null;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "退出失败");
  } finally {
    busy.value = false;
  }
}

onMounted(() => void load());
</script>

<template>
  <div class="page">
    <header class="head">
      <div>
        <h2>Telegram</h2>
        <p>使用手机号验证码登录；会话保存在配置目录，重启后可复用。</p>
      </div>
      <n-button secondary :loading="loading" @click="load">刷新</n-button>
    </header>

    <n-spin :show="loading">
      <n-card size="small" class="block">
        <h3>账户</h3>
        <n-alert :type="loggedIn ? 'success' : 'info'" style="margin-bottom: 12px">
          {{ status?.message || "未知状态" }}
        </n-alert>

        <div v-if="status?.user || loggedIn" class="user-bar">
          <div v-if="status?.user" class="user-row">
            <div class="user-item">
              <span class="label">ID</span>
              <span>{{ status.user.id }}</span>
            </div>
            <div v-if="status.user.username" class="user-item">
              <span class="label">用户名</span>
              <span>@{{ status.user.username }}</span>
            </div>
            <div v-if="status.user.phone" class="user-item">
              <span class="label">手机</span>
              <span>{{ status.user.phone }}</span>
            </div>
          </div>
          <n-button
            v-if="loggedIn"
            type="error"
            secondary
            size="small"
            :loading="busy"
            @click="confirmLogout"
          >
            退出登录
          </n-button>
        </div>
      </n-card>

      <n-card v-if="loggedIn" size="small" class="block">
        <div class="card-title">
          <h3>对话与收藏</h3>
          <span v-if="syncedAtText" class="synced-at">上次同步 {{ syncedAtText }}</span>
        </div>
        <div class="stats-bar">
          <div class="stats">
            <div>
              <span class="label">频道/群</span>
              {{ summary?.dialogCount ?? 0 }}
            </div>
            <div>
              <span class="label">收藏</span>
              {{ summary?.savedCount ?? 0 }}
            </div>
            <div>
              <span class="label">收藏已下</span>
              {{ summary?.savedDownloaded ?? 0 }}
            </div>
          </div>
          <n-button type="primary" size="small" :loading="syncing" @click="syncAll">
            同步收藏/对话
          </n-button>
        </div>
      </n-card>

      <n-card v-if="!loggedIn" size="small" class="block">
        <h3>验证码登录</h3>
        <n-alert v-if="status && !status.configured" type="warning" style="margin-bottom: 12px">
          API 凭证未就绪，请重启后端（会自动写入 Desktop 公开凭证）。
        </n-alert>
        <n-form label-placement="top">
          <n-form-item label="手机号（含国际区号）">
            <n-input
              v-model:value="form.phone"
              placeholder="+86138xxxxxxxx"
              :disabled="busy || step !== 'idle' || !status?.configured"
            />
          </n-form-item>
          <n-button
            v-if="step === 'idle'"
            type="primary"
            :loading="busy"
            :disabled="!form.phone || !status?.configured"
            @click="sendCode"
          >
            发送验证码
          </n-button>

          <template v-if="step === 'code' || step === 'password'">
            <n-form-item v-if="step === 'code'" label="验证码">
              <n-input v-model:value="form.code" placeholder="Telegram / 短信验证码" />
            </n-form-item>
            <n-form-item label="两步验证密码（如有）">
              <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="step === 'password' ? '必填' : '未开启可留空'"
              />
            </n-form-item>
            <div class="actions">
              <n-button type="primary" :loading="busy" @click="signIn">登录</n-button>
              <n-button quaternary :disabled="busy" @click="step = 'idle'">重新开始</n-button>
            </div>
          </template>
        </n-form>
      </n-card>
    </n-spin>
  </div>
</template>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 16px;
}
.head h2 {
  margin: 0;
  font-size: 22px;
}
.head p {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
}
.block {
  margin-bottom: 14px;
}
.block h3 {
  margin: 0;
  font-size: 15px;
  color: #f9a8d4;
}
.card-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.synced-at {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  font-weight: 400;
}
.hint {
  margin: 12px 0 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.45);
}
.user-bar,
.stats-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.user-row {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
  font-size: 14px;
  color: rgba(255, 255, 255, 0.78);
  min-width: 0;
}
.user-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.user-item .label,
.stats .label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
}
.actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
  font-size: 15px;
  min-width: 0;
}
.stats .label {
  display: block;
  margin-bottom: 4px;
}
</style>