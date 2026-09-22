<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import {
  NButton,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NSwitch,
  useMessage,
} from "naive-ui";
import {
  CloudDownloadOutline,
  DesktopOutline,
  EyeOutline,
  GlobeOutline,
  InformationCircleOutline,
  SaveOutline,
} from "@vicons/ionicons5";
import { api } from "../api/http";
import type { Settings } from "../api/types";
import { useMobile } from "../composables/useMobile";
import { applyNoImageSetting } from "../composables/useNoImage";

const sectionHeadColor = "rgba(255, 255, 255, 0.92)";
const isMobile = useMobile();
const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const about = reactive({
  version: "",
  sourceUrl: "",
});

const form = reactive({
  proxy: "",
  template: "",
  threads: 8,
  concurrency: 4,
  skipSame: true,
  groupAlbum: true,
  rewriteExt: false,
  takeout: true,
  noImage: false,
  watchIntervalMinutes: 30,
});

async function load() {
  loading.value = true;
  try {
    const [s, aboutData] = await Promise.all([
      api<Settings>("/api/settings"),
      api<{ version: string; sourceUrl: string }>("/api/about").catch(() => null),
    ]);
    Object.assign(form, {
      proxy: s.proxy,
      template: s.template,
      threads: s.threads,
      concurrency: s.concurrency,
      skipSame: s.skipSame,
      groupAlbum: s.groupAlbum,
      rewriteExt: s.rewriteExt,
      takeout: s.takeout,
      noImage: !!s.noImage,
      watchIntervalMinutes: s.watchIntervalMinutes || 30,
    });
    if (aboutData) {
      about.version = aboutData.version || "";
      about.sourceUrl = aboutData.sourceUrl || "";
    }
    applyNoImageSetting(form.noImage);
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    const s = await api<Settings>("/api/settings", {
      method: "PUT",
      body: JSON.stringify({
        proxy: form.proxy,
        template: form.template,
        threads: form.threads,
        concurrency: form.concurrency,
        skipSame: form.skipSame,
        groupAlbum: form.groupAlbum,
        rewriteExt: form.rewriteExt,
        takeout: form.takeout,
        noImage: form.noImage,
        watchIntervalMinutes: form.watchIntervalMinutes,
      }),
    });
    applyNoImageSetting(!!s.noImage);
    if (s.watchIntervalMinutes) form.watchIntervalMinutes = s.watchIntervalMinutes;
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
  <n-form
    class="page settings-form"
    :label-placement="isMobile ? 'top' : 'left'"
    :label-width="isMobile ? 'auto' : 140"
    :disabled="loading"
  >
    <section class="settings-section sec-ui">
      <div class="section-head">
        <h3 class="section-title">
          <n-icon class="section-icon" :component="DesktopOutline" :size="18" :color="sectionHeadColor" />
          界面浏览
        </h3>
      </div>
      <n-form-item label="无图模式">
        <n-switch v-model:value="form.noImage" />
      </n-form-item>
    </section>

    <section class="settings-section sec-download">
      <div class="section-head">
        <h3 class="section-title">
          <n-icon class="section-icon" :component="CloudDownloadOutline" :size="18" :color="sectionHeadColor" />
          下载任务
        </h3>
      </div>
      <n-form-item label="文件名模板">
        <n-input v-model:value="form.template" />
      </n-form-item>
      <p class="hint" :class="{ mobile: isMobile }">
        文件路径：downloads / {频道ID}-{频道名称} / {模板名}.{扩展名}
      </p>
      <n-form-item label="并发数">
        <div class="pair-inputs" :class="{ mobile: isMobile }">
          <n-input-number v-model:value="form.concurrency" :min="1" :max="16" class="num" />
          <span class="pair-side-label">线程数</span>
          <n-input-number v-model:value="form.threads" :min="1" :max="32" class="num" />
        </div>
      </n-form-item>
      <n-form-item label="跳过已下载">
        <div class="switch-pair">
          <n-switch v-model:value="form.skipSame" />
          <span class="pair-side-label">相册整组</span>
          <n-switch v-model:value="form.groupAlbum" />
        </div>
      </n-form-item>
      <n-form-item label="纠正扩展名">
        <div class="switch-pair">
          <n-switch v-model:value="form.rewriteExt" />
          <span class="pair-side-label">Takeout</span>
          <n-switch v-model:value="form.takeout" />
        </div>
      </n-form-item>
    </section>

    <section class="settings-section sec-watch">
      <div class="section-head">
        <h3 class="section-title">
          <n-icon class="section-icon" :component="EyeOutline" :size="18" :color="sectionHeadColor" />
          监听
        </h3>
      </div>
      <n-form-item label="执行间隔（分钟）">
        <n-input-number
          v-model:value="form.watchIntervalMinutes"
          :min="10"
          :max="300"
          :step="5"
          class="num"
        />
      </n-form-item>
      <p class="hint" :class="{ mobile: isMobile }">
        默认 30 分钟，范围 10–300。仅下载加入监听后新增区间内的消息（含端点），下载前按索引去重。
      </p>
    </section>

    <section class="settings-section sec-proxy">
      <div class="section-head">
        <h3 class="section-title">
          <n-icon class="section-icon" :component="GlobeOutline" :size="18" :color="sectionHeadColor" />
          网络
        </h3>
      </div>
      <n-form-item label="代理">
        <n-input v-model:value="form.proxy" placeholder="socks5://127.0.0.1:1080 或 http://…" />
      </n-form-item>
    </section>

    <section class="settings-section sec-info">
      <div class="section-head">
        <h3 class="section-title">
          <n-icon
            class="section-icon"
            :component="InformationCircleOutline"
            :size="18"
            :color="sectionHeadColor"
          />
          系统信息
        </h3>
      </div>
      <n-form-item label="版本">
        <span>{{ about.version || "—" }}</span>
      </n-form-item>
      <n-form-item label="源码">
        <a v-if="about.sourceUrl" class="source-link" :href="about.sourceUrl" target="_blank" rel="noopener">
          {{ about.sourceUrl }}
        </a>
        <span v-else>—</span>
      </n-form-item>
    </section>

    <div class="form-actions">
      <n-button type="primary" :loading="saving" :disabled="loading" @click="save">
        <template #icon>
          <n-icon :component="SaveOutline" />
        </template>
        保存
      </n-button>
    </div>
  </n-form>
</template>

<style scoped>
.settings-form {
  width: 100%;
  min-width: 0;
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
  align-items: stretch;
}
.settings-section {
  margin: 0;
  padding: 16px 16px 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.025);
  min-width: 0;
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0 0 14px;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 15px;
  font-weight: 650;
  letter-spacing: 0.02em;
  color: v-bind(sectionHeadColor);
}
.section-icon {
  flex-shrink: 0;
}
.num {
  width: 100%;
  max-width: 180px;
}
.pair-inputs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
  width: 100%;
}
.pair-inputs .num {
  flex: 1 1 120px;
  max-width: 180px;
}
.pair-side-label {
  flex: 0 0 auto;
  color: rgba(255, 255, 255, 0.82);
  font-size: 14px;
  white-space: nowrap;
}
.pair-inputs.mobile .num {
  max-width: none;
}
.switch-pair {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 60px;
}
.hint {
  margin: -8px 0 12px 140px;
  color: rgba(255, 255, 255, 0.45);
  font-size: 12px;
  line-height: 1.5;
}
.hint.mobile {
  margin-left: 0;
}
.source-link {
  color: #f472b6;
  word-break: break-all;
  text-decoration: none;
}
.source-link:hover {
  text-decoration: underline;
}
.form-actions {
  margin-top: 0;
}

@media (min-width: 1100px) {
  .settings-form {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    grid-template-areas:
      "ui download"
      "watch download"
      "proxy download"
      "info download"
      "actions actions";
  }
  .sec-ui {
    grid-area: ui;
  }
  .sec-download {
    grid-area: download;
  }
  .sec-watch {
    grid-area: watch;
  }
  .sec-proxy {
    grid-area: proxy;
  }
  .sec-info {
    grid-area: info;
  }
  .form-actions {
    grid-area: actions;
  }
}

@media (max-width: 640px) {
  .settings-section {
    padding: 14px 12px 4px;
  }
  .num {
    max-width: none;
  }
}
</style>
