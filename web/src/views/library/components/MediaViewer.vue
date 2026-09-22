<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { NButton, NIcon } from "naive-ui";
import { ChevronBackOutline, ChevronForwardOutline, CloseOutline } from "@vicons/ionicons5";
import type { LibraryItem } from "../../../api/types";

const props = defineProps<{
  open: boolean;
  item: LibraryItem | null;
  src: string;
  counter: string;
  index: number;
  total: number;
  imgReady: boolean;
}>();

const emit = defineEmits<{
  close: [];
  prev: [];
  next: [];
  "update:imgReady": [v: boolean];
  removeIndex: [];
  removeFile: [];
  touchStart: [e: TouchEvent];
  touchEnd: [e: TouchEvent];
}>();

const videoEl = ref<HTMLVideoElement | null>(null);

watch(
  () => [props.open, props.item?.id, props.item?.mediaKind] as const,
  async ([open, , kind]) => {
    if (!open || kind !== "video") return;
    await nextTick();
    videoEl.value?.load();
  },
);

defineExpose({ pauseVideo: () => { try { videoEl.value?.pause(); } catch { /* */ } } });
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open && item"
      class="viewer"
      role="dialog"
      aria-modal="true"
      @touchstart.passive="emit('touchStart', $event)"
      @touchend.passive="emit('touchEnd', $event)"
    >
      <header class="viewer-bar">
        <n-button quaternary @click="emit('close')">
          <template #icon>
            <n-icon :component="CloseOutline" />
          </template>
          关闭
        </n-button>
        <h1 class="viewer-title" :title="item.fileName">{{ item.fileName }}</h1>
        <div class="viewer-actions">
          <n-button size="small" quaternary @click="emit('removeIndex')">删索引</n-button>
          <n-button size="small" quaternary type="error" @click="emit('removeFile')">删文件</n-button>
          <span class="viewer-counter">{{ counter }}</span>
        </div>
      </header>

      <div class="viewer-body">
        <button
          class="viewer-nav prev"
          type="button"
          :disabled="index <= 0"
          aria-label="上一项"
          @click="emit('prev')"
        >
          <n-icon size="28" :component="ChevronBackOutline" />
        </button>

        <div class="viewer-stage">
          <img
            v-if="item.mediaKind === 'image'"
            :key="`img-${item.id}`"
            class="viewer-media"
            :class="{ ready: imgReady }"
            :src="src"
            :alt="item.fileName"
            @load="emit('update:imgReady', true)"
          />
          <video
            v-else-if="item.mediaKind === 'video'"
            :key="`vid-${item.id}`"
            ref="videoEl"
            class="viewer-media video ready"
            controls
            playsinline
            preload="metadata"
            :src="src"
          />
        </div>

        <button
          class="viewer-nav next"
          type="button"
          :disabled="index >= total - 1"
          aria-label="下一项"
          @click="emit('next')"
        >
          <n-icon size="28" :component="ChevronForwardOutline" />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<style>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  flex-direction: column;
  background: rgba(8, 8, 12, 0.96);
}
.viewer-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
  height: 52px;
  padding: 0 10px 0 6px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
.viewer-title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.viewer-counter {
  flex-shrink: 0;
  color: rgba(255, 255, 255, 0.5);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.viewer-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.viewer-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 48px 1fr 48px;
  touch-action: pan-y;
}
.viewer-stage {
  grid-column: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  min-height: 0;
  padding: 12px 0;
}
.viewer-media {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  opacity: 0.35;
  transition: opacity 0.16s ease;
  user-select: none;
  background: #000;
}
.viewer-media.ready {
  opacity: 1;
}
.viewer-media.video {
  width: min(100%, 1100px);
  max-height: calc(100vh - 80px);
  border-radius: 8px;
}
.viewer-nav {
  align-self: center;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 64px;
  border: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.78);
  cursor: pointer;
}
.viewer-nav.prev {
  grid-column: 1;
}
.viewer-nav.next {
  grid-column: 3;
}
.viewer-nav:disabled {
  opacity: 0.2;
  cursor: default;
}
.viewer-nav:not(:disabled):hover {
  color: #fff;
}
@media (max-width: 1000px) {
  .viewer-body {
    grid-template-columns: 36px 1fr 36px;
  }
}
</style>
