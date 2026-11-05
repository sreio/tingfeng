<script setup>
import { computed, nextTick, ref } from 'vue';
import { messageTemplate } from '../lib/model.js';
const props = defineProps({ rule: Object, source: Object });
const custom = ref(Boolean(props.rule.template));
const input = ref(null);
let draft = props.rule.template || '';
const common = [
  ['消息标题', '{{.Title}}', '监控名称加提醒原因。', '金价监控 · 数据发生变化'],
  ['监控名称', '{{.Monitor}}', '你给这条监控起的名字。', '金价监控'],
  ['当前值', '{{.Value}}', '换算后的判断值；未指定判断字段时使用第一个字段。', '904.41'],
  ['对比值', '{{.Previous}}', '变化或阈值提醒的上次判断值；定时提醒使用最新默认值。', '900.00'],
  ['提醒规则', '{{.Rule}}', '本条消息对应的提醒条件。', '数据发生变化'],
  [
    '采集时间',
    '{{.CollectedTime}}',
    '听风成功取到这份数据的时间，不是消息发送时间。',
    '2026-11-05 09:00:00'
  ],
  [
    '来源更新时间',
    '{{.SourceTime}}',
    '数据源自己标记的更新时间；未提供时显示“来源未提供”。',
    '2026-11-05 08:59:00'
  ],
  [
    '来源链接',
    '{{.SourceURL}}',
    '采集地址，已去除查询参数等敏感信息。',
    'https://example.com/prices'
  ]
];
const variables = computed(() =>
  (props.source.fields || []).flatMap((f) => [
    [f.label, '{{.Fields.' + f.key + '}}', '这个字段换算后的值，不包含单位。'],
    [
      f.label + '单位',
      '{{.Units.' + f.key + '}}',
      '你为这个字段填写的显示单位。',
      f.unit || '未设置，发送时为空'
    ]
  ])
);
function mode(enabled) {
  if (custom.value === enabled) return;
  if (!enabled) draft = props.rule.template;
  custom.value = enabled;
  props.rule.template = enabled ? draft || messageTemplate(props.source) : '';
}
async function insert(variable) {
  const start = input.value?.selectionStart ?? props.rule.template.length;
  const end = input.value?.selectionEnd ?? start;
  const next = props.rule.template.slice(0, start) + variable + props.rule.template.slice(end);
  if (next.length > 4096) return;
  props.rule.template = next;
  await nextTick();
  input.value?.focus();
  input.value?.setSelectionRange(start + variable.length, start + variable.length);
}
</script>

<template>
  <section class="template-editor" aria-label="消息内容与变量">
    <div class="label-row"><strong>消息内容</strong><small>每条提醒可单独设置</small></div>
    <div class="template-modes" role="group" aria-label="消息内容方式">
      <button
        type="button"
        :class="{ selected: !custom }"
        :aria-pressed="!custom"
        @click="mode(false)"
      >
        默认消息
      </button>
      <button
        type="button"
        :class="{ selected: custom }"
        :aria-pressed="custom"
        @click="mode(true)"
      >
        自定义模板
      </button>
    </div>
    <p v-if="!custom" class="fine">
      自动展示全部提取字段、单位、采集时间与来源。下方就是实际消息预览。
    </p>
    <template v-else>
      <label class="field"
        >消息模板
        <textarea
          ref="input"
          v-model="rule.template"
          rows="7"
          maxlength="4096"
          placeholder="编写消息，点击下方变量插入"
          spellcheck="false"
        />
      </label>
      <p class="fine">
        点击下方按钮插入光标位置；选中文字时会替换选中内容。示例用于解释含义，发送内容以下方预览为准。
      </p>
      <div v-if="variables.length" class="variable-chips">
        <button
          v-for="[label, value, description, example] in variables"
          :key="value"
          type="button"
          :aria-label="'插入' + label + '变量'"
          @click="insert(value)"
        >
          <strong>{{ label }}<span aria-hidden="true">＋ 插入</span></strong>
          <small>{{ description }}</small>
          <code v-text="value" />
          <small v-if="example" class="variable-example">示例：{{ example }}</small>
        </button>
      </div>
      <div class="variable-chips">
        <button
          v-for="[label, value, description, example] in common"
          :key="value"
          type="button"
          :aria-label="'插入' + label + '变量'"
          @click="insert(value)"
        >
          <strong>{{ label }}<span aria-hidden="true">＋ 插入</span></strong>
          <small>{{ description }}</small>
          <code v-text="value" />
          <small class="variable-example">示例：{{ example }}</small>
        </button>
      </div>
      <p v-if="!rule.template" class="fine">模板为空时使用默认消息。</p>
    </template>
  </section>
</template>

<style scoped>
.template-editor {
  margin: 22px 0 18px;
}
.template-modes {
  display: flex;
  gap: 8px;
  margin: 14px 0;
}
.template-modes button,
.variable-chips button {
  border: 1px solid #dce3ef;
  border-radius: 8px;
  background: #fff;
  padding: 9px 12px;
  color: #405470;
  cursor: pointer;
}
.template-modes button.selected {
  border-color: #5875d7;
  color: #3554b9;
  background: #eef3ff;
}
.template-modes button:hover,
.variable-chips button:hover {
  background: #eef3ff;
}
.variable-chips {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin: 12px 0;
}
.variable-chips button {
  display: flex;
  flex-direction: column;
  text-align: left;
  gap: 6px;
  max-width: 100%;
  font-size: 12px;
}
.variable-chips strong {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  font-size: 13px;
}
.variable-chips strong span {
  color: #3554b9;
  font-size: 11px;
  white-space: nowrap;
}
.variable-chips small {
  font-size: 12px;
  line-height: 1.5;
}
.variable-example {
  color: #566983;
  overflow-wrap: anywhere;
}
.variable-chips code {
  font-size: 11px;
  overflow-wrap: anywhere;
}
textarea {
  font-family: inherit;
}
@media (max-width: 600px) {
  .variable-chips {
    grid-template-columns: 1fr;
  }
}
</style>
