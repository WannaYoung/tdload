import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { api, getToken, setToken, refreshTickets, clearTickets } from "../api/http";
import type { LoginData, User } from "../api/types";

export const useAuthStore = defineStore("auth", () => {
  const token = ref<string | null>(getToken());
  const user = ref<User | null>(null);

  const isAuthed = computed(() => Boolean(token.value));

  async function login(username: string, password: string) {
    const data = await api<LoginData>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    token.value = data.token;
    user.value = data.user;
    setToken(data.token);
    await refreshTickets().catch(() => {});
  }

  async function fetchMe() {
    if (!token.value) return;
    user.value = await api<User>("/api/auth/me");
    await refreshTickets().catch(() => {});
  }

  function logout() {
    token.value = null;
    user.value = null;
    setToken(null);
    clearTickets();
  }

  return { token, user, isAuthed, login, fetchMe, logout };
});
