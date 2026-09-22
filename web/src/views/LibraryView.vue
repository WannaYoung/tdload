<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  NButton,
  NEmpty,
  NImage,
  NSelect,
  NSpin,
  useMessage,
} from "naive-ui";
import { api, getToken } from "../api/http";
import type { LibraryItem } from "../api/types";

defineOptions({ name: "LibraryView" });

const message = useMessage();
const loading = ref(false);
const items = ref<LibraryItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const chat = ref<string>("all");
const mediaType = ref<string>("all");
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
  const t = getToken();
  return t ? `/api/library/${id}/file?token=${encodeURIComponent(t)}` : "";
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
        opts.push({ label: it.title, value: String(it.chatId) });
      }
    }
    filterOptions.value = opts;
  } catch {
    /* keep default */
  }
}

async function load() {
  loading.value = true;
  try {
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

const previewItems = computed(() =>
  items.value.filter((i) => i.mediaKind === "image" || i.mediaKind === "video"),
);

onMounted(() => {
  void loadFilters();
  void load();
});
</script>

<template>
  <div class="page list-page pinned">
    <div class="toolbar">
      <div>
        <h2>资源库</h2>
        <p class="muted">按频道筛选；「我的收藏」在频道列表最前</p>
      </div>
      <n-button quaternary :loading="loading" @click="load">刷新</n-button>
    </div>

    <div class="filters">
      <n-select v-model:value="chat" :options="filterOptions" style="min-width: 200px" @update:value="page = 1; load()" />
      <n-select
        v-model:value="mediaType"
        :options="mediaOptions"
        style="width: 120px"
        @update:value="page = 1; load()"
      />
    </div>

    <n-spin :show="loading">
      <n-empty v-if="!items.length && !loading" description="暂无资源，先完成下载" />
      <div v-else class="grid">
        <div v-for="it in previewItems" :key="it.id" class="card">
          <n-image v-if="it.mediaKind === 'image'" :src="fileUrl(it.id)" object-fit="cover" class="thumb" />
          <video v-else-if="it.mediaKind === 'video'" class="thumb" controls :src="fileUrl(it.id)" />
          <div class="cap">{{ it.fileName }}</div>
        </div>
      </div>
      <ul v-if="items.length" class="list">
        <li v-for="it in items" :key="'l-' + it.id">
          <span class="kind">{{ it.mediaKind }}</span> {{ it.fileName }}
          <span class="path">{{ it.localPath }}</span>
        </li>
      </ul>
      <div v-if="total > pageSize" class="pager">
        <n-button size="small" :disabled="page <= 1" @click="page--; load()">上一页</n-button>
        <span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">下一页</n-button>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
}
.muted {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
}
.filters {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
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
.cap {
  padding: 8px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.55);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.list {
  list-style: none;
  padding: 0;
  margin: 0;
  font-size: 13px;
}
.list li {
  padding: 8px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.kind {
  color: #f9a8d4;
  margin-right: 8px;
}
.path {
  display: block;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.35);
}
.pager {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
  font-size: 13px;
}
</style>
