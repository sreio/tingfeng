<script setup>
import { computed, onBeforeUnmount, onMounted, provide, reactive, ref } from 'vue';
import Icon from './components/Icon.vue';
import Dialog from './components/Dialog.vue';
import AuthView from './views/AuthView.vue';
import MonitorList from './views/MonitorList.vue';
import MonitorEditor from './views/MonitorEditor.vue';
import PeopleView from './views/PeopleView.vue';
import SettingsView from './views/SettingsView.vue';
import RecordsView from './views/RecordsView.vue';
import MonitorDetail from './components/MonitorDetail.vue';
import { api, configureAuth } from './lib/api.js';
import { clone, newMonitor, uid } from './lib/model.js';
const auth = reactive({
  ready: false,
  initialized: true,
  authenticated: false,
  csrf: '',
  passwordUpdatedAt: 0
});
const config = ref(null),
  states = ref([]),
  system = ref(null),
  loading = ref(false),
  loadError = ref(''),
  mobile = ref(false);
const view = ref(
  ['monitors', 'people', 'records', 'settings'].includes(location.hash.slice(1))
    ? location.hash.slice(1)
    : 'monitors'
);
const editor = ref(null),
  detail = ref(null),
  detailTab = ref('overview');
const toast = ref('');
let toastTimer, poll;
const titles = {
  monitors: '监控任务',
  people: '通知管理',
  records: '数据记录',
  settings: '系统设置'
};
const confirmState = reactive({ open: false, title: '', text: '', resolve: null });
function notice(s) {
  toast.value = s;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (toast.value = ''), 5000);
}
function confirm(title, text) {
  return new Promise((resolve) =>
    Object.assign(confirmState, { open: true, title, text, resolve })
  );
}
function answer(v) {
  confirmState.open = false;
  confirmState.resolve?.(v);
}
function expired() {
  Object.assign(auth, { authenticated: false, csrf: '' });
  config.value = null;
  editor.value = null;
  detail.value = null;
  notice('登录已过期，请重新登录。');
}
configureAuth('', expired);
async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    config.value = await api('/config');
    await refresh();
  } catch (e) {
    loadError.value = e.message;
  } finally {
    loading.value = false;
  }
}
async function refresh() {
  const results = await Promise.allSettled([api('/monitors/status'), api('/system')]);
  if (results[0].status === 'fulfilled') states.value = results[0].value;
  if (results[1].status === 'fulfilled') system.value = results[1].value;
}
async function authenticated(session) {
  Object.assign(auth, session, { authenticated: true, initialized: true });
  configureAuth(session.csrf, expired);
  await load();
}
async function save(next) {
  const saved = await api('/config', { method: 'PUT', body: next });
  config.value = saved;
  await refresh();
  return saved;
}
async function navigate(next) {
  if (
    editor.value &&
    !(await confirm('离开编辑？', '尚未保存的修改会丢失，也可以取消后保存为草稿。'))
  )
    return;
  editor.value = null;
  detail.value = null;
  view.value = next;
  mobile.value = false;
  history.replaceState(null, '', '#' + next);
  window.scrollTo({ top: 0, behavior: 'instant' });
}
function create() {
  editor.value = newMonitor(config.value);
  view.value = 'monitors';
  mobile.value = false;
  window.scrollTo({ top: 0, behavior: 'instant' });
}
function edit(m, copy = false) {
  let value = clone(m);
  if (copy) {
    value.copyFrom = m.id;
    value.id = uid('monitor');
    value.name += ' · 副本';
    value.generation = 0;
    value.enabled = false;
    value.draft = true;
    value.rules = value.rules.map((r) => ({ ...r, id: uid('rule') }));
  }
  editor.value = value;
  detail.value = null;
  view.value = 'monitors';
  window.scrollTo({ top: 0, behavior: 'instant' });
}
function openDetail(m, tab = 'overview') {
  detail.value = m.id;
  detailTab.value = tab;
}
function savedMonitor() {
  editor.value = null;
  notice('监控已保存，调度会按配置自动运行。');
}
async function logout() {
  if (editor.value && !(await confirm('退出登录？', '未保存的监控修改会丢失。'))) return;
  try {
    await api('/auth/logout', { method: 'POST', body: {} });
    expired();
    toast.value = '';
  } catch (e) {
    notice(e.message);
  }
}
async function initialized() {
  try {
    const status = await api('/auth/status');
    Object.assign(auth, status);
    configureAuth(status.csrf, expired);
    if (status.authenticated) await load();
  } catch (e) {
    loadError.value = e.message;
  } finally {
    auth.ready = true;
  }
}
provide('notice', notice);
provide('confirm', confirm);
provide('saveConfig', save);
provide('refreshStatus', refresh);
onMounted(() => {
  initialized();
  poll = setInterval(() => {
    if (auth.authenticated && !document.hidden) refresh();
  }, 10000);
});
onBeforeUnmount(() => {
  clearInterval(poll);
  clearTimeout(toastTimer);
});
const detailMonitor = computed(() => config.value?.monitors.find((m) => m.id === detail.value));
const detailState = computed(() => states.value.find((r) => r.monitorId === detail.value));
</script>
<template>
  <div v-if="!auth.ready" class="startup">
    <Icon name="wind" :size="36" />
    <p>正在打开工作空间…</p>
  </div>
  <div v-else-if="!auth.authenticated && loadError" class="startup">
    <h2>暂时无法连接听风</h2>
    <p class="error">{{ loadError }}</p>
    <button
      class="btn"
      @click="
        loadError = '';
        initialized();
      "
    >
      重新连接
    </button>
  </div>
  <AuthView
    v-else-if="!auth.authenticated"
    :initialized="auth.initialized"
    @authenticated="authenticated"
  />
  <div v-else class="app-shell">
    <button v-if="mobile" class="nav-backdrop" aria-label="关闭导航" @click="mobile = false" />
    <aside class="sidebar" :class="{ 'is-open': mobile }">
      <button class="brand" @click="navigate('monitors')">
        <span class="brand-mark"><Icon name="wind" :size="27" /></span
        ><span>听风<span class="brand-en">MONITOR</span></span>
      </button>
      <div class="workspace">
        <span class="workspace-avatar">我的</span>
        <div><strong>我的工作空间</strong><span>个人使用 · 自托管</span></div>
      </div>
      <div class="nav-label">工作台</div>
      <nav aria-label="主导航">
        <button
          v-for="item in [
            { id: 'monitors', name: '监控任务', icon: 'monitor' },
            { id: 'records', name: '数据记录', icon: 'data' },
            { id: 'people', name: '通知管理', icon: 'bell' }
          ]"
          :key="item.id"
          :class="{ active: view === item.id }"
          @click="navigate(item.id)"
        >
          <Icon :name="item.icon" /><span>{{ item.name }}</span
          ><span v-if="item.id === 'monitors'" class="nav-count">{{
            config?.monitors.length || 0
          }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="quiet-card">
          <span class="quiet-icon"><Icon name="wind" :size="22" /></span
          ><strong>关心变化，少些打扰。</strong>
          <p>让每一条通知<br />都值得被看见。</p>
          <span class="quiet-line" />
        </div>
        <button :class="{ active: view === 'settings' }" @click="navigate('settings')">
          <Icon name="settings" /><span>系统设置</span>
        </button>
        <div class="profile">
          <span class="avatar">我</span>
          <div><strong>管理员</strong><small>私人工作空间</small></div>
          <button class="icon-btn" aria-label="退出登录" @click="logout">
            <Icon name="logout" :size="18" />
          </button>
        </div>
      </div>
    </aside>
    <div class="main-shell">
      <header class="topbar">
        <div class="breadcrumb">
          <button class="icon-btn mobile-menu" aria-label="打开导航" @click="mobile = !mobile">
            <Icon name="grid" /></button
          ><span>工作台</span><Icon name="chevron" :size="13" /><strong>{{ titles[view] }}</strong
          ><template v-if="editor"
            ><Icon name="chevron" :size="13" /><span>配置监控</span></template
          >
        </div>
        <div class="top-actions">
          <span class="local-status"><span class="online-dot" />服务已连接</span
          ><span class="tag">数据留在你的服务器</span>
        </div>
      </header>
      <div v-if="system?.storagePressure" class="storage-warning">
        <Icon
          name="info"
          :size="16"
        />存储接近上限，已暂停积累历史和过程日志；采集与提醒继续运行。<button
          class="text-btn"
          @click="navigate('settings')"
        >
          处理存储
        </button>
      </div>
      <main v-if="!config" class="page">
        <p>{{ loadError || '正在载入配置…' }}</p>
        <button v-if="loadError" class="btn" @click="load">重试</button>
      </main>
      <MonitorEditor
        v-else-if="editor"
        :key="editor.id"
        :monitor="editor"
        :config="config"
        @saved="savedMonitor"
        @cancel="navigate('monitors')"
      />
      <MonitorList
        v-else-if="view === 'monitors'"
        :config="config"
        :states="states"
        @create="create"
        @edit="edit"
        @detail="openDetail"
      />
      <PeopleView v-else-if="view === 'people'" :config="config" />
      <RecordsView v-else-if="view === 'records'" :config="config" @edit="edit" />
      <SettingsView
        v-else-if="view === 'settings'"
        :config="config"
        :system="system"
        :auth="auth"
        @password-changed="expired"
      />
    </div>
  </div>
  <MonitorDetail
    v-if="detailMonitor"
    :key="detailMonitor.id"
    :monitor="detailMonitor"
    :state="detailState"
    :config="config"
    :initial-tab="detailTab"
    @close="detail = null"
    @edit="edit"
    @copy="(m) => edit(m, true)"
    @deleted="detail = null"
  />
  <Dialog :open="confirmState.open" :title="confirmState.title" @close="answer(false)"
    ><p>{{ confirmState.text }}</p>
    <template #footer
      ><button class="btn" @click="answer(false)">取消</button
      ><button class="btn primary" @click="answer(true)">确认</button></template
    ></Dialog
  >
  <Transition name="toast"
    ><div v-if="toast" class="toast" role="status">
      <Icon name="info" :size="17" />{{ toast
      }}<button class="icon-btn" aria-label="关闭提示" @click="toast = ''">
        <Icon name="close" :size="14" />
      </button></div
  ></Transition>
</template>
