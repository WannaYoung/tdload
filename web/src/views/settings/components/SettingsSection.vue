<script setup lang="ts">
import type { Component } from "vue";
import { NIcon } from "naive-ui";

defineProps<{
  title: string;
  icon: Component;
  sectionClass?: string;
}>();

const sectionHeadColor = "rgba(255, 255, 255, 0.92)";
</script>

<template>
  <section class="settings-section" :class="sectionClass">
    <div class="section-head">
      <h3 class="section-title">
        <n-icon class="section-icon" :component="icon" :size="18" :color="sectionHeadColor" />
        {{ title }}
      </h3>
    </div>
    <slot />
  </section>
</template>

<style scoped>
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

@media (max-width: 640px) {
  .settings-section {
    padding: 14px 12px 4px;
  }
}
</style>
