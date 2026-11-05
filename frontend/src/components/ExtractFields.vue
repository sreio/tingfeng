<script setup>
import Icon from './Icon.vue';
import { newField } from '../lib/model.js';
const props = defineProps({ source: Object });
function add() {
  let n = props.source.fields.length + 1;
  while (props.source.fields.some((f) => f.key === 'field' + n)) n++;
  props.source.fields.push({ ...newField(), key: 'field' + n, label: '字段 ' + n });
}
function operation(field, value) {
  field.calculation = value
    ? {
        operation: value,
        operand: field.calculation?.operand || '',
        decimals: field.calculation?.decimals ?? 2
      }
    : null;
}
</script>

<template>
  <section class="extract-fields" aria-label="提取字段与数值换算">
    <div class="label-row">
      <strong>提取字段</strong><small>{{ source.fields.length }} / 16 个</small>
    </div>
    <p class="fine">一次请求可提取多个字段。列表默认展示第一个字段，通知可引用全部字段。</p>
    <div v-for="(field, i) in source.fields" :key="i" class="extract-field">
      <div class="label-row">
        <strong>字段 {{ i + 1 }}</strong>
        <button
          class="icon-btn"
          type="button"
          :disabled="source.fields.length === 1"
          :aria-label="'删除字段 ' + (i + 1)"
          @click="source.fields.splice(i, 1)"
        >
          <Icon name="trash" :size="16" />
        </button>
      </div>
      <div class="extract-grid">
        <label class="field"
          >显示名称<input
            v-model.trim="field.label"
            maxlength="100"
            placeholder="例如 买入价"
            :aria-label="'字段 ' + (i + 1) + ' 显示名称'"
        /></label>
        <label class="field"
          >变量名<input
            v-model.trim="field.key"
            maxlength="32"
            placeholder="例如 buyprice"
            :aria-label="'字段 ' + (i + 1) + ' 变量名'"
          /><small>字母开头，支持数字、下划线；每个字段不同。</small></label
        >
      </div>
      <label v-if="source.type === 'api'" class="field"
        >JSON 字段路径
        <input
          v-model.trim="field.path"
          placeholder="例如 /pronoinfo/0/buyprice，或从测试结果添加"
          :aria-label="'字段 ' + (i + 1) + ' JSON 路径'"
        />
        <small>留空提取完整 JSON；下方测试后可直接选择字段。</small>
      </label>
      <template v-else>
        <label class="field"
          >CSS 选择器<input
            v-model.trim="field.selector"
            placeholder="例如 .price；留空提取正文"
            :aria-label="'字段 ' + (i + 1) + ' CSS 选择器'"
        /></label>
        <label class="field"
          >元素属性（可选）<input
            v-model.trim="field.attribute"
            placeholder="例如 data-price；留空提取文字"
            :aria-label="'字段 ' + (i + 1) + ' 元素属性'"
        /></label>
      </template>
      <div class="extract-grid">
        <label class="field"
          >数值换算<select
            :value="field.calculation?.operation || ''"
            :aria-label="'字段 ' + (i + 1) + ' 换算方式'"
            @change="operation(field, $event.target.value)"
          >
            <option value="">保留原值</option>
            <option value="divide">除以 ÷</option>
            <option value="multiply">乘以 ×</option>
            <option value="add">加上 +</option>
            <option value="subtract">减去 −</option>
            <option value="round">仅设置小数位</option>
          </select></label
        >
        <label v-if="field.calculation && field.calculation.operation !== 'round'" class="field"
          >换算参数
          <input
            v-model.trim="field.calculation.operand"
            inputmode="decimal"
            placeholder="例如 100"
            maxlength="64"
            :aria-label="'字段 ' + (i + 1) + ' 换算参数'"
          />
        </label>
        <label v-if="field.calculation" class="field"
          >保留小数位
          <input
            v-model.number="field.calculation.decimals"
            type="number"
            min="0"
            max="12"
            :aria-label="'字段 ' + (i + 1) + ' 小数位'"
          />
        </label>
        <label class="field"
          >单位（可选）<input
            v-model.trim="field.unit"
            maxlength="30"
            placeholder="例如 元/克、%、件"
            :aria-label="'字段 ' + (i + 1) + ' 单位'"
        /></label>
      </div>
      <p v-if="field.calculation" class="fine">
        先换算再四舍五入，提醒阈值与历史记录均使用换算后的值。单位仅作为显示文字。
      </p>
    </div>
    <button v-if="source.fields.length < 16" class="text-btn" type="button" @click="add">
      <Icon name="plus" :size="16" />添加一个字段
    </button>
  </section>
</template>

<style scoped>
.extract-fields {
  margin: 24px 0;
}
.extract-field {
  border: 1px solid #dce3ef;
  border-radius: 12px;
  padding: 18px;
  margin: 12px 0;
  background: #fbfcff;
}
.extract-field > .label-row {
  margin-bottom: 14px;
}
.extract-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.extract-grid .field {
  min-width: 0;
}
@media (max-width: 560px) {
  .extract-grid {
    grid-template-columns: 1fr;
  }
  .extract-field {
    padding: 14px;
  }
}
</style>
