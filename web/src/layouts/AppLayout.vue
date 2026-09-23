<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NButton,
  NDropdown,
  NIcon,
  NLayout,
  NLayoutSider,
  NMenu,
  useDialog,
  type DropdownOption,
  type MenuOption,
} from "naive-ui";
import {
  CloudDownloadOutline,
  EyeOutline,
  FolderOpenOutline,
  GridOutline,
  HeartOutline,
  PeopleOutline,
  LogOutOutline,
  MenuOutline,
  PaperPlaneOutline,
  PersonCircleOutline,
  SettingsOutline,
} from "@vicons/ionicons5";
import { useAuthStore } from "../stores/auth";
import { useMobile } from "../composables/useMobile";
import { applyNoImageSetting } from "../composables/useNoImage";
import { api } from "../api/http";
import type { DashboardStats, Settings } from "../api/types";

const BADGE_STYLE =
  "display:inline-flex;align-items:center;justify-content:center;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:#f472b6;color:#500724;font-size:11px;font-weight:600;line-height:1;flex-shrink:0;";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const dialog = useDialog();
const isMobile = useMobile();
const menuOpen = ref(false);
const siderCollapsed = ref(false);
const messageActive = ref(0);
const savedActive = ref(0);
const channelActive = ref(0);

let taskCountTimer: number | null = null;

function icon(comp: typeof GridOutline) {
  return () => h(NIcon, null, { default: () => h(comp) });
}

function badgeText(n: number) {
  return n > 99 ? "99+" : String(n);
}

function renderBadge(n: number, extraStyle = "") {
  return h("span", { style: BADGE_STYLE + extraStyle }, badgeText(n));
}

function menuIcon(comp: typeof GridOutline, count: number) {
  const iconNode = () => h(NIcon, null, { default: () => h(comp) });
  if (!siderCollapsed.value || count <= 0) return iconNode;
  return () =>
    h(
      "span",
      { style: "position:relative;display:inline-flex;align-items:center;justify-content:center;" },
      [iconNode(), renderBadge(count, "position:absolute;top:-4px;right:-8px;")],
    );
}

function menuLabel(text: string, count: number) {
  if (count <= 0) return text;
  if (isMobile.value) {
    return () =>
      h("span", { style: "position:relative;display:block;width:100%;" }, [
        text,
        renderBadge(count, "position:absolute;right:0;top:50%;transform:translateY(-50%);"),
      ]);
  }
  return () =>
    h(
      "span",
      {
        style:
          "display:flex;align-items:center;justify-content:space-between;width:100%;gap:8px;",
      },
      [h("span", null, text), renderBadge(count)],
    );
}

const menuOptions = computed<MenuOption[]>(() => [
  { label: "仪表盘", key: "dashboard", icon: icon(GridOutline) },
  { label: "Telegram", key: "telegram", icon: icon(PaperPlaneOutline) },
  {
    label: menuLabel("收藏", savedActive.value),
    key: "saved",
    icon: menuIcon(HeartOutline, savedActive.value),
  },
  {
    label: menuLabel("频道", channelActive.value),
    key: "channels",
    icon: menuIcon(PeopleOutline, channelActive.value),
  },
  {
    label: menuLabel("任务", messageActive.value),
    key: "tasks",
    icon: menuIcon(CloudDownloadOutline, messageActive.value),
  },
  { label: "监听", key: "watch", icon: icon(EyeOutline) },
  { label: "资源库", key: "library", icon: icon(FolderOpenOutline) },
  { label: "设置", key: "settings", icon: icon(SettingsOutline) },
]);

const headerBadgeTotal = computed(
  () => messageActive.value + savedActive.value + channelActive.value,
);

const dropdownOptions = computed<DropdownOption[]>(() => [
  ...menuOptions.value,
  { type: "divider", key: "d1" },
  {
    label: () =>
      h(
        "span",
        { style: { color: "rgba(255, 255, 255, 0.45)" }, title: auth.user?.username || "已登录" },
        auth.user?.username || "已登录",
      ),
    key: "user",
    disabled: true,
    icon: icon(PersonCircleOutline),
  },
  {
    label: () => h("span", { style: { color: "#f9a8d4" } }, "退出"),
    key: "logout",
    icon: () => h(NIcon, { color: "#f472b6" }, { default: () => h(LogOutOutline) }),
  },
]);

const activeKey = computed(() => {
  const name = String(route.name || "dashboard");
  if (name === "channel-detail") return "channels";
  return name;
});

const contentClass = computed(() => {
  const embedMeta = route.meta.embedScroll;
  const embed = embedMeta === true || (embedMeta === "desktop" && !isMobile.value);
  return {
    "main-content": true,
    mobile: isMobile.value,
    bleed: Boolean(route.meta.fullBleed),
    embed,
  };
});

const keepAliveNames = ["LibraryView"];

async function refreshActiveTaskCount() {
  try {
    const s = await api<DashboardStats>("/api/dashboard");
    messageActive.value = s.tasksMessageActive ?? 0;
    savedActive.value = s.tasksSavedActive ?? 0;
    channelActive.value = s.tasksChannelActive ?? 0;
  } catch {
    /* keep */
  }
}

onMounted(() => {
  void auth.fetchMe();
  void refreshActiveTaskCount();
  void api<Settings>("/api/settings")
    .then((s) => applyNoImageSetting(!!s.noImage))
    .catch(() => undefined);
  taskCountTimer = window.setInterval(() => {
    void refreshActiveTaskCount();
  }, 5000);
});

onUnmounted(() => {
  if (taskCountTimer != null) {
    window.clearInterval(taskCountTimer);
    taskCountTimer = null;
  }
});

function onUpdateKey(key: string) {
  void router.push({ name: key });
}

function onDropdownSelect(key: string) {
  menuOpen.value = false;
  if (key === "logout") {
    confirmLogout();
    return;
  }
  if (key === "user") return;
  void router.push({ name: key });
}

function goHome() {
  void router.push({ name: "dashboard" });
}

function confirmLogout() {
  dialog.warning({
    title: "退出登录",
    content: "确定退出当前账号？",
    positiveText: "退出",
    negativeText: "取消",
    onPositiveClick: () => {
      auth.logout();
      void router.push({ name: "login" });
    },
  });
}
</script>

<template>
  <n-layout has-sider class="root-layout" :class="{ mobile: isMobile }">
    <n-layout-sider
      v-if="!isMobile"
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="220"
      :collapsed="siderCollapsed"
      show-trigger
      @collapse="siderCollapsed = true"
      @expand="siderCollapsed = false"
    >
      <button class="brand" type="button" @click="goHome">
        <n-icon size="22" :component="PaperPlaneOutline" />
        <span v-if="!siderCollapsed">TDLoad</span>
      </button>
      <n-menu
        :value="activeKey"
        :collapsed="siderCollapsed"
        :options="menuOptions"
        :collapsed-width="64"
        @update:value="onUpdateKey"
      />
    </n-layout-sider>

    <div class="main-pane">
      <header class="header" :class="{ mobile: isMobile }">
        <button class="header-left brand-btn" type="button" @click="goHome">
          <n-icon v-if="isMobile" size="20" :component="PaperPlaneOutline" />
          <span v-if="isMobile" class="brand-text">TDLoad</span>
          <span v-else class="muted">Telegram 批量下载控制台</span>
        </button>
        <div v-if="isMobile" class="header-right">
          <n-dropdown
            trigger="click"
            placement="bottom-end"
            :options="dropdownOptions"
            :show="menuOpen"
            @update:show="menuOpen = $event"
            @select="onDropdownSelect"
          >
            <span class="menu-btn-wrap">
              <n-button quaternary circle aria-label="菜单">
                <n-icon size="22" :component="MenuOutline" />
              </n-button>
              <span v-if="headerBadgeTotal > 0" class="header-task-badge">
                {{ headerBadgeTotal > 99 ? "99+" : headerBadgeTotal }}
              </span>
            </span>
          </n-dropdown>
        </div>
        <div v-else class="header-right">
          <span class="muted">{{ auth.user?.username }}</span>
          <n-button size="small" type="primary" @click="confirmLogout">
            <template #icon>
              <n-icon :component="LogOutOutline" />
            </template>
            退出
          </n-button>
        </div>
      </header>

      <main :class="contentClass">
        <router-view v-slot="{ Component }">
          <keep-alive :include="keepAliveNames">
            <component :is="Component" />
          </keep-alive>
        </router-view>
      </main>
    </div>
  </n-layout>
</template>

<style scoped>
.root-layout {
  height: 100vh;
  min-width: 0;
  overflow: hidden;
}
.root-layout > :deep(.n-layout-scroll-container) {
  overflow: hidden !important;
  height: 100%;
}

.main-pane {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #101014;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  height: 56px;
  padding: 0 20px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-weight: 650;
  letter-spacing: 0.04em;
  cursor: pointer;
  text-align: left;
}
.brand:hover {
  background: rgba(244, 114, 182, 0.08);
}
.brand .n-icon {
  color: #f472b6;
}

.header {
  flex: 0 0 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  z-index: 100;
  background: #18181c;
  border-bottom: 1px solid rgba(255, 255, 255, 0.09);
  box-sizing: border-box;
}
.header.mobile {
  padding: 0 10px 0 16px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  padding: 0;
}
.brand-btn .n-icon {
  color: #f472b6;
}
.brand-text {
  font-weight: 650;
  letter-spacing: 0.04em;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}
.muted {
  color: rgba(255, 255, 255, 0.45);
  font-size: 13px;
}
.menu-btn-wrap {
  position: relative;
  display: inline-flex;
}
.header-task-badge {
  position: absolute;
  top: 2px;
  right: 2px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: #f472b6;
  color: #500724;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  pointer-events: none;
}

.main-content {
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  overflow: auto;
  padding: 20px 28px 32px;
  box-sizing: border-box;
}
.main-content.mobile {
  padding: 12px 12px 20px;
}
.main-content.bleed {
  padding: 0;
}
.main-content.embed {
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 0;
}
.main-content.embed > :deep(*) {
  flex: 1 1 auto;
  min-height: 0;
}

:deep(.n-layout-sider) {
  background: #18181c !important;
}
:deep(.n-menu) {
  background: transparent;
}
</style>
