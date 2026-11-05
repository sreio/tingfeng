<script setup>
import { computed, inject, ref, watch } from 'vue';
import Icon from './Icon.vue';
import Pager from './Pager.vue';
import { api, download } from '../lib/api.js';
import { fmt } from '../lib/model.js';
const props = defineProps({ monitor: Object, timeZone: String });
const notice = inject('notice'),
  confirm = inject('confirm');
const rows = ref([]),
  total = ref(0),
  page = ref(1),
  busy = ref(false),
  error = ref('');
let request = 0;
async function load(p = 1) {
  const id = ++request;
  page.value = p;
  busy.value = true;
  error.value = '';
  try {
    const data = await api('/monitors/' + props.monitor.id + '/records?page=' + p);
    if (id === request) {
      rows.value = data.items;
      total.value = data.total;
    }
  } catch (e) {
    error.value = e.message;
  } finally {
    if (id === request) busy.value = false;
  }
}
watch(
  () => props.monitor.id,
  () => load(),
  { immediate: true }
);
const numbers = computed(() =>
  rows.value
    .map((r) =>
      typeof r.value === 'number'
        ? r.value
        : typeof r.value === 'string' && r.value.trim() !== ''
          ? Number(r.value)
          : NaN
    )
    .filter(Number.isFinite)
    .reverse()
);
const points = computed(() => {
  const n = numbers.value,
    min = Math.min(...n),
    range = Math.max(...n) - min;
  return n
    .map(
      (v, i) => 5 + (290 * i) / (n.length - 1) + ',' + (range ? 65 - (57 * (v - min)) / range : 35)
    )
    .join(' ');
});
async function exportCSV() {
  try {
    await download(
      '/monitors/' + props.monitor.id + '/records.csv',
      '听风-' + props.monitor.name + '.csv'
    );
  } catch (e) {
    notice(e.message);
  }
}
async function clear() {
  if (
    !(await confirm(
      '清空历史记录？',
      '仅删除「' + props.monitor.name + '」的采集历史。监控、最新值与通知规则会保留。'
    ))
  )
    return;
  try {
    await api('/monitors/' + props.monitor.id + '/records', { method: 'DELETE', body: {} });
    await load();
    notice('历史记录已清空。');
  } catch (e) {
    notice(e.message);
  }
}
</script>
<template>
  <div class="data-heading">
    <div>
      <h2>{{ monitor.name }}</h2>
      <p>
        {{
          monitor.storage.history
            ? '历史保留 ' + monitor.storage.days + ' 天 / 最多 ' + monitor.storage.maxRows + ' 条'
            : '当前不新增历史，已保存的数据到期清理'
        }}
      </p>
    </div>
    <button class="btn small" :disabled="!total || busy" @click="exportCSV">
      <Icon name="data" :size="15" />导出 CSV
    </button>
  </div>
  <p v-if="error" class="error">
    {{ error }} <button class="text-btn" @click="load(page)">重试</button>
  </p>
  <figure v-if="numbers.length > 1" class="price-trend">
    <figcaption>
      本页数据趋势 <span>{{ numbers[0] }} → {{ numbers.at(-1) }} {{ monitor.source.unit }}</span>
    </figcaption>
    <svg
      viewBox="0 0 300 73"
      preserveAspectRatio="none"
      role="img"
      aria-label="本页采集数据的数值走势"
    >
      <path d="M0 67H300 M0 37H300 M0 7H300" stroke="#e0e7f3" stroke-width=".6" />
      <polyline
        :points="points"
        fill="none"
        stroke="#506cdb"
        stroke-width="2.5"
        vector-effect="non-scaling-stroke"
      />
    </svg>
  </figure>
  <div class="table-wrap">
    <table>
      <thead>
        <tr>
          <th>采集时间</th>
          <th>采集值 {{ monitor.source.unit }}</th>
          <th>来源更新时间</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.id">
          <td>{{ fmt(r.at, timeZone) }}</td>
          <td class="value-cell">
            {{ r.display }}
            <details v-if="r.value && typeof r.value === 'object'" class="record-detail">
              <summary>查看提取数据</summary>
              <pre>{{ JSON.stringify(r.value, null, 2) }}</pre>
            </details>
          </td>
          <td>{{ r.sourceAt ? fmt(r.sourceAt, timeZone) : '来源未提供' }}</td>
        </tr>
        <tr v-if="!rows.length">
          <td colspan="3" class="empty-cell">
            {{
              busy ? '正在加载…' : '还没有历史记录。开启保存历史后，下一次成功采集会显示在这里。'
            }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <Pager :total="total" :page="page" :busy="busy" @change="load" /><button
    v-if="total"
    class="text-btn danger-text"
    @click="clear"
  >
    清空此监控的历史
  </button>
</template>
