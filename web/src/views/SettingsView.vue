<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import {
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSwitch,
  useMessage,
} from "naive-ui";
import { api } from "../api/http";
import type { Settings } from "../api/types";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const form = reactive({
  downloadDir: "",
  proxy: "",
  template: "",
  threads: 8,
  concurrency: 4,
  skipSame: true,
  groupAlbum: true,
  rewriteExt: false,
  takeout: false,
  appId: 0,
  appHashSet: false,
});

async function load() {
  loading.value = true;
  try {
    const s = await api<Settings>("/api/settings");
    Object.assign(form, {
      downloadDir: s.downloadDir,
      proxy: s.proxy,
      template: s.template,
      threads: s.threads,
      concurrency: s.concurrency,
      skipSame: s.skipSame,
      groupAlbum: s.groupAlbum,
      rewriteExt: s.rewriteExt,
      takeout: s.takeout,
      appId: s.appId,
      appHashSet: s.appHashSet,
    });
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    await api<Settings>("/api/settings", {
      method: "PUT",
      body: JSON.stringify({
        downloadDir: form.downloadDir,
        proxy: form.proxy,
        template: form.template,
        threads: form.threads,
        concurrency: form.concurrency,
        skipSame: form.skipSame,
        groupAlbum: form.groupAlbum,
        rewriteExt: form.rewriteExt,
        takeout: form.takeout,
      }),
    });
    message.success("已保存");
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

onMounted(() => void load());
</script>

<template>
  <div class="page settings">
    <header class="head">
      <h2>设置</h2>
      <p>下载参数写回 config.yaml；Telegram API 凭证建议用环境变量 TG_APP_ID / TG_APP_HASH。</p>
    </header>

    <n-card size="small" :bordered="true">
      <n-form label-placement="top" :show-feedback="false">
        <n-form-item label="下载目录">
          <n-input v-model:value="form.downloadDir" :disabled="loading" />
        </n-form-item>
        <n-form-item label="代理（socks5:// 或 http://）">
          <n-input v-model:value="form.proxy" placeholder="可选" :disabled="loading" />
        </n-form-item>
        <n-form-item label="文件名模板">
          <n-input v-model:value="form.template" :disabled="loading" />
        </n-form-item>
        <p class="hint" style="margin-top: -8px; margin-bottom: 12px">
          实际路径：下载目录 / 频道ID_频道名称 / 模板文件名。可用字段 DialogID、MessageID、FileName 等。
        </p>
        <div class="row">
          <n-form-item label="单任务线程 (-t)">
            <n-input-number v-model:value="form.threads" :min="1" :max="32" :disabled="loading" />
          </n-form-item>
          <n-form-item label="并发任务 (-l)">
            <n-input-number
              v-model:value="form.concurrency"
              :min="1"
              :max="16"
              :disabled="loading"
            />
          </n-form-item>
        </div>
        <div class="switches">
          <label><n-switch v-model:value="form.skipSame" /> 跳过同名同大小</label>
          <label><n-switch v-model:value="form.groupAlbum" /> 相册整组下载</label>
          <label><n-switch v-model:value="form.rewriteExt" /> 按 MIME 纠正扩展名</label>
          <label><n-switch v-model:value="form.takeout" /> Takeout 会话</label>
        </div>
        <p class="hint">
          App ID：{{ form.appId || "未设置" }} · App Hash：{{ form.appHashSet ? "已设置" : "未设置" }}
        </p>
        <div class="form-actions">
          <n-button type="primary" :loading="saving" :disabled="loading" @click="save">
            保存
          </n-button>
        </div>
      </n-form>
    </n-card>
  </div>
</template>

<style scoped>
.settings {
  max-width: 720px;
}
.head {
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
.row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.switches {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 8px 0 16px;
}
.switches label {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
}
.hint {
  font-size: 12px;
  color: rgba(249, 168, 212, 0.85);
  margin: 0 0 12px;
}
@media (max-width: 1000px) {
  .row {
    grid-template-columns: 1fr;
  }
}
</style>
