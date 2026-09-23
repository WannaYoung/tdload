import { createRouter, createWebHistory } from "vue-router";
import { getToken } from "../api/http";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/login",
      name: "login",
      component: () => import("../views/login/LoginView.vue"),
      meta: { public: true },
    },
    {
      path: "/",
      component: () => import("../layouts/AppLayout.vue"),
      children: [
        { path: "", name: "dashboard", component: () => import("../views/dashboard/DashboardView.vue") },
        { path: "telegram", name: "telegram", component: () => import("../views/telegram/TelegramView.vue") },
        {
          path: "saved",
          name: "saved",
          component: () => import("../views/saved/SavedView.vue"),
          meta: { embedScroll: "desktop" },
        },
        { path: "channels", name: "channels", component: () => import("../views/channels/ChannelsView.vue") },
        {
          path: "channels/:chatId",
          name: "channel-detail",
          component: () => import("../views/channels/ChannelDetailView.vue"),
        },
        {
          path: "tasks",
          name: "tasks",
          component: () => import("../views/tasks/TasksView.vue"),
          meta: { embedScroll: "desktop" },
          beforeEnter: (to) => {
            if (to.query.tab === "saved") return { name: "saved" };
            if (to.query.tab === "channel") return { name: "channels" };
            return true;
          },
        },
        {
          path: "library",
          name: "library",
          component: () => import("../views/library/LibraryView.vue"),
          meta: { embedScroll: "desktop" },
        },
        {
          path: "watch",
          name: "watch",
          component: () => import("../views/watch/WatchView.vue"),
          meta: { embedScroll: "desktop" },
        },
        { path: "settings", name: "settings", component: () => import("../views/settings/SettingsView.vue") },
      ],
    },
  ],
});

router.beforeEach((to) => {
  if (to.meta.public) return true;
  if (!getToken()) return { name: "login", query: { redirect: to.fullPath } };
  return true;
});

export default router;
