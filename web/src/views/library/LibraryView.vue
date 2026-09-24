<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NEmpty, NIcon, NSelect, NSpin, useMessage } from "naive-ui";
import { PlayOutline, RefreshOutline, SyncOutline } from "@vicons/ionicons5";
import { api, ensureTicket, getToken } from "../../api/http";
import type { LibraryItem } from "../../api/types";
import { useNoImage, withImagePlaceholder } from "../../composables/useNoImage";
import MediaViewer from "./components/MediaViewer.vue";

defineOptions({ name: "LibraryView" });

const FILTER_KEY = "tdload.library.filters";

type SavedFilters = { chat?: string; mediaType?: string };

function readSavedFilters(): SavedFilters {
  try {
    const raw = localStorage.getItem(FILTER_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as SavedFilters;
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

function writeSavedFilters(chatVal: string, mediaVal: string) {
  try {
    localStorage.setItem(FILTER_KEY, JSON.stringify({ chat: chatVal, mediaType: mediaVal }));
  } catch {
    /* ignore quota / private mode */
  }
}

const saved = readSavedFilters();
const allowedMedia = new Set(["all", "image", "video"]);

const { t, locale } = useI18n();
const { noImage } = useNoImage();
const message = useMessage();
const loading = ref(false);
const syncing = ref(false);
const items = ref<LibraryItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const chat = ref<string>(typeof saved.chat === "string" && saved.chat ? saved.chat : "all");
const mediaType = ref<string>(
  typeof saved.mediaType === "string" && allowedMedia.has(saved.mediaType) ? saved.mediaType : "all",
);
const mediaToken = ref("");
const broken = ref<Record<number, boolean>>({});

const viewerOpen = ref(false);
const viewerIndex = ref(0);
const imgReady = ref(false);
const touchStartX = ref(0);
const viewerRef = ref<{ pauseVideo: () => void } | null>(null);

const filterOptions = ref<{ label: string; value: string }[]>([]);

const mediaOptions = computed(() => [
  { label: t("mediaKind.all"), value: "all" },
  { label: t("mediaKind.image"), value: "image" },
  { label: t("mediaKind.video"), value: "video" },
]);

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
  const tok = mediaToken.value || getToken();
  return tok ? `/api/library/${id}/file?token=${encodeURIComponent(tok)}` : "";
}

/** 列表缩略图 / 视频封面 */
function thumbUrl(id: number) {
  const tok = mediaToken.value || getToken();
  return tok ? `/api/library/${id}/thumb?token=${encodeURIComponent(tok)}` : "";
}

function thumbSrc(it: LibraryItem) {
  return withImagePlaceholder(thumbUrl(it.id));
}

/** 浏览页：始终用原文件 */
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
    const opts = [{ label: t("library.allChannels"), value: "all" }];
    for (const it of data.items || []) {
      if (it.key === "saved") {
        opts.push({ label: t("library.mySaved"), value: "saved" });
      } else {
        opts.push({ label: it.title || String(it.chatId), value: String(it.chatId) });
      }
    }
    filterOptions.value = opts;
    // 缓存的频道若不在选项中，回退到全部
    if (chat.value !== "all" && !opts.some((o) => o.value === chat.value)) {
      chat.value = "all";
      writeSavedFilters(chat.value, mediaType.value);
    }
  } catch {
    filterOptions.value = [
      { label: t("library.allChannels"), value: "all" },
      { label: t("library.mySaved"), value: "saved" },
    ];
  }
}

function onFilterChange() {
  page.value = 1;
  writeSavedFilters(chat.value, mediaType.value);
  void load();
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
    message.error(e instanceof Error ? e.message : t("common.loadFailed"));
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
      t("library.syncSuccess", {
        kept: data.kept,
        pruned: data.pruned,
        imported: data.imported,
      }),
    );
    await loadFilters();
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("library.syncFailed"));
  } finally {
    syncing.value = false;
  }
}

async function removeItem(it: LibraryItem, withFile: boolean) {
  try {
    const q = withFile ? "?delete_file=1" : "";
    await api(`/api/library/${it.id}${q}`, { method: "DELETE" });
    message.success(withFile ? t("library.deletedIndexAndFile") : t("library.deletedIndex"));
    closeViewer();
    await load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : t("common.deleteFailed"));
  }
}

function onThumbError(it: LibraryItem) {
  // 图片缩略图失败 → 标坏；视频封面失败 → 回退到 metadata 帧
  broken.value = { ...broken.value, [it.id]: true };
}

function videoFallbackSrc(it: LibraryItem) {
  const u = fileUrl(it.id);
  return u ? `${u}#t=0.1` : "";
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


watch(locale, () => void loadFilters());

onMounted(() => {
  window.addEventListener("keydown", onKey);
  filterOptions.value = [
    { label: t("library.allChannels"), value: "all" },
    { label: t("library.mySaved"), value: "saved" },
  ];
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
      <h2>{{ t("library.title") }}</h2>
      <div class="toolbar-actions">
        <n-button :loading="loading" @click="load">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          {{ t("common.refresh") }}
        </n-button>
        <n-button type="primary" :loading="syncing" @click="syncDisk">
          <template #icon>
            <n-icon :component="SyncOutline" />
          </template>
          {{ t("common.sync") }}
        </n-button>
      </div>
    </div>

    <div class="filters">
      <n-select
        v-model:value="chat"
        :options="filterOptions"
        class="filter-chat"
        @update:value="onFilterChange"
      />
      <n-select
        v-model:value="mediaType"
        :options="mediaOptions"
        class="filter-type"
        @update:value="onFilterChange"
      />
    </div>

    <div class="table-wrap">
      <n-spin :show="loading" class="spin-fill">
        <n-empty v-if="!items.length && !loading" :description="t('library.empty')" />
        <div v-else class="grid">
          <button
            v-for="it in items"
            :key="`${it.id}-${noImage ? 'n' : 'i'}`"
            type="button"
            class="card"
            @click="openViewer(it)"
          >
            <template v-if="it.mediaKind === 'image' && !broken[it.id]">
              <img
                class="thumb"
                loading="lazy"
                decoding="async"
                :src="thumbSrc(it)"
                :alt="it.fileName"
                @error="onThumbError(it)"
              />
            </template>
            <template v-else-if="it.mediaKind === 'video'">
              <div class="thumb video-thumb">
                <img
                  v-if="!broken[it.id]"
                  class="thumb-cover"
                  loading="lazy"
                  decoding="async"
                  :src="thumbSrc(it)"
                  :alt="it.fileName"
                  @error="onThumbError(it)"
                />
                <video
                  v-else
                  class="thumb-cover"
                  muted
                  playsinline
                  preload="metadata"
                  :src="videoFallbackSrc(it)"
                />
                <span class="play-badge" aria-hidden="true">
                  <n-icon size="28" :component="PlayOutline" />
                </span>
              </div>
            </template>
            <div v-else class="thumb placeholder">{{ t("library.noPreview") }}</div>
            <div class="cap" :title="it.fileName">{{ it.fileName }}</div>
          </button>
        </div>
      </n-spin>
    </div>

    <div v-if="total > pageSize" class="pager">
      <n-button size="small" :disabled="page <= 1" @click="page--; load()">{{ t("common.prevPage") }}</n-button>
      <span>{{ page }} / {{ Math.max(1, Math.ceil(total / pageSize)) }}</span>
      <n-button size="small" :disabled="page * pageSize >= total" @click="page++; load()">{{ t("common.nextPage") }}</n-button>
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
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
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
  width: 100%;
  aspect-ratio: 1;
  background: #121218;
  overflow: hidden;
}
.thumb-cover {
  width: 100%;
  height: 100%;
  display: block;
  object-fit: cover;
  background: #101014;
}
.play-badge {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.9);
  pointer-events: none;
  background: rgba(0, 0, 0, 0.18);
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
