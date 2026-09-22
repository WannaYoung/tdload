<script setup lang="ts">
import { onMounted, ref } from "vue";
import { NButton, NEmpty, NSelect, NSpin, useMessage } from "naive-ui";
import { api, ensureTicket, getToken } from "../api/http";
import type { LibraryItem } from "../api/types";
import { useNoImage, withImagePlaceholder } from "../composables/useNoImage";

defineOptions({ name: "LibraryView" });

const { noImage } = useNoImage();
const message = useMessage();
const loading = ref(false);
const items = ref<LibraryItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const chat = ref<string>("all");
const mediaType = ref<string>("all");
const mediaToken = ref("");
const broken = ref<Record<number, boolean>>({});
const filterOptions = ref<{ label: string; value: string }[]>([
  { label: "全部频道", value: "all" },
  { label: "我的收藏", value: "saved" },
]);

const mediaOptions = [
  { label: "全部", value: "all" },
  { label: "图片", value: "image" },
  { label: "视频", value: "video" },
];

function fileUrl(id: number) {
  const t = mediaToken.value || getToken();
  return t ? `/api/library/${id}/file?token=${encodeURIComponent(t)}` : "";
}

function thumbSrc(it: LibraryItem) {
  return withImagePlaceholder(fileUrl(it.id));
}

async function loadFilters() {
  try {
    const data = await api<{ items: { key: string; chatId: number; title: string }[] }>(
      "/api/library/filters",
    );
    const opts = [{ label: "全部频道", value: "all" }];
    for (const it of data.items || []) {
      if (it.key === "saved") {
        opts.push({ label: "我的收藏", value: "saved" });
      } else {
        opts.push({ label: it.title || String(it.chatId), value: String(it.chatId) });
      }
    }
    filterOptions.value = opts;
  } catch {
    /* keep default */
  }
}

async function load() {
  loading.value = true;
  broken.value = {};
  try {
    const ticket = await ensureTicket("media");
    if (ticket) mediaToken.value = ticket;
    const params = new URLSearchParams({
      page: String(page.value),
      pageSize: String(pageSize),
      mediaType: mediaType.value,
      chat: chat.value,
    });
    const data = await api<{ items: LibraryItem[]; total: number }>(`/api/library?${params}`);
    items.value = data.items || [];
    total.value = data.total || 0;
  } catch (e) {
    message.error(e instanceof Error ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function onImgError(id: number) {
  broken.value = { ...broken.value, [id]: true };
}

onMounted(() => {
  void loadFilters();
  void load();
});
</script>

<template>
  <div class="page list-page pinned">
    <div class="toolbar">
      <h2>资源库</h2>
      <n-button quaternary :loading="loading" @click="load">刷新</n-button>
    </div>

    <div class="filters">
      <n-select
        v-model:value="chat"
        :options="filterOptions"
        class="filter-chat"
        @update:value="
          page = 1;
          load();
        "
      />
      <n-select
        v-model:value="mediaType"
        :options="mediaOptions"
        class="filter-type"
        @update:value="
          page = 1;
          load();
        "
      />
    </div>

    <div class="table-wrap">
      <n-spin :show="loading" class="spin-fill">
        <n-empty v-if="!items.length && !loading" description="暂无可用文件" />
        <div v-else class="grid">
          <div v-for="it in items" :key="`${it.id}-${noImage ? 'n' : 'i'}`" class="card">
            <template v-if="it.mediaKind === 'image' && !broken[it.id]">
              <img class="thumb" :src="thumbSrc(it)" :alt="it.fileName" @error="onImgError(it.id)" />
            </template>
            <template v-else-if="it.mediaKind === 'video'">
              <video class="thumb" controls preload="metadata" :src="fileUrl(it.id)" />
            </template>
            <div v-else class="thumb placeholder">无预览</div>
            <div class="cap" :title="it.fileName">{{ it.fileName }}</div>
          </div>
        </div>
      </n-spin>
    </div>

    <div v-if="total > pageSize" class="pager">
      <n-button size="small" :disabled="page <= 1" @click="page--; load()">上一页</n-button>
      <span>{{ page }} / {{ Math.max(1, Math.ceil(total / pageSize)) }}</span>
      <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">下一页</n-button>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: nowrap;
}
.filters {
  display: flex;
  flex-direction: row;
  flex-wrap: nowrap;
  align-items: center;
  gap: 10px;
}
.filter-chat {
  flex: 1 1 auto;
  min-width: 160px;
  max-width: 360px;
}
.filter-type {
  flex: 0 0 120px;
  width: 120px;
}
.spin-fill {
  min-height: 120px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  padding-bottom: 8px;
}
.card {
  background: #18181c;
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.thumb {
  width: 100%;
  aspect-ratio: 1;
  display: block;
  object-fit: cover;
  background: #101014;
}
.thumb.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.35);
  font-size: 12px;
}
.cap {
  padding: 8px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.55);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pager {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}
</style>
