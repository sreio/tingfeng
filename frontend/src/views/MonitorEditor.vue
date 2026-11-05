<script setup>
import { computed, inject, onBeforeUnmount, ref, watch } from 'vue';
import Icon from '../components/Icon.vue';
import SourceEditor from '../components/SourceEditor.vue';
import RuleEditor from '../components/RuleEditor.vue';
import TargetPicker from '../components/TargetPicker.vue';
import PersonEditor from '../components/PersonEditor.vue';
import MessageCard from '../components/MessageCard.vue';
import { api } from '../lib/api.js';
import {
  clone,
  defaults,
  destination,
  estimate,
  newRule,
  ruleText,
  sourceInfo,
  stageNames,
  targets
} from '../lib/model.js';
const props = defineProps({ monitor: Object, config: Object });
const emit = defineEmits(['saved', 'cancel']);
const saveConfig = inject('saveConfig'),
  notice = inject('notice');
const m = ref(clone(props.monitor)),
  step = ref(1),
  busy = ref(false),
  error = ref(''),
  observation = ref(null),
  tested = ref(
    props.config.monitors.some((x) => x.id === props.monitor.id) && !props.monitor.needsReview
  );
const personOpen = ref(false),
  person = ref(null),
  personRule = ref(null),
  extraIndex = ref(-1),
  previewIndex = ref(0),
  message = ref(null),
  messageError = ref(''),
  sending = ref(false),
  sendResult = ref('');
const ruleErrors = ref({}),
  sourceError = ref('');
const primary = computed(() => m.value.rules[0]),
  previewRule = computed(() => m.value.rules[previewIndex.value] || primary.value);
const previewTargets = computed(() => targets(props.config, previewRule.value.selections));
const stepTitles = ['采集什么', '何时提醒', '发给谁'];
function sourceTest(p) {
  tested.value = true;
  observation.value = p.observation;
  error.value = '';
}
function setType(type) {
  const old = primary.value.selections;
  m.value.rules = [{ ...newRule(), trigger: type === 'rss' ? 'new' : 'change', selections: old }];
  previewIndex.value = 0;
}
function next() {
  error.value = '';
  if (step.value === 1) {
    if (!m.value.name.trim()) {
      error.value = '请先给监控起个名字。';
      return;
    }
    if (!tested.value) {
      error.value = '请先测试采集，确认字段和结果。';
      return;
    }
  }
  if (
    step.value === 2 &&
    (!Number.isInteger(m.value.interval) ||
      m.value.interval < 1 ||
      m.value.interval > 1440 ||
      ruleErrors.value[primary.value.id])
  ) {
    error.value = ruleErrors.value[primary.value.id] || '采集间隔需要为 1～1440 的整数分钟。';
    return;
  }
  step.value++;
  window.scrollTo({ top: 0, behavior: 'smooth' });
}
function openPerson(p, r) {
  person.value = p;
  personRule.value = r;
  personOpen.value = true;
}
function personSaved(p) {
  personRule.value.selections = personRule.value.selections.filter(
    (k) => !k.startsWith(p.id + ':')
  );
  personRule.value.selections.push(...defaults(p, props.config));
  personOpen.value = false;
}
function extra() {
  const r = { ...newRule(), trigger: 'periodic', selections: [...primary.value.selections] };
  m.value.rules.push(r);
  extraIndex.value = m.value.rules.length - 1;
  previewIndex.value = extraIndex.value;
}
function removeRule(i) {
  m.value.rules.splice(i, 1);
  previewIndex.value = 0;
  extraIndex.value = -1;
}
async function save(draft = false) {
  error.value = '';
  if (sourceError.value) {
    step.value = 1;
    error.value = sourceError.value;
    return;
  }
  if (!draft && !tested.value) {
    step.value = 1;
    error.value = '来源已更改，请重新测试采集。';
    return;
  }
  if (!draft) {
    for (const [i, r] of m.value.rules.entries()) {
      if (!targets(props.config, r.selections).length) {
        step.value = 3;
        extraIndex.value = i;
        error.value = '提醒 ' + (i + 1) + ' 需要一种可用的通知方式。';
        return;
      }
    }
  }
  busy.value = true;
  try {
    const monitor = clone(m.value);
    monitor.name = monitor.name.trim() || '未命名监控';
    monitor.draft = draft;
    monitor.enabled = !draft;
    monitor.needsReview = draft ? monitor.needsReview : false;
    const next = clone(props.config),
      i = next.monitors.findIndex((x) => x.id === monitor.id);
    if (i < 0) next.monitors.push(monitor);
    else next.monitors[i] = monitor;
    await saveConfig(next);
    emit('saved');
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
let timer, controller;
async function refreshMessage() {
  if (step.value !== 3) return;
  controller?.abort();
  controller = new AbortController();
  try {
    message.value = await api('/messages/preview', {
      method: 'POST',
      body: {
        name: m.value.name,
        source: m.value.source,
        rule: previewRule.value,
        observation: observation.value
      },
      signal: controller.signal
    });
    messageError.value = '';
  } catch (e) {
    if (e.name !== 'AbortError') messageError.value = e.message;
  }
}
watch(
  () => [step.value, JSON.stringify(previewRule.value), m.value.name, observation.value],
  () => {
    clearTimeout(timer);
    timer = setTimeout(refreshMessage, 200);
    sendResult.value = '';
  }
);
onBeforeUnmount(() => {
  clearTimeout(timer);
  controller?.abort();
});
async function testSend() {
  if (!message.value || !previewTargets.value.length) return;
  sending.value = true;
  sendResult.value = '';
  const list = [...previewTargets.value],
    text = message.value.text;
  const result = await Promise.allSettled(
    list.map((t) =>
      api('/channels/test', {
        method: 'POST',
        body: { channel: t.channel, address: t.method.address, text }
      })
    )
  );
  const success = result.filter((r) => r.status === 'fulfilled').length;
  sendResult.value =
    success +
    '/' +
    list.length +
    ' 个通知方式已确认接收。' +
    result
      .flatMap((r, i) =>
        r.status === 'rejected' ? [list[i].channel.name + '：' + r.reason.message] : []
      )
      .join('；');
  sending.value = false;
}
</script>
<template>
  <main class="page wizard-page">
    <div class="wizard-heading">
      <button class="text-btn muted" :disabled="busy" @click="$emit('cancel')">
        <Icon name="back" :size="17" />返回监控任务</button
      ><button class="text-btn" :disabled="busy" @click="save(true)">保存草稿</button>
    </div>
    <div class="page-heading">
      <div>
        <h1>{{ config.monitors.some((x) => x.id === m.id) ? '编辑监控' : '新建监控' }}</h1>
        <p>告诉听风你在关注什么，其余的交给规则。</p>
      </div>
      <span class="step-indicator">第 {{ step }} 步，共 3 步</span>
    </div>
    <ol class="stepper">
      <li
        v-for="(name, i) in stepTitles"
        :key="name"
        :class="{ current: step === i + 1, complete: step > i + 1 }"
      >
        <button
          :disabled="i + 1 > step"
          @click="
            step = i + 1;
            error = '';
          "
        >
          <span class="step-number"
            ><Icon v-if="step > i + 1" name="check" :size="16" /><template v-else>{{
              i + 1
            }}</template></span
          ><span
            >{{ name
            }}<small>{{
              ['选择来源，提取数据', '设置频率和提醒条件', '选择接收人，完成配置'][i]
            }}</small></span
          >
        </button>
      </li>
    </ol>
    <div class="wizard-grid">
      <section class="panel wizard-panel">
        <template v-if="step === 1"
          ><div class="section-title">
            <div>
              <h2>你想关注什么数据？</h2>
              <p>先选来源，测试一次，确认关心的字段。</p>
            </div>
          </div>
          <div class="field">
            <label for="monitor-name">监控名称</label
            ><input
              id="monitor-name"
              v-model="m.name"
              placeholder="例如 每日金价、产品更新"
              maxlength="100"
              required
            />
          </div>
          <SourceEditor
            :source="m.source"
            :monitor-id="m.copyFrom || m.id"
            @tested="sourceTest"
            @invalid="sourceError = $event"
            @invalidate="tested = false"
            @type="setType"
        /></template>
        <template v-if="step === 2"
          ><div class="section-title">
            <div>
              <h2>什么时候需要提醒你？</h2>
              <p>检查和发消息可以用不同的频率。</p>
            </div>
          </div>
          <div class="field">
            <label for="collect-interval">多久检查一次</label>
            <div class="input-unit">
              <input
                id="collect-interval"
                v-model.number="m.interval"
                type="number"
                min="1"
                max="1440"
              /><span>分钟</span>
            </div>
            <div class="pills">
              <button
                v-for="n in [1, 5, 15, 30, 60]"
                :key="n"
                :class="{ selected: m.interval === n }"
                @click="m.interval = n"
              >
                {{ n === 60 ? '1 小时' : n + ' 分钟' }}
              </button>
            </div>
          </div>
          <RuleEditor
            :rule="primary"
            :source="m.source"
            :interval="m.interval"
            @invalid="ruleErrors[primary.id] = $event"
        /></template>
        <template v-if="step === 3"
          ><div class="section-title">
            <div>
              <h2>把消息发给谁？</h2>
              <p>先使用默认通知方式，需要时再更改。</p>
            </div>
          </div>
          <div class="label-row rule-heading">
            <strong>提醒 1</strong><small>{{ ruleText(primary) }}</small>
          </div>
          <TargetPicker :config="config" :rule="primary" @edit="openPerson" />
          <div v-for="(r, i) in m.rules.slice(1)" :key="r.id" class="extra-rule">
            <div class="label-row">
              <strong>提醒 {{ i + 2 }}</strong
              ><button class="text-btn" @click="extraIndex = extraIndex === i + 1 ? -1 : i + 1">
                {{ extraIndex === i + 1 ? '完成编辑' : '编辑规则' }}</button
              ><button
                class="icon-btn"
                :aria-label="'移除提醒 ' + (i + 2)"
                @click="removeRule(i + 1)"
              >
                <Icon name="trash" :size="15" />
              </button>
            </div>
            <p class="fine">{{ ruleText(r) }}</p>
            <RuleEditor
              v-if="extraIndex === i + 1"
              :rule="r"
              :source="m.source"
              :interval="m.interval"
              @invalid="ruleErrors[r.id] = $event"
            /><TargetPicker :config="config" :rule="r" @edit="openPerson" />
          </div>
          <button v-if="m.rules.length < 24" class="text-btn add-rule" @click="extra">
            <Icon name="plus" :size="16" />再加一条提醒
          </button>
          <p class="fine route-hint">例如：价格越线发给微信，每小时把最新值发到邮箱。</p>
          <div class="notification-preview">
            <div class="label-row">
              <strong><Icon name="bell" :size="17" />通知预览</strong
              ><select
                v-if="m.rules.length > 1"
                v-model.number="previewIndex"
                aria-label="预览哪条提醒"
              >
                <option v-for="(r, i) in m.rules" :key="r.id" :value="i">
                  提醒 {{ i + 1 }} · {{ ruleText(r) }}
                </option></select
              ><small v-else>{{ previewTargets.length }} 个通知方式</small>
            </div>
            <MessageCard v-if="message" :item="message" preview />
            <p v-if="messageError" class="error">{{ messageError }}</p>
            <button
              class="btn"
              :disabled="sending || !previewTargets.length || !message || !!messageError"
              @click="testSend"
            >
              <Icon name="send" :size="15" />{{ sending ? '正在发送…' : '发送测试通知' }}
            </button>
            <p class="fine">会真实发送给当前选中的接收方式。</p>
            <p v-if="sendResult" class="callout" role="status">{{ sendResult }}</p>
          </div>
          <div class="storage-setting">
            <div class="toggle-row">
              <div>
                <strong>保存数据历史</strong>
                <p>之后可以查看数据表、趋势或导出 CSV。</p>
              </div>
              <button
                class="switch"
                :class="{ on: m.storage.history }"
                role="switch"
                :aria-checked="m.storage.history"
                aria-label="保存数据历史"
                @click="m.storage.history = !m.storage.history"
              >
                <span />
              </button>
            </div>
            <div v-if="m.storage.history" class="storage-inline">
              <label
                >保留
                <input
                  v-model.number="m.storage.days"
                  aria-label="历史保留天数"
                  type="number"
                  min="1"
                  max="365"
                />
                天</label
              ><span
                >预计最多 <b>{{ estimate(m).toLocaleString() }}</b> 条，已计入条数上限</span
              >
            </div>
            <p v-else class="fine">
              不新增历史；已保存数据按原保留周期清理，最新成功结果仍用于提醒。
            </p>
            <details class="advanced">
              <summary>
                存储与执行日志 <span>适合小型 VPS</span><Icon name="down" :size="16" />
              </summary>
              <div class="advanced-body">
                <div class="field">
                  <label
                    >每个监控最多保存记录数<input
                      v-model.number="m.storage.maxRows"
                      type="number"
                      min="1"
                      max="100000"
                  /></label>
                </div>
                <div class="field">
                  <label
                    >过程日志<select v-model="m.storage.logLevel">
                      <option value="off">关闭过程日志</option>
                      <option value="error">仅记录错误（推荐）</option>
                      <option value="summary">记录执行摘要</option>
                    </select></label
                  ><small>日志关闭后，最近执行状态、最新值和规则去重仍会保存。</small>
                </div>
                <div v-if="m.storage.logLevel !== 'off'">
                  <div class="stage-options">
                    <label v-for="(label, k) in stageNames" :key="k"
                      ><input type="checkbox" v-model="m.storage.stages[k]" />{{ label }}</label
                    >
                  </div>
                  <div class="field">
                    <label
                      >日志保留天数<input
                        v-model.number="m.storage.logDays"
                        type="number"
                        min="1"
                        max="30"
                    /></label>
                  </div>
                </div>
              </div>
            </details>
          </div>
        </template>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div class="wizard-footer">
          <button
            v-if="step > 1"
            class="btn"
            :disabled="busy"
            @click="
              step--;
              error = '';
            "
          >
            <Icon name="back" :size="15" />上一步</button
          ><span v-else /><button v-if="step < 3" class="btn primary" @click="next">
            下一步：{{ stepTitles[step] }}<Icon name="arrow" :size="16" /></button
          ><button v-else class="btn primary" :disabled="busy || sending" @click="save(false)">
            {{ busy ? '正在保存…' : '保存并启用' }}<Icon name="check" :size="16" />
          </button>
        </div>
      </section>
      <aside class="wizard-aside">
        <section class="panel summary-panel">
          <div class="eyebrow">随配置实时更新</div>
          <h3>你的监控会这样工作</h3>
          <div class="summary-item">
            <span class="summary-dot blue"><Icon name="globe" :size="16" /></span>
            <div>
              <small>采集</small><strong>{{ m.name || '等待命名的监控' }}</strong>
              <p>
                {{ sourceInfo(m.source.type).name }} ·
                {{ m.source.path || m.source.selector || '完整结果' }}
              </p>
            </div>
          </div>
          <div class="summary-item">
            <span class="summary-dot purple"><Icon name="clock" :size="16" /></span>
            <div>
              <small>检查与提醒</small><strong>每 {{ m.interval || '…' }} 分钟检查</strong>
              <p v-for="r in m.rules" :key="r.id">{{ ruleText(r) }}</p>
            </div>
          </div>
          <div class="summary-item">
            <span class="summary-dot green"><Icon name="people" :size="16" /></span>
            <div>
              <small>发送给</small><strong>{{ destination(config, primary) }}</strong>
            </div>
          </div>
          <div class="summary-storage">
            <Icon name="data" :size="16" /><span
              >{{ m.storage.history ? '历史保留 ' + m.storage.days + ' 天' : '不新增历史'
              }}<small>{{
                m.storage.logLevel === 'off'
                  ? '过程日志关闭'
                  : '日志保留 ' + m.storage.logDays + ' 天'
              }}</small></span
            >
          </div>
        </section>
        <p class="aside-tip">
          <Icon name="info" :size="16" />保存后由服务器自动运行，关闭浏览器也会继续采集和提醒。
        </p>
      </aside>
    </div>
    <PersonEditor
      v-if="personOpen"
      :person="person"
      :config="config"
      @close="personOpen = false"
      @saved="personSaved"
    />
  </main>
</template>
