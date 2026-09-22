<script setup lang="ts">
import { reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NButton, NCard, NForm, NFormItem, NInput, useMessage } from "naive-ui";
import { useAuthStore } from "../../stores/auth";

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();
const message = useMessage();
const loading = ref(false);
const form = reactive({
  username: "",
  password: "",
});

async function submit() {
  loading.value = true;
  try {
    await auth.login(form.username, form.password);
    const redirect = typeof route.query.redirect === "string" ? route.query.redirect : "/";
    await router.replace(redirect);
  } catch (err) {
    message.error(err instanceof Error ? err.message : "登录失败");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="wrap">
    <div class="glow glow-a" />
    <div class="glow glow-b" />
    <n-card class="card" :bordered="false">
      <header class="brand">
        <h1>TDLoad</h1>
        <p>Telegram 批量下载控制台</p>
      </header>
      <n-form :model="form" @submit.prevent="submit">
        <n-form-item label="用户名">
          <n-input
            v-model:value="form.username"
            placeholder="用户名"
            size="large"
            :disabled="loading"
          />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="form.password"
            type="password"
            size="large"
            show-password-on="click"
            placeholder="密码"
            :disabled="loading"
            @keyup.enter="submit"
          />
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="submit">
          登录
        </n-button>
      </n-form>
    </n-card>
  </div>
</template>

<style scoped>
.wrap {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  box-sizing: border-box;
  overflow: hidden;
  background:
    radial-gradient(1200px 600px at 50% -10%, rgba(244, 114, 182, 0.18), transparent 55%),
    #101014;
}
.glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.glow-a {
  width: 360px;
  height: 360px;
  top: 12%;
  left: 18%;
  background: rgba(244, 114, 182, 0.22);
}
.glow-b {
  width: 280px;
  height: 280px;
  right: 16%;
  bottom: 18%;
  background: rgba(219, 39, 119, 0.14);
}
.card {
  position: relative;
  width: min(400px, 100%);
  padding: 12px 8px 8px;
  background: rgba(24, 24, 28, 0.88);
  border: 1px solid rgba(244, 114, 182, 0.12);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(16px);
}
.brand {
  margin-bottom: 28px;
  text-align: center;
}
.brand h1 {
  margin: 0;
  font-size: 30px;
  font-weight: 700;
  letter-spacing: 0.06em;
  line-height: 1.1;
  color: #f9a8d4;
}
.brand p {
  margin: 10px 0 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.45);
}
</style>
