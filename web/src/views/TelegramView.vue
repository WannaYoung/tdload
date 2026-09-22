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
import { api } from "../api/http";
import type { Settings, TgStatus, TGSummary } from "../api/types";

const message = useMessage();
const dialog = useDialog();
const loading = ref(true);
const busy = ref(false);
const status = ref<TgStatus | null>(null);
const summary = ref<TGSummary | null>(null);
const syncing = ref(false);
const step = ref<"idle" | "code" | "password">("idle");
const loginId = ref("");
const showCredForm = ref(false);

const form = reactive({
  phone: "",
  code: "",
  password: "",
});

const apiForm = reactive({
  appId: "" as string,
  appHash: "",
});

const loggedIn = computed(() => Boolean(status.value?.loggedIn));

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
    const s = await api<Settings>("/api/settings");
    apiForm.appId = s.appId ? String(s.appId) : "";
    showCredForm.value = false;
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

async function saveAPI() {
  const id = Number(apiForm.appId);
  if (!id || !apiForm.appHash.trim()) {
    message.warning("请填写 App ID 与 App Hash");
    return;
  }
  busy.value = true;
  try {
    await api("/api/settings", {
      method: "PUT",
      body: JSON.stringify({ appId: id, appHash: apiForm.appHash.trim() }),
    });
    message.success("API 凭证已保存");
    apiForm.appHash = "";
    showCredForm.value = false;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    busy.value = false;
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
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "退出失败");
  } finally {
    busy.value = false;
  }
}

async function useDesktopPreset() {
  busy.value = true;
  try {
    await api("/api/tg/credentials/desktop", { method: "POST", body: "{}" });
    message.success("已使用 Desktop 内置凭证，可直接登录");
    showCredForm.value = false;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "设置失败");
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
        <h3>API 凭证</h3>
        <n-alert v-if="!status?.configured" type="warning" style="margin-bottom: 12px">
          尚未配置凭证。默认应已自动写入 Desktop 公开凭证，也可点下面按钮重新写入。
        </n-alert>
        <n-alert
          v-else
          :type="status?.usingDesktopPreset ? 'success' : 'info'"
          style="margin-bottom: 12px"
        >
          {{
            status?.usingDesktopPreset
              ? "当前使用 Desktop 公开凭证（与 tdl 相同，可直接登录 / 发 Docker）"
              : "当前使用自定义 API 凭证"
          }}
        </n-alert>
        <div class="actions" style="margin-bottom: 12px">
          <n-button
            v-if="!status?.usingDesktopPreset"
            type="primary"
            :loading="busy"
            @click="useDesktopPreset"
          >
            使用 Desktop 公开凭证
          </n-button>
          <n-button quaternary size="small" @click="showCredForm = !showCredForm">
            {{ showCredForm ? "收起手动填写" : "改用自己的 api_id" }}
          </n-button>
        </div>
        <p class="hint">
          公开凭证来自 Telegram Desktop（api_id=2040）。若遇
          <code>API_ID_PUBLISHED_FLOOD</code>，再换自己的凭证。
        </p>
        <template v-if="showCredForm">
          <n-form label-placement="top">
            <div class="row">
              <n-form-item label="App ID">
                <n-input v-model:value="apiForm.appId" placeholder="数字" />
              </n-form-item>
              <n-form-item label="App Hash">
                <n-input
                  v-model:value="apiForm.appHash"
                  type="password"
                  show-password-on="click"
                />
              </n-form-item>
            </div>
            <div class="actions">
              <n-button type="primary" :loading="busy" @click="saveAPI">保存凭证</n-button>
            </div>
          </n-form>
        </template>
      </n-card>
      <n-card v-if="loggedIn" size="small" class="block">
        <h3>对话与收藏</h3>
        <div class="stats">
          <div><span class="label">频道/群</span> {{ summary?.dialogCount ?? 0 }}</div>
          <div><span class="label">收藏</span> {{ summary?.savedCount ?? 0 }}</div>
          <div>
            <span class="label">收藏已下</span> {{ summary?.savedDownloaded ?? 0 }}
          </div>
        </div>
        <p v-if="summary?.dialogsSyncedAt" class="hint">
          上次同步：对话 {{ summary.dialogsSyncedAt }} · 收藏 {{ summary.savedSyncedAt || "—" }}
        </p>
        <n-button type="primary" :loading="syncing" @click="syncAll">同步频道、群组与收藏</n-button>
      </n-card>

      <n-card size="small" class="block">
        <h3>登录状态</h3>
        <n-alert :type="loggedIn ? 'success' : 'info'" style="margin-bottom: 12px">
          {{ status?.message || "未知状态" }}
        </n-alert>
        <div v-if="status?.user" class="user">
          <div>ID：{{ status.user.id }}</div>
          <div v-if="status.user.username">用户名：@{{ status.user.username }}</div>
          <div v-if="status.user.firstName">昵称：{{ status.user.firstName }}</div>
          <div v-if="status.user.phone">手机：{{ status.user.phone }}</div>
        </div>
        <n-button v-if="loggedIn" type="error" secondary :loading="busy" @click="confirmLogout">
          退出 Telegram
        </n-button>
      </n-card>

      <n-card v-if="status?.configured && !loggedIn" size="small" class="block">
        <h3>验证码登录</h3>
        <n-form label-placement="top">
          <n-form-item label="手机号（含国际区号）">
            <n-input
              v-model:value="form.phone"
              placeholder="+86138xxxxxxxx"
              :disabled="busy || step !== 'idle'"
            />
          </n-form-item>
          <n-button
            v-if="step === 'idle'"
            type="primary"
            :loading="busy"
            :disabled="!form.phone"
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
  margin: 0 0 8px;
  font-size: 15px;
  color: #f9a8d4;
}
.hint {
  margin: 0 0 12px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.45);
}
.hint a {
  color: #f9a8d4;
}
.row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.user {
  margin-bottom: 12px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
  line-height: 1.7;
}
.actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
  margin-bottom: 12px;
  font-size: 15px;
}
.stats .label {
  display: block;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  margin-bottom: 4px;
}
@media (max-width: 1000px) {
  .row {
    grid-template-columns: 1fr;
  }
}
</style>
