<script setup lang="ts">
import {
  darkTheme,
  dateZhCN,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  zhCN,
  type GlobalThemeOverrides,
} from "naive-ui";

/** 深色 + 粉色高亮（对齐 xtools 布局，主色改为粉） */
const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#f472b6",
    primaryColorHover: "#f9a8d4",
    primaryColorPressed: "#db2777",
    primaryColorSuppl: "#f472b6",
    infoColor: "#f472b6",
    infoColorHover: "#f9a8d4",
    infoColorPressed: "#db2777",
    infoColorSuppl: "#f472b6",
    successColor: "#f472b6",
    successColorHover: "#f9a8d4",
    successColorPressed: "#db2777",
    successColorSuppl: "#f472b6",
  },
  Button: {
    textColorPrimary: "#500724",
    textColorHoverPrimary: "#500724",
    textColorPressedPrimary: "#500724",
    textColorFocusPrimary: "#500724",
  },
  Menu: {
    itemTextColorActive: "#f9a8d4",
    itemTextColorActiveHover: "#fbcfe8",
    itemIconColorActive: "#f472b6",
    itemIconColorActiveHover: "#f9a8d4",
    itemTextColorChildActive: "#f9a8d4",
    itemIconColorChildActive: "#f472b6",
  },
};
</script>

<template>
  <n-config-provider
    :theme="darkTheme"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
