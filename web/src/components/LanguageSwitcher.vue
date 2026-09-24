<script setup lang="ts">
import { computed } from "vue";
import { NButton, NIcon } from "naive-ui";
import { LanguageOutline } from "@vicons/ionicons5";
import { useI18n } from "vue-i18n";
import { currentLocale, toggleLocale } from "../i18n";

const { t } = useI18n();

const title = computed(() =>
  currentLocale.value === "zh-CN" ? t("layout.switchToEn") : t("layout.switchToZh"),
);
</script>

<template>
  <n-button quaternary circle :title="title" :aria-label="title" @click="toggleLocale">
    <n-icon size="20" :component="LanguageOutline" />
  </n-button>
</template>
