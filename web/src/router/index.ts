import { createRouter, createWebHistory } from "vue-router";
import { getToken } from "../api/http";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/login",
      name: "login",
      component: () => import("../views/LoginView.vue"),
      meta: { public: true },
    },
    {
      path: "/",
      component: () => import("../layouts/AppLayout.vue"),
      children: [
        { path: "", name: "dashboard", component: () => import("../views/DashboardView.vue") },
        { path: "telegram", name: "telegram", component: () => import("../views/TelegramView.vue") },
        { path: "channels", name: "channels", component: () => import("../views/ChannelsView.vue") },
        {
          path: "tasks",
          name: "tasks",
          component: () => import("../views/TasksView.vue"),
          meta: { embedScroll: "desktop" },
        },
        {
          path: "library",
          name: "library",
          component: () => import("../views/LibraryView.vue"),
          meta: { embedScroll: "desktop" },
        },
        { path: "watch", name: "watch", component: () => import("../views/WatchView.vue") },
        { path: "settings", name: "settings", component: () => import("../views/SettingsView.vue") },
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
