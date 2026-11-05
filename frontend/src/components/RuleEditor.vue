<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import Icon from './Icon.vue';
import { api } from '../lib/api.js';
import { timed, fmt } from '../lib/model.js';
const props = defineProps({ rule: Object, source: Object, interval: Number, anchor: Number });
const emit = defineEmits(['invalid']);
const preview = ref(null),
  error = ref(''),
  trial = ref(false),
  sample = ref('129, 131, 132, 128, 131'),
  pending = ref(false);
let timer, controller;
const choices = computed(() => [
  ['change', '数据有变化', '与上一次成功结果不同才提醒'],
  ...(props.source.type !== 'rss'
    ? [['threshold', '达到指定数值', '向上或向下越过阈值时提醒一次']]
    : [['new', '发现新条目', '首次记住已有条目，之后只发新内容']]),
  ['periodic', '每隔一段时间发送', '例如每 1 小时发一次，没变化也发送'],
  ['scheduled', '每天定时通知', '每天固定时间发送最新采集结果']
]);
const values = computed(() =>
  sample.value
    .split(/[,，→\n]/)
    .map((v) => v.trim())
    .filter(Boolean)
    .map((v) => (v !== '' && Number.isFinite(Number(v)) ? Number(v) : v))
);
async function refresh() {
  controller?.abort();
  controller = new AbortController();
  pending.value = true;
  try {
    const result = await api('/rules/preview', {
      method: 'POST',
      body: {
        rule: props.rule,
        source: props.source,
        values: values.value,
        interval: Number(props.interval) || 0,
        anchor: props.anchor || 0
      },
      signal: controller.signal
    });
    preview.value = result;
    error.value = '';
    emit('invalid', '');
  } catch (e) {
    if (e.name !== 'AbortError') {
      error.value = e.message;
      emit('invalid', e.message);
    }
  } finally {
    pending.value = false;
  }
}
watch(
  () => [
    JSON.stringify(props.rule),
    JSON.stringify(props.source.fields),
    props.interval,
    sample.value
  ],
  () => {
    clearTimeout(timer);
    timer = setTimeout(refresh, 250);
  },
  { immediate: true }
);
watch(
  () => props.rule.trigger,
  (t) => {
    sample.value = t === 'new' ? '已有条目, 公告 A, 公告 A, 公告 B' : '129, 131, 132, 128, 131';
  }
);
onBeforeUnmount(() => {
  clearTimeout(timer);
  controller?.abort();
});
</script>
<template>
  <div class="rule-editor">
    <fieldset class="field choice-field">
      <legend>什么情况发通知</legend>
      <div class="rule-options">
        <label v-for="c in choices" :key="c[0]" :class="{ selected: rule.trigger === c[0] }"
          ><input type="radio" v-model="rule.trigger" :value="c[0]" />
          <div>
            <strong>{{ c[1] }}</strong
            ><small>{{ c[2] }}</small>
          </div></label
        >
      </div>
    </fieldset>
    <template v-if="rule.trigger === 'periodic'"
      ><div class="schedule-box">
        <span class="eyebrow"><Icon name="clock" :size="15" />发送频率</span>
        <div class="frequency-line">
          <span>每隔</span
          ><input
            type="number"
            v-model.number="rule.every"
            min="1"
            step="1"
            aria-label="发送间隔数值"
          /><select v-model="rule.unit" aria-label="发送间隔单位">
            <option value="minute">分钟</option>
            <option value="hour">小时</option>
            <option value="day">天</option></select
          ><strong>发一次</strong>
        </div>
        <div class="pills">
          <button
            v-for="p in [
              [30, 'minute', '每 30 分钟'],
              [1, 'hour', '每 1 小时'],
              [2, 'hour', '每 2 小时']
            ]"
            type="button"
            :key="p[2]"
            :class="{ selected: rule.every === p[0] && rule.unit === p[1] }"
            @click="
              rule.every = p[0];
              rule.unit = p[1];
            "
          >
            {{ p[2] }}
          </button>
        </div>
        <div class="window-row">
          <label
            >发送时段<select v-model="rule.timing" aria-label="发送时段">
              <option value="always">全天</option>
              <option value="window">每天指定时段</option>
            </select></label
          >
          <div v-if="rule.timing === 'window'" class="clock-pair">
            <input v-model="rule.windowStart" type="time" aria-label="时段开始" /><span>至</span
            ><input v-model="rule.windowEnd" type="time" aria-label="时段结束" />
          </div>
        </div>
        <p class="fine">
          {{
            rule.timing === 'window'
              ? '从时段开始时发送，结束时间停止；支持跨午夜。'
              : '启用后等待一个间隔，首次发送；重启不会重置计时。'
          }}
        </p>
      </div></template
    >
    <div v-if="rule.trigger === 'scheduled'" class="field">
      <label>每天发送时间<input v-model="rule.time" type="time" /></label>
    </div>
    <div v-if="timed(rule)" class="upcoming">
      <div class="label-row">
        <strong><Icon name="clock" :size="16" />接下来 3 次发送</strong
        ><small>{{ preview?.timeZone || '按系统时区' }}</small>
      </div>
      <div class="time-chips">
        <time v-for="t in preview?.next || []" :key="t">{{ fmt(t, preview.timeZone) }}</time>
      </div>
      <p>
        每 {{ interval || '…' }} 分钟采集，到点发送最近一次成功结果；没有结果时跳过。{{
          pending ? '正在更新时间…' : ''
        }}
      </p>
    </div>
    <template v-else>
      <label v-if="source.fields?.length && rule.trigger !== 'new'" class="field">
        判断哪个字段
        <select v-model="rule.field" aria-label="判断字段">
          <option value="">
            {{
              rule.trigger === 'change' ? '任一字段有变化' : '第一个字段：' + source.fields[0].label
            }}
          </option>
          <option v-for="f in source.fields" :key="f.key" :value="f.key">
            {{ f.label }}{{ f.unit ? '（' + f.unit + '）' : '' }}
          </option>
          <option
            v-if="rule.field && !source.fields.some((f) => f.key === rule.field)"
            :value="rule.field"
          >
            字段已删除，请重新选择
          </option>
        </select>
      </label>
      <div v-if="rule.trigger === 'threshold'" class="field">
        <label>提醒阈值</label>
        <div class="frequency-line">
          <select v-model="rule.operator" aria-label="阈值方向">
            <option value="gt">向上越过</option>
            <option value="lt">向下越过</option></select
          ><input v-model.number="rule.threshold" type="number" step="any" aria-label="阈值" />
        </div>
        <small
          >使用所选字段换算后的数值判断，单位与字段一致。持续高于或低于阈值不会重复提醒。</small
        >
      </div>
      <p v-if="rule.trigger === 'new'" class="info-line">
        第一次只记住已有条目，不会把历史文章一次发给你。
      </p>
      <details class="advanced">
        <summary>
          减少重复打扰 <span>{{ rule.cooldown ? rule.cooldown + ' 分钟冷却' : '不冷却' }}</span
          ><Icon name="down" :size="16" />
        </summary>
        <div class="advanced-body field">
          <label
            >两次提醒至少间隔（分钟）<input
              type="number"
              v-model.number="rule.cooldown"
              min="0"
              max="1440" /></label
          ><small>冷却期间继续采集，跳过提醒，不会积压补发。</small>
        </div>
      </details>
      <button class="btn" type="button" :aria-expanded="trial" @click="trial = !trial">
        <Icon name="play" :size="16" />{{ trial ? '收起规则试算' : '用示例数据测试规则' }}
      </button>
      <div v-if="trial" class="simulation">
        <label
          >连续采集的示例值<input
            v-model="sample"
            aria-label="连续采集的示例值"
            maxlength="1000" /></label
        ><small>用逗号分隔，每个值相隔 {{ interval }} 分钟。试算不会发送消息。</small
        ><strong class="simulation-title"
          >这 {{ preview?.rows?.length || 0 }} 次采集会提醒
          {{ preview?.rows?.filter((x) => x.send).length || 0 }} 次</strong
        >
        <ol>
          <li v-for="r in preview?.rows || []" :key="r.index">
            <small>+{{ r.minute }} 分</small><b>{{ r.value }}</b
            ><span :class="{ 'sent-mark': r.send }"
              >{{ r.send ? '发送 · ' : '跳过 · ' }}{{ r.reason }}</span
            >
          </li>
        </ol>
      </div>
    </template>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </div>
</template>
