<script setup lang="ts">
import { computed } from "vue";
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  type GlobalThemeOverrides,
} from "naive-ui";
import { naiveDateLocale, naiveLocale } from "./i18n";

/** 深色 + 粉色高亮 */
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

const locale = computed(() => naiveLocale.value);
const dateLocale = computed(() => naiveDateLocale.value);
</script>

<template>
  <n-config-provider
    :theme="darkTheme"
    :theme-overrides="themeOverrides"
    :locale="locale"
    :date-locale="dateLocale"
  >
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
