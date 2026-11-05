<script setup>
import { inject, ref, watch } from 'vue';
import Dialog from './Dialog.vue';
import Icon from './Icon.vue';
import HistoryPanel from './HistoryPanel.vue';
import DeliveriesPanel from './DeliveriesPanel.vue';
import Pager from './Pager.vue';
import { api } from '../lib/api.js';
import {
  clone,
  destination,
  fmt,
  monitorStatus,
  ruleText,
  sourceInfo,
  stageNames,
  statusLabels,
  timed
} from '../lib/model.js';
const props = defineProps({ monitor: Object, state: Object, config: Object, initialTab: String });
const emit = defineEmits(['close', 'edit', 'copy', 'deleted']);
const tab = ref(props.initialTab || 'overview'),
  busy = ref(false),
  error = ref(''),
  stage = ref(''),
  logs = ref([]),
  total = ref(0),
  page = ref(1);
const notice = inject('notice'),
  confirm = inject('confirm'),
  saveConfig = inject('saveConfig'),
  refresh = inject('refreshStatus');
async function check() {
  busy.value = true;
  try {
    const r = await api('/monitors/' + props.monitor.id + '/check', { method: 'POST', body: {} });
    await refresh();
    notice(r.fetch + ' · ' + r.send);
    if (tab.value === 'runs') await loadLogs();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function remove() {
  if (
    !(await confirm(
      '删除监控？',
      '将删除「' +
        props.monitor.name +
        '」及其采集历史和过程日志。接收人和渠道会保留，尚未发送的消息将取消。'
    ))
  )
    return;
  busy.value = true;
  try {
    const c = clone(props.config);
    c.monitors = c.monitors.filter((m) => m.id !== props.monitor.id);
    await saveConfig(c);
    emit('deleted');
    notice('监控已删除。');
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function loadLogs(p = 1) {
  page.value = p;
  try {
    const data = await api(
      '/monitors/' + props.monitor.id + '/logs?page=' + p + '&stage=' + stage.value
    );
    logs.value = data.items;
    total.value = data.total;
  } catch (e) {
    error.value = e.message;
  }
}
watch(
  [tab, stage],
  () => {
    if (tab.value === 'runs') loadLogs();
  },
  { immediate: true }
);
</script>
<template>
  <Dialog open :title="monitor.name" wide :busy="busy" @close="$emit('close')"
    ><div class="tabs detail-tabs">
      <button
        v-for="t in [
          { id: 'overview', name: '概览' },
          { id: 'data', name: '数据' },
          { id: 'runs', name: '执行情况' },
          { id: 'deliveries', name: '发送记录' }
        ]"
        :key="t.id"
        :class="{ active: tab === t.id }"
        @click="tab = t.id"
      >
        {{ t.name }}
      </button>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <template v-if="tab === 'overview'"
      ><div class="detail-value">
        <span class="status" :class="monitorStatus(monitor, state, config)"
          ><i />{{ statusLabels[monitorStatus(monitor, state, config)] }}</span
        ><small>最新成功结果</small
        ><strong
          >{{ state?.latest?.display || '暂无采集结果'
          }}<small>{{ monitor.source.unit }}</small></strong
        >
      </div>
      <dl class="detail-list">
        <div>
          <dt>采集来源</dt>
          <dd>
            {{ sourceInfo(monitor.source.type).name }}<br /><span class="value-wrap">{{
              monitor.source.url
            }}</span>
          </dd>
        </div>
        <div>
          <dt>采集频率</dt>
          <dd>每 {{ monitor.interval }} 分钟</dd>
        </div>
        <div>
          <dt>最近检查</dt>
          <dd>{{ fmt(state?.lastCheck, config.settings.timeZone) }}</dd>
        </div>
        <div>
          <dt>最近成功采集</dt>
          <dd>{{ fmt(state?.lastSuccess, config.settings.timeZone) }}</dd>
        </div>
        <div>
          <dt>来源更新时间</dt>
          <dd>
            {{
              state?.latest?.sourceAt
                ? fmt(state.latest.sourceAt, config.settings.timeZone)
                : '来源没有提供'
            }}
          </dd>
        </div>
        <div>
          <dt>下次检查</dt>
          <dd>
            {{ monitor.enabled ? fmt(state?.nextCheck, config.settings.timeZone) : '已暂停' }}
          </dd>
        </div>
        <div v-for="(r, i) in monitor.rules" :key="r.id">
          <dt>提醒 {{ i + 1 }}</dt>
          <dd>
            {{ ruleText(r) }}<br /><small>发给 {{ destination(config, r) }}</small
            ><small v-if="timed(r) && monitor.enabled"
              ><br />预计下次：{{
                fmt(state?.rules?.[r.id]?.nextSend, config.settings.timeZone)
              }}</small
            >
          </dd>
        </div>
        <div>
          <dt>数据历史</dt>
          <dd>
            {{
              monitor.storage.history
                ? '保留 ' + monitor.storage.days + ' 天 / 最多 ' + monitor.storage.maxRows + ' 条'
                : '不新增历史'
            }}
          </dd>
        </div>
      </dl>
      <div v-if="state?.incident" class="incident-card">
        <strong>{{ state.incident.resolved ? '最近异常 · 已恢复' : '需要处理' }}</strong>
        <p>{{ state.incident.message }}</p>
      </div>
      <button v-if="!monitor.draft" class="btn" :disabled="busy" @click="check">
        <Icon name="refresh" :size="16" />{{ busy ? '正在检查…' : '立即检查' }}
      </button>
      <p class="fine">
        立即检查会按现有规则判断，可能发送通知。暂停的监控只检查，不发消息。
      </p></template
    >
    <HistoryPanel
      v-else-if="tab === 'data'"
      :monitor="monitor"
      :time-zone="config.settings.timeZone"
    />
    <template v-else-if="tab === 'runs'"
      ><div class="callout">
        <Icon name="info" :size="18" />
        <p>
          {{
            monitor.storage.logLevel === 'off'
              ? '过程日志已关闭，下方仍会显示最近执行状态。'
              : '过程日志按配置保留 ' + monitor.storage.logDays + ' 天，每项最多 5000 条。'
          }}
        </p>
      </div>
      <div class="execution">
        <div v-for="(label, k) in stageNames" :key="k">
          <Icon name="clock" :size="16" /><strong>{{ label }}</strong
          ><span>{{ state?.[k] || '尚未执行' }}</span>
        </div>
      </div>
      <div class="table-tools">
        <label
          >查看过程
          <select v-model="stage">
            <option value="">全部过程</option>
            <option v-for="(label, k) in stageNames" :key="k" :value="k">{{ label }}</option>
          </select></label
        ><button class="btn small" @click="loadLogs(page)">刷新日志</button>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>过程</th>
              <th>结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="l in logs" :key="l.id">
              <td>{{ fmt(l.at, config.settings.timeZone) }}</td>
              <td>{{ stageNames[l.stage] }}</td>
              <td :class="{ 'danger-text': l.level === 'error' }">{{ l.message }}</td>
            </tr>
            <tr v-if="!logs.length">
              <td colspan="3" class="empty-cell">没有符合条件的过程日志。</td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pager :total="total" :page="page" @change="loadLogs"
    /></template>
    <DeliveriesPanel v-else :monitor-id="monitor.id" :time-zone="config.settings.timeZone" />
    <template #footer
      ><button class="icon-btn" aria-label="删除监控" :disabled="busy" @click="remove">
        <Icon name="trash" :size="18" /></button
      ><button class="btn" :disabled="busy" @click="$emit('copy', monitor)">复制为草稿</button
      ><button class="btn primary" :disabled="busy" @click="$emit('edit', monitor)">
        <Icon name="edit" :size="16" />编辑监控
      </button></template
    >
  </Dialog>
</template>
