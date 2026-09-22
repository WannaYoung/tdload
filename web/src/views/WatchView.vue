<script setup lang="ts">
import { NAlert, NEmpty } from "naive-ui";
</script>

<template>
  <div class="page">
    <header class="head">
      <h2>监听</h2>
      <p>频道 / 群持续监听，新媒体按规则自动入队（二期 M5）。</p>
    </header>
    <n-alert type="warning" style="margin-bottom: 16px">
      一期先完成批量下载；本页为占位，里程碑 M5 实现对话列表、过滤规则与水位。
    </n-alert>
    <n-empty description="暂无监听规则" />
  </div>
</template>

<style scoped>
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
</style>
