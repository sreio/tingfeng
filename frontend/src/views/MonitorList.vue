<script setup>
import { primaryUnit } from '../lib/model.js';
import { computed, inject, ref } from 'vue';
import Icon from '../components/Icon.vue';
import { api } from '../lib/api.js';
import {
  clone,
  destination,
  fmt,
  monitorStatus,
  ruleText,
  sourceInfo,
  statusLabels
} from '../lib/model.js';
const props = defineProps({ config: Object, states: Array });
const emit = defineEmits(['create', 'edit', 'detail']);
const saveConfig = inject('saveConfig'),
  notice = inject('notice'),
  refresh = inject('refreshStatus');
const search = ref(''),
  filter = ref('all'),
  busy = ref('');
const runtime = (m) => props.states.find((s) => s.monitorId === m.id);
const status = (m) => monitorStatus(m, runtime(m), props.config);
const attention = (m) => ['attention', 'review'].includes(status(m));
const recent = (m) => runtime(m)?.incident?.at > Date.now() - 7 * 86400000;
const active = computed(() => props.config.monitors.filter((m) => m.enabled && !m.draft).length);
const filtered = computed(() =>
  props.config.monitors.filter(
    (m) =>
      m.name.toLowerCase().includes(search.value.toLowerCase()) &&
      {
        all: true,
        active: m.enabled,
        attention: attention(m),
        incidents: recent(m),
        paused: !m.enabled && !m.draft,
        draft: m.draft
      }[filter.value]
  )
);
const tabs = computed(() => [
  { id: 'all', name: '全部监控' },
  { id: 'active', name: '运行中' },
  { id: 'paused', name: '已暂停' },
  { id: 'draft', name: '草稿' },
  ...(['attention', 'incidents'].includes(filter.value)
    ? [{ id: filter.value, name: filter.value === 'attention' ? '需要处理' : '最近异常' }]
    : [])
]);
async function check(m) {
  busy.value = m.id;
  try {
    const rt = await api('/monitors/' + m.id + '/check', { method: 'POST', body: {} });
    await refresh();
    notice(rt.fetch + ' · ' + rt.send);
  } catch (e) {
    notice(e.message);
  } finally {
    busy.value = '';
  }
}
async function toggle(m) {
  if (m.needsReview) {
    emit('edit', m);
    return;
  }
  busy.value = m.id;
  try {
    const next = clone(props.config);
    next.monitors.find((x) => x.id === m.id).enabled = !m.enabled;
    await saveConfig(next);
    notice(m.enabled ? '监控已暂停。' : '监控已启用。');
  } catch (e) {
    notice(e.message);
  } finally {
    busy.value = '';
  }
}
const problem = (m) =>
  m.needsReview
    ? '恢复的配置需要补充密钥并测试'
    : m.rules.some(
          (r) =>
            !destination(props.config, r) || destination(props.config, r) === '尚未选择通知方式'
        )
      ? '缺少可用通知方式'
      : runtime(m)?.incident?.message || '请检查通知配置';
</script>
<template>
  <main class="page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">保持关注，轻松一点</div>
        <h1>监控任务</h1>
        <p>把重复检查交给听风，只接收你关心的变化。</p>
      </div>
      <button class="btn primary" @click="$emit('create')">
        <Icon name="plus" :size="18" />新建监控
      </button>
    </div>
    <div class="stats-grid health-stats">
      <button class="stat-card" :class="{ chosen: filter === 'active' }" @click="filter = 'active'">
        <span class="stat-icon blue"><Icon name="monitor" /></span>
        <div>
          <span>运行中的监控</span><strong>{{ active }}<small>个</small></strong>
        </div></button
      ><button
        class="stat-card"
        :class="{ chosen: filter === 'attention' }"
        @click="filter = 'attention'"
      >
        <span class="stat-icon orange"><Icon name="bell" /></span>
        <div>
          <span>需要处理</span
          ><strong>{{ config.monitors.filter(attention).length }}<small>个监控</small></strong>
        </div></button
      ><button
        class="stat-card"
        :class="{ chosen: filter === 'incidents' }"
        @click="filter = 'incidents'"
      >
        <span class="stat-icon purple"><Icon name="clock" /></span>
        <div>
          <span>最近异常</span
          ><strong>{{ config.monitors.filter(recent).length }}<small>近 7 天</small></strong>
        </div>
      </button>
    </div>
    <div v-if="!config.monitors.length" class="start-banner">
      <span class="banner-icon"><Icon name="spark" :size="24" /></span>
      <div>
        <strong>第一个监控，三步就好</strong>
        <p>选数据 → 设规则 → 选接收人。可以先从一个常用接口开始。</p>
      </div>
      <button class="text-btn" @click="$emit('create')">
        开始配置<Icon name="arrow" :size="16" />
      </button>
    </div>
    <section class="task-section">
      <div class="list-toolbar">
        <div class="tabs compact">
          <button
            v-for="t in tabs"
            :key="t.id"
            :class="{ active: filter === t.id }"
            @click="filter = t.id"
          >
            {{ t.name }}
          </button>
        </div>
        <div class="search-box">
          <Icon name="search" :size="16" /><input
            v-model="search"
            aria-label="搜索监控"
            placeholder="搜索监控名称"
          />
        </div>
      </div>
      <div v-if="!filtered.length" class="panel empty">
        <Icon name="monitor" :size="34" />
        <h3>
          {{
            filter === 'attention'
              ? '没有需要处理的监控'
              : filter === 'incidents'
                ? '近 7 天没有异常'
                : '这里还没有监控'
          }}
        </h3>
        <p>{{ search ? '换个关键词试试。' : '用一个接口、网页或 RSS 订阅开始。' }}</p>
        <button class="btn" @click="filter === 'all' ? $emit('create') : (filter = 'all')">
          {{ filter === 'all' ? '新建监控' : '查看全部监控' }}
        </button>
      </div>
      <article v-for="m in filtered" :key="m.id" class="monitor-card">
        <div class="monitor-main">
          <span class="source-icon" :class="m.source.type"
            ><Icon :name="sourceInfo(m.source.type).icon" :size="23"
          /></span>
          <div class="monitor-identity">
            <div class="title-row">
              <button class="title-button" @click="$emit('detail', m)">{{ m.name }}</button
              ><span class="status" :class="status(m)"><i />{{ statusLabels[status(m)] }}</span>
            </div>
            <p>{{ sourceInfo(m.source.type).name }}<span>·</span>每 {{ m.interval }} 分钟检查</p>
          </div>
          <div class="monitor-value">
            <small>最新成功结果</small
            ><strong
              :class="{ numeric: typeof runtime(m)?.latest?.value === 'number' }"
              :title="runtime(m)?.latest?.display"
              >{{ runtime(m)?.latest?.display || '等待首次采集'
              }}<small>{{ primaryUnit(m.source) }}</small></strong
            >
          </div>
        </div>
        <button
          v-if="attention(m) || (filter === 'incidents' && recent(m))"
          class="incident-line"
          @click="$emit('detail', m, 'runs')"
        >
          <Icon name="info" :size="15" />{{
            runtime(m)?.incident?.resolved && !attention(m) ? '已恢复' : '待处理'
          }}
          · {{ problem(m) }}<span>查看原因 ›</span>
        </button>
        <div class="monitor-bottom">
          <div class="monitor-meta">
            <span
              ><Icon name="bell" :size="15" />{{ m.rules[0] ? ruleText(m.rules[0]) : '待配置提醒'
              }}{{ m.rules.length > 1 ? ' 等 ' + m.rules.length + ' 条提醒' : '' }}</span
            ><span class="last-check"
              ><Icon name="clock" :size="15" />检查
              {{ fmt(runtime(m)?.lastCheck, config.settings.timeZone) }} / 来源
              {{
                runtime(m)?.latest?.sourceAt
                  ? fmt(runtime(m).latest.sourceAt, config.settings.timeZone)
                  : '未提供时间'
              }}</span
            >
          </div>
          <div class="card-actions">
            <button v-if="m.draft" class="text-btn" @click="$emit('edit', m)">继续配置</button
            ><button
              v-else
              class="text-btn"
              :disabled="!!busy"
              @click="check(m)"
              title="检查结果可能触发已配置的提醒"
            >
              <Icon name="refresh" :size="14" :class="{ spinning: busy === m.id }" />{{
                busy === m.id ? '处理中…' : '立即检查'
              }}</button
            ><button class="icon-btn" :aria-label="'编辑' + m.name" @click="$emit('edit', m)">
              <Icon name="edit" :size="16" /></button
            ><button
              v-if="!m.draft"
              class="icon-btn"
              :aria-label="(m.enabled ? '暂停' : '启用') + m.name"
              :disabled="!!busy"
              @click="toggle(m)"
            >
              <Icon :name="m.enabled ? 'pause' : 'play'" :size="16" />
            </button>
          </div>
        </div>
      </article>
    </section>
    <div class="page-foot">
      <Icon name="shield" :size="14" />服务器自动运行 · 关闭浏览器后继续采集和通知
    </div>
  </main>
</template>
