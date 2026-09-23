<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { NButton, NEmpty, NIcon, NSelect, NSpin, useMessage } from "naive-ui";
import { PlayOutline } from "@vicons/ionicons5";
import { api, ensureTicket, getToken } from "../../api/http";
import type { LibraryItem } from "../../api/types";
import { useNoImage, withImagePlaceholder } from "../../composables/useNoImage";
import MediaViewer from "./components/MediaViewer.vue";

defineOptions({ name: "LibraryView" });

const { noImage } = useNoImage();
const message = useMessage();
const loading = ref(false);
const syncing = ref(false);
const items = ref<LibraryItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const chat = ref<string>("all");
const mediaType = ref<string>("all");
const mediaToken = ref("");
const broken = ref<Record<number, boolean>>({});

const viewerOpen = ref(false);
const viewerIndex = ref(0);
const imgReady = ref(false);
const touchStartX = ref(0);
const viewerRef = ref<{ pauseVideo: () => void } | null>(null);

const filterOptions = ref<{ label: string; value: string }[]>([
  { label: "全部频道", value: "all" },
  { label: "我的收藏", value: "saved" },
]);

const mediaOptions = [
  { label: "全部", value: "all" },
  { label: "图片", value: "image" },
  { label: "视频", value: "video" },
];

const browseable = computed(() =>
  items.value.filter((it) => {
    if (it.mediaKind === "video") return true;
    if (it.mediaKind === "image" && !broken.value[it.id]) return true;
    return false;
  }),
);

const current = computed(() => browseable.value[viewerIndex.value] || null);

const counter = computed(() => {
  if (!viewerOpen.value || !browseable.value.length) return "";
  return `${viewerIndex.value + 1} / ${browseable.value.length}`;
});

function fileUrl(id: number) {
  const t = mediaToken.value || getToken();
  return t ? `/api/library/${id}/file?token=${encodeURIComponent(t)}` : "";
}

function thumbSrc(it: LibraryItem) {
  return withImagePlaceholder(fileUrl(it.id));
}

function currentSrc() {
  if (!current.value) return "";
  if (current.value.mediaKind === "image") return withImagePlaceholder(fileUrl(current.value.id));
  return fileUrl(current.value.id);
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
  closeViewer();
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

async function syncDisk() {
  syncing.value = true;
  try {
    const data = await api<{ kept: number; pruned: number; imported: number; cursorsUpdated: number }>(
      "/api/library/sync",
      {
        method: "POST",
        body: "{}",
      },
    );
    message.success(
      `同步完成：保留 ${data.kept}，清理 ${data.pruned}，导入 ${data.imported}`,
    );
    await loadFilters();
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "扫盘失败");
  } finally {
    syncing.value = false;
  }
}

async function removeItem(it: LibraryItem, withFile: boolean) {
  try {
    const q = withFile ? "?delete_file=1" : "";
    await api(`/api/library/${it.id}${q}`, { method: "DELETE" });
    message.success(withFile ? "已删除索引与文件" : "已删除索引");
    closeViewer();
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : "删除失败");
  }
}

function onImgError(id: number) {
  broken.value = { ...broken.value, [id]: true };
}

function pauseVideo() {
  viewerRef.value?.pauseVideo();
}

function openViewer(it: LibraryItem) {
  const idx = browseable.value.findIndex((x) => x.id === it.id);
  if (idx < 0) return;
  viewerIndex.value = idx;
  imgReady.value = false;
  viewerOpen.value = true;
  document.body.style.overflow = "hidden";
}

function closeViewer() {
  pauseVideo();
  viewerOpen.value = false;
  document.body.style.overflow = "";
}

function go(delta: number) {
  const next = viewerIndex.value + delta;
  if (next < 0 || next >= browseable.value.length) return;
  pauseVideo();
  viewerIndex.value = next;
  imgReady.value = false;
}

function onTouchStart(e: TouchEvent) {
  touchStartX.value = e.changedTouches[0]?.clientX ?? 0;
}

function onTouchEnd(e: TouchEvent) {
  if (!viewerOpen.value) return;
  const dx = (e.changedTouches[0]?.clientX ?? 0) - touchStartX.value;
  if (dx > 50) go(-1);
  else if (dx < -50) go(1);
}

function onKey(e: KeyboardEvent) {
  if (!viewerOpen.value) return;
  const tag = (e.target as HTMLElement | null)?.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
  if (e.key === "Escape") {
    closeViewer();
    return;
  }
  if (e.key === "ArrowLeft") go(-1);
  if (e.key === "ArrowRight") go(1);
}


onMounted(() => {
  window.addEventListener("keydown", onKey);
  void loadFilters();
  void load();
});

onUnmounted(() => {
  window.removeEventListener("keydown", onKey);
  document.body.style.overflow = "";
});
</script>

<template>
  <div class="page list-page pinned">
    <div class="toolbar">
      <h2>资源库</h2>
      <div class="toolbar-actions">
        <n-button quaternary :loading="syncing" @click="syncDisk">扫盘同步</n-button>
        <n-button quaternary :loading="loading" @click="load">刷新</n-button>
      </div>
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
          <button
            v-for="it in items"
            :key="`${it.id}-${noImage ? 'n' : 'i'}`"
            type="button"
            class="card"
            @click="openViewer(it)"
          >
            <template v-if="it.mediaKind === 'image' && !broken[it.id]">
              <img class="thumb" :src="thumbSrc(it)" :alt="it.fileName" @error="onImgError(it.id)" />
            </template>
            <template v-else-if="it.mediaKind === 'video'">
              <div class="thumb video-thumb">
                <span class="play-badge" aria-hidden="true">
                  <n-icon size="28" :component="PlayOutline" />
                </span>
              </div>
            </template>
            <div v-else class="thumb placeholder">无预览</div>
            <div class="cap" :title="it.fileName">{{ it.fileName }}</div>
          </button>
        </div>
      </n-spin>
    </div>

    <div v-if="total > pageSize" class="pager">
      <n-button size="small" :disabled="page <= 1" @click="page--; load()">上一页</n-button>
      <span>{{ page }} / {{ Math.max(1, Math.ceil(total / pageSize)) }}</span>
      <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">下一页</n-button>
    </div>

    <MediaViewer
      ref="viewerRef"
      :open="viewerOpen"
      :item="current"
      :src="currentSrc()"
      :counter="counter"
      :index="viewerIndex"
      :total="browseable.length"
      v-model:img-ready="imgReady"
      @close="closeViewer"
      @prev="go(-1)"
      @next="go(1)"
      @remove-index="current && removeItem(current, false)"
      @remove-file="current && removeItem(current, true)"
      @touch-start="onTouchStart"
      @touch-end="onTouchEnd"
    />
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
  font-size: 22px;
  flex-shrink: 0;
  white-space: nowrap;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: nowrap;
}
.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  /* 覆盖全局窄屏 width:100%，避免标题被挤成竖排 */
  width: auto;
  margin-left: auto;
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
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 10px;
  padding-bottom: 8px;
}
.card {
  display: block;
  width: 100%;
  padding: 0;
  margin: 0;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: pointer;
  background: #18181c;
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.06);
}
.card:hover {
  border-color: rgba(244, 114, 182, 0.45);
}
.thumb {
  width: 100%;
  aspect-ratio: 1;
  display: block;
  object-fit: cover;
  background: #101014;
}
.video-thumb {
  position: relative;
  background: #121218;
}
.play-badge {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.85);
  pointer-events: none;
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
@media (max-width: 1000px) {
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
    gap: 8px;
  }
  .cap {
    padding: 6px;
    font-size: 10px;
  }
}
</style>

