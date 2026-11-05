<script setup>
import { ref } from 'vue';
import Icon from '../components/Icon.vue';
import PasswordInput from '../components/PasswordInput.vue';
import { api } from '../lib/api.js';
const props = defineProps({ initialized: Boolean });
const emit = defineEmits(['authenticated']);
const password = ref(''),
  again = ref(''),
  token = ref(''),
  busy = ref(false),
  error = ref(''),
  help = ref(false);
async function submit() {
  error.value = '';
  if (!props.initialized && password.value !== again.value) {
    error.value = '两次输入的密码不一致。';
    return;
  }
  busy.value = true;
  try {
    const session = await api(props.initialized ? '/auth/login' : '/auth/setup', {
      method: 'POST',
      body: props.initialized
        ? { password: password.value }
        : { password: password.value, token: token.value.trim() }
    });
    password.value = '';
    again.value = '';
    token.value = '';
    emit('authenticated', session);
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="access-page">
    <header class="access-header">
      <span class="brand"
        ><span class="brand-mark"><Icon name="wind" :size="27" /></span>听风
        <small>MONITOR</small></span
      ><span class="local-status">你的私人监控工作空间</span>
    </header>
    <main class="access-layout">
      <section class="access-story">
        <span class="access-eyebrow">轻量部署，简单使用</span>
        <h1>关心变化，<br />少些打扰。</h1>
        <p>接口、网页、RSS，一个地方管理。<br />让每一条通知，都值得被看见。</p>
        <div class="access-flow">
          <div><Icon name="globe" /><strong>关注数据</strong><small>选择你关心的来源</small></div>
          <span class="access-connector" />
          <div>
            <Icon name="clock" /><strong>设置规则</strong><small>变化提醒，或每小时告诉你</small>
          </div>
          <span class="access-connector" />
          <div>
            <Icon name="bell" /><strong>送达身边</strong><small>一个人，多种通知方式</small>
          </div>
        </div>
        <p class="access-story-note">
          <Icon name="shield" :size="14" />一个管理员，无需注册。接收人不需要后台账号。
        </p>
      </section>
      <section class="panel access-card">
        <div class="access-lock"><Icon name="lock" :size="22" /></div>
        <div class="eyebrow">{{ initialized ? '欢迎回来' : '初次见面，欢迎使用听风' }}</div>
        <h2>{{ initialized ? '登录工作空间' : '设置管理员密码' }}</h2>
        <p class="access-intro">
          {{
            initialized
              ? '输入密码，看看你关心的变化。'
              : '首次使用需要服务器上的初始化令牌，确保只有你能设置管理员。'
          }}
        </p>
        <form @submit.prevent="submit">
          <div v-if="!initialized" class="field">
            <label for="setup-token">初始化令牌</label
            ><input
              id="setup-token"
              v-model="token"
              type="password"
              autocomplete="off"
              maxlength="256"
              required
            /><small>在服务器数据目录的 setup.token 文件中获取。</small>
            <details class="fine">
              <summary>Docker 部署如何获取？</summary>
              <code>make setup-token</code>
              <p>初始化完成后该令牌立即失效。密码和令牌不会保存在浏览器中。</p>
            </details>
          </div>
          <PasswordInput
            v-model="password"
            :label="initialized ? '管理员密码' : '设置密码'"
            :autocomplete="initialized ? 'current-password' : 'new-password'"
          /><PasswordInput
            v-if="!initialized"
            v-model="again"
            label="确认密码"
            autocomplete="new-password"
          />
          <p v-if="error" class="error" role="alert">{{ error }}</p>
          <button class="btn primary full-width" :disabled="busy">
            {{ busy ? '正在验证…' : initialized ? '登录' : '保存并进入后台'
            }}<Icon name="arrow" :size="16" />
          </button>
        </form>
        <button v-if="initialized" class="text-btn access-help-btn" @click="help = !help">
          忘记密码
        </button>
        <div v-if="help" class="callout">
          <p>
            在服务器执行
            <code>docker compose exec tingfeng /tingfeng reset-password --confirm</code
            >，然后刷新页面，用新的初始化令牌设置密码。监控配置会保留，已有登录会失效。
          </p>
        </div>
      </section>
    </main>
    <footer class="access-page-footer">Golang + Vue · SQLite · 数据保存在你自己的服务器</footer>
  </div>
</template>
