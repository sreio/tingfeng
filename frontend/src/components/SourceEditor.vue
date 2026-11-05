<script setup>
import { computed, ref, watch } from 'vue';
import Icon from './Icon.vue';
import { api } from '../lib/api.js';
import { sourceTypes, parsePairs, pairsText } from '../lib/model.js';
const props = defineProps({ source: Object, monitorId: String });
const emit = defineEmits(['tested', 'invalidate', 'invalid', 'type']);
const busy = ref(false),
  error = ref(''),
  result = ref(null),
  fields = ref([]);
const headerText = ref(pairsText(props.source.headers)),
  queryText = ref(pairsText(props.source.query, '=')),
  advanced = ref(false);
watch(
  () => JSON.stringify(props.source),
  () => {
    emit('invalidate');
    result.value = null;
  }
);
watch([headerText, queryText], () => {
  emit('invalidate');
  try {
    props.source.headers = parsePairs(headerText.value);
    props.source.query = parsePairs(queryText.value, '=');
    error.value = '';
    emit('invalid', '');
  } catch (e) {
    error.value = e.message;
    emit('invalid', e.message);
  }
});
function setType(type) {
  props.source.type = type;
  props.source.method = 'GET';
  props.source.path = '';
  props.source.selector = '';
  result.value = null;
  fields.value = [];
  emit('type', type);
}
async function test() {
  error.value = '';
  try {
    props.source.headers = parsePairs(headerText.value);
    props.source.query = parsePairs(queryText.value, '=');
  } catch (e) {
    error.value = e.message;
    return;
  }
  busy.value = true;
  const signature = JSON.stringify(props.source);
  try {
    const p = await api('/sources/test', {
      method: 'POST',
      body: { source: props.source, monitorId: props.monitorId }
    });
    if (signature !== JSON.stringify(props.source)) {
      error.value = '配置已修改，请重新测试。';
      return;
    }
    result.value = p;
    fields.value = p.fields || [];
    emit('tested', p);
  } catch (e) {
    error.value = e.message;
    fields.value = e.data?.fields || [];
  } finally {
    busy.value = false;
  }
}
const value = computed(() => result.value?.observation.display || '');
</script>
<template>
  <div class="source-options">
    <button
      v-for="type in sourceTypes"
      type="button"
      :key="type.id"
      class="source-option"
      :class="{ selected: source.type === type.id }"
      @click="setType(type.id)"
    >
      <Icon :name="type.icon" :size="24" /><strong>{{ type.name }}</strong
      ><small>{{ type.description }}</small
      ><span class="radio-dot" />
    </button>
  </div>
  <div class="field">
    <label for="source-url">{{
      source.type === 'rss' ? '订阅地址' : source.type === 'page' ? '网页地址' : '请求地址'
    }}</label
    ><input
      id="source-url"
      v-model.trim="source.url"
      type="url"
      placeholder="https://example.com/api/price"
      autocomplete="off"
      required
    /><small v-if="source.hasRequestSecrets"
      >已保存认证或请求参数。留空会保留；更换域名后需要重新填写认证。</small
    >
  </div>
  <div v-if="source.type === 'api'" class="field">
    <label for="source-path">要关注的 JSON 字段</label
    ><input
      id="source-path"
      v-model.trim="source.path"
      placeholder="例如 data.price、items[0].price 或 /data/price"
    /><small>留空比较完整 JSON。测试后可以直接点击字段。</small>
  </div>
  <div v-if="source.type === 'page'" class="field">
    <label for="css-selector">CSS 选择器</label
    ><input
      id="css-selector"
      v-model.trim="source.selector"
      placeholder="例如 .price、#stock；留空关注正文"
    /><small>提取静态 HTML。页面由 JavaScript 渲染时，建议使用页面背后的数据接口。</small>
  </div>
  <details class="advanced">
    <summary>高级请求配置 <span>可选</span><Icon name="down" :size="16" /></summary>
    <div class="advanced-body">
      <div v-if="source.type === 'api'" class="field">
        <label for="source-method">请求方法</label
        ><select id="source-method" v-model="source.method">
          <option>GET</option>
          <option>POST</option>
        </select>
      </div>
      <div class="field">
        <label for="source-query">请求参数</label
        ><textarea
          id="source-query"
          v-model="queryText"
          rows="3"
          placeholder="symbol=gold&#10;currency=CNY"
          autocomplete="off"
        />
      </div>
      <div class="field">
        <label for="source-headers">请求头</label
        ><textarea
          id="source-headers"
          v-model="headerText"
          rows="3"
          placeholder="Authorization: Bearer …"
          autocomplete="off"
        />
      </div>
      <div v-if="source.method === 'POST'" class="field">
        <label for="source-body">请求体</label
        ><textarea
          id="source-body"
          v-model="source.body"
          rows="4"
          placeholder="JSON 或表单内容；对应的 Content-Type 可在请求头设置"
          autocomplete="off"
        />
      </div>
      <div v-if="source.type === 'page'" class="field">
        <label for="source-attribute">提取元素属性</label
        ><input
          id="source-attribute"
          v-model="source.attribute"
          placeholder="例如 href、data-price；留空提取文字"
        />
      </div>
      <div v-if="source.type === 'api'" class="field">
        <label for="source-time">来源更新时间字段</label
        ><input id="source-time" v-model="source.timePath" placeholder="例如 updated_at；可选" />
      </div>
      <div class="field">
        <label for="source-unit">显示单位</label
        ><input
          id="source-unit"
          v-model="source.unit"
          placeholder="例如 元/克、件；可选"
          maxlength="30"
        />
      </div>
      <label v-if="source.hasRequestSecrets" class="checkbox-row"
        ><input type="checkbox" v-model="source.clearRequestSecrets" /><span
          >清除已保存的请求认证与参数，仅使用这次填写的内容</span
        ></label
      >
    </div>
  </details>
  <button type="button" class="btn test-button" :disabled="busy" @click="test">
    <Icon :name="busy ? 'refresh' : 'play'" :size="16" :class="{ spinning: busy }" />{{
      busy ? '正在测试采集…' : '测试采集'
    }}
  </button>
  <div v-if="error" class="callout danger" role="alert">
    <Icon name="info" :size="18" />
    <div>
      <strong>这次没有采集成功</strong>
      <p>{{ error }}</p>
    </div>
  </div>
  <div v-if="result" class="callout source-result">
    <Icon name="check" :size="18" />
    <div>
      <strong>采集成功 · {{ result.millis }} ms</strong>
      <p class="value-wrap">{{ value }}</p>
      <small v-if="source.type === 'rss'"
        >发现
        {{ result.observation.items?.length || 0 }}
        个条目。首次启用只建立基准，不批量发送已有内容。</small
      >
    </div>
  </div>
  <div v-if="source.type === 'api' && fields.length" class="field-picker">
    <strong>从响应中选择字段</strong>
    <p class="fine">点击后重新测试，确认提取结果。</p>
    <button
      v-for="f in fields"
      :key="f.path"
      type="button"
      :class="{ selected: source.path === f.path }"
      @click="source.path = f.path"
    >
      <code>{{ f.path }}</code
      ><small>{{ f.type }}</small
      ><span>{{ f.value }}</span>
    </button>
  </div>
</template>
