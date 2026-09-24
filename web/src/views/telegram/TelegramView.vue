<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NSpin,
  useDialog,
  useMessage,
} from "naive-ui";
import { RefreshOutline, SyncOutline } from "@vicons/ionicons5";
import { api } from "../../api/http";
import type { TgStatus, TGSummary } from "../../api/types";

const { t } = useI18n();
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
    message.error(e instanceof Error ? e.message : t("common.loadFailed"));
  } finally {
    loading.value = false;
  }
}

async function syncAll() {
  if (!loggedIn.value) {
    message.warning(t("telegram.needLogin"));
    return;
  }
  syncing.value = true;
  try {
    await api("/api/tg/sync", { method: "POST", body: JSON.stringify({ scope: "all" }) });
    message.success(t("telegram.syncSuccess"));
    await loadSummary();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.syncFailed"));
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
    message.success(res.message || t("telegram.codeSent"));
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("telegram.sendFailed"));
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
      message.info(t("telegram.need2FA"));
      return;
    }
    message.success(t("telegram.loginSuccess"));
    step.value = "idle";
    form.code = "";
    form.password = "";
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("telegram.loginFailed"));
  } finally {
    busy.value = false;
  }
}

function confirmLogout() {
  dialog.warning({
    title: t("telegram.logoutTitle"),
    content: t("telegram.logoutContent"),
    positiveText: t("telegram.logoutConfirm"),
    negativeText: t("common.cancel"),
    onPositiveClick: () => logout(),
  });
}

async function logout() {
  busy.value = true;
  try {
    await api("/api/tg/logout", { method: "POST", body: "{}" });
    message.success(t("telegram.logoutSuccess"));
    step.value = "idle";
    summary.value = null;
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("telegram.logoutFailed"));
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
        <h2>{{ t("telegram.title") }}</h2>
      </div>
      <n-button :loading="loading" @click="load">
        <template #icon>
          <n-icon :component="RefreshOutline" />
        </template>
        {{ t("common.refresh") }}
      </n-button>
    </header>

    <n-spin :show="loading">
      <n-card size="small" class="block">
        <h3>{{ t("telegram.account") }}</h3>
        <n-alert :type="loggedIn ? 'success' : 'info'" style="margin-bottom: 12px">
          {{ status?.message || t("telegram.unknownStatus") }}
        </n-alert>

        <div v-if="status?.user || loggedIn" class="user-bar">
          <div v-if="status?.user" class="user-row">
            <div class="user-item">
              <span class="label">{{ t("telegram.id") }}</span>
              <span>{{ status.user.id }}</span>
            </div>
            <div v-if="status.user.username" class="user-item">
              <span class="label">{{ t("telegram.username") }}</span>
              <span>@{{ status.user.username }}</span>
            </div>
            <div v-if="status.user.phone" class="user-item">
              <span class="label">{{ t("telegram.phone") }}</span>
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
            {{ t("telegram.logout") }}
          </n-button>
        </div>
      </n-card>

      <n-card v-if="loggedIn" size="small" class="block">
        <div class="card-title">
          <h3>{{ t("telegram.dialogsTitle") }}</h3>
          <span v-if="syncedAtText" class="synced-at">{{ t("telegram.lastSynced", { time: syncedAtText }) }}</span>
        </div>
        <div class="stats-bar">
          <div class="stats">
            <div>
              <span class="label">{{ t("telegram.channelsGroups") }}</span>
              {{ summary?.dialogCount ?? 0 }}
            </div>
            <div>
              <span class="label">{{ t("telegram.saved") }}</span>
              {{ summary?.savedCount ?? 0 }}
            </div>
            <div>
              <span class="label">{{ t("telegram.savedDownloaded") }}</span>
              {{ summary?.savedDownloaded ?? 0 }}
            </div>
          </div>
          <n-button type="primary" size="small" :loading="syncing" @click="syncAll">
            <template #icon>
              <n-icon :component="SyncOutline" />
            </template>
            {{ t("common.sync") }}
          </n-button>
        </div>
      </n-card>

      <n-card v-if="!loggedIn" size="small" class="block">
        <h3>{{ t("telegram.loginTitle") }}</h3>
        <n-alert v-if="status && !status.configured" type="warning" style="margin-bottom: 12px">
          {{ t("telegram.apiNotReady") }}
        </n-alert>
        <n-form label-placement="top">
          <n-form-item :label="t('telegram.phoneLabel')">
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
            {{ t("telegram.sendCode") }}
          </n-button>

          <template v-if="step === 'code' || step === 'password'">
            <n-form-item v-if="step === 'code'" :label="t('telegram.codeLabel')">
              <n-input v-model:value="form.code" :placeholder="t('telegram.codePlaceholder')" />
            </n-form-item>
            <n-form-item :label="t('telegram.passwordLabel')">
              <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="step === 'password' ? t('telegram.passwordRequired') : t('telegram.passwordOptional')"
              />
            </n-form-item>
            <div class="actions">
              <n-button type="primary" :loading="busy" @click="signIn">{{ t("telegram.signIn") }}</n-button>
              <n-button quaternary :disabled="busy" @click="step = 'idle'">{{ t("telegram.restart") }}</n-button>
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
