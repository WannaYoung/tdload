<script setup lang="ts">
import { NProgress } from "naive-ui";

defineProps<{
  percent: number;
  text: string;
}>();
</script>

<template>
  <div class="cov">
    <n-progress type="line" :percentage="percent" :show-indicator="false" style="width: 100%" />
    <div class="cov-text">{{ text }}</div>
  </div>
</template>

<style scoped>
.cov {
  width: 140px;
}
.cov-text {
  margin-top: 2px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.45);
  font-variant-numeric: tabular-nums;
}
</style>
