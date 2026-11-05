<script setup>
import { inject, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { api } from '../lib/api.js';
import { fmt } from '../lib/model.js';
import Pager from './Pager.vue';
import Dialog from './Dialog.vue';
import MessageCard from './MessageCard.vue';
const props = defineProps({ monitorId: String, timeZone: String });
const notice = inject('notice'),
  status = ref(''),
  page = ref(1),
  total = ref(0),
  rows = ref([]),
  busy = ref(false),
  error = ref(''),
  selected = ref(null);
const labels = {
  pending: '等待投递',
  sending: '正在发送',
  success: '已送达',
  failed: '失败',
  cancelled: '已取消'
};
let timer,
  version = 0;
async function load(p = page.value) {
  const req = ++version;
  busy.value = true;
  error.value = '';
  page.value = p;
  try {
    const data = await api(
      '/deliveries?monitorId=' +
        encodeURIComponent(props.monitorId || '') +
        '&status=' +
        status.value +
        '&page=' +
        p
    );
    if (req === version) {
      rows.value = data.items;
      total.value = data.total;
    }
  } catch (e) {
    error.value = e.message;
  } finally {
    if (req === version) busy.value = false;
  }
}
async function retry(d) {
  try {
    await api('/deliveries/' + d.id + '/retry', { method: 'POST', body: {} });
    notice('已重新加入发送队列。');
    await load();
  } catch (e) {
    notice(e.message);
  }
}
watch([() => props.monitorId, status], () => load(1), { immediate: true });
onMounted(() => {
  timer = setInterval(() => {
    if (!document.hidden) load();
  }, 10000);
});
onBeforeUnmount(() => {
  clearInterval(timer);
  version++;
});
</script>
<template>
  <div class="label-row table-tools">
    <label
      >投递状态
      <select v-model="status">
        <option value="">全部状态</option>
        <option v-for="(label, k) in labels" :key="k" :value="k">{{ label }}</option>
      </select></label
    ><button class="btn small" :disabled="busy" @click="load()">刷新</button>
  </div>
  <p v-if="error" class="error">{{ error }}</p>
  <div class="table-wrap">
    <table>
      <thead>
        <tr>
          <th>通知内容</th>
          <th>接收对象 / 渠道</th>
          <th>时间</th>
          <th>状态</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in rows" :key="d.id">
          <td>
            <button class="text-btn table-title" @click="selected = d">{{ d.payload.title }}</button
            ><small v-if="d.kind === 'test'" class="fine">手动测试</small>
          </td>
          <td>
            {{ d.recipient }}<small class="table-sub">{{ d.channel }}</small>
          </td>
          <td>{{ fmt(d.createdAt, timeZone) }}</td>
          <td>
            <span
              class="status"
              :class="
                d.status === 'success' ? 'normal' : d.status === 'failed' ? 'error' : 'paused'
              "
              ><i />{{ labels[d.status] }}</span
            ><small v-if="d.attempts" class="table-sub">尝试 {{ d.attempts }} 次</small>
          </td>
          <td>
            <button
              v-if="d.status === 'failed' && d.kind !== 'test'"
              class="text-btn"
              @click="retry(d)"
            >
              重试</button
            ><button v-else class="text-btn" @click="selected = d">详情</button>
          </td>
        </tr>
        <tr v-if="!rows.length">
          <td colspan="5" class="empty-cell">
            {{ busy ? '正在加载…' : '暂无投递记录。发送测试消息或规则触发后会显示在这里。' }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <Pager :page="page" :total="total" :busy="busy" @change="load" /><Dialog
    :open="!!selected"
    title="发送详情"
    @close="selected = null"
    ><template v-if="selected"
      ><MessageCard :item="selected.payload" />
      <dl class="detail-list">
        <div>
          <dt>接收对象</dt>
          <dd>{{ selected.recipient }} · {{ selected.channel }}</dd>
        </div>
        <div>
          <dt>投递结果</dt>
          <dd>{{ labels[selected.status] }}</dd>
        </div>
        <div>
          <dt>尝试次数</dt>
          <dd>{{ selected.attempts }}</dd>
        </div>
        <div v-if="selected.error">
          <dt>原因</dt>
          <dd>{{ selected.error }}</dd>
        </div>
        <div v-if="selected.status === 'pending'">
          <dt>预计下次尝试</dt>
          <dd>{{ fmt(selected.nextAttempt, timeZone) }}</dd>
        </div>
      </dl></template
    ></Dialog
  >
</template>
