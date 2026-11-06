<script setup>
import { computed, ref } from 'vue';
import Icon from './Icon.vue';
import { targets, defaults, destination } from '../lib/model.js';
const props = defineProps({ config: Object, rule: Object });
defineEmits(['edit']);
const selected = computed(() => targets(props.config, props.rule.selections)),
  open = ref(!selected.value.length);
const channel = (id) => props.config.channels.find((c) => c.id === id);
function toggle(p) {
  const prefix = p.id + ':';
  if (props.rule.selections.some((k) => k.startsWith(prefix)))
    props.rule.selections = props.rule.selections.filter((k) => !k.startsWith(prefix));
  else {
    const def = defaults(p, props.config);
    props.rule.selections.push(
      ...(def.length
        ? def
        : targets(
            props.config,
            p.methods.map((m) => prefix + m.id)
          ).map((t) => t.key))
    );
  }
}
</script>
<template>
  <div class="fast-target">
    <span class="target-symbol"><Icon name="user" :size="18" /></span>
    <div>
      <small>发给</small><strong>{{ destination(config, rule) }}</strong>
    </div>
    <button type="button" class="text-btn" @click="open = !open" :aria-expanded="open">
      {{ open ? '收起' : '更改' }}
    </button>
  </div>
  <div v-if="open" class="recipient-picker">
    <div
      v-for="p in config.recipients"
      :key="p.id"
      class="recipient-choice"
      :class="{ selected: rule.selections.some((k) => k.startsWith(p.id + ':')) }"
    >
      <div class="recipient-top">
        <label
          ><input
            type="checkbox"
            :checked="rule.selections.some((k) => k.startsWith(p.id + ':'))"
            @change="toggle(p)"
          /><strong>{{ p.name }}</strong></label
        ><button type="button" class="text-btn small" @click="$emit('edit', p, rule)">编辑</button>
      </div>
      <div class="method-choices">
        <label
          v-for="m in p.methods"
          :key="m.id"
          :class="{ unavailable: !channel(m.channelId)?.enabled }"
          ><input
            type="checkbox"
            v-model="rule.selections"
            :value="p.id + ':' + m.id"
            :disabled="!channel(m.channelId)?.enabled"
          />{{ channel(m.channelId)?.name || '渠道已删除'
          }}<small v-if="channel(m.channelId)?.type === 'telegram'">{{ m.address }}</small
          ><small>{{
            !channel(m.channelId)?.enabled ? '已停用' : m.default ? '默认' : ''
          }}</small></label
        >
      </div>
    </div>
    <button type="button" class="add-recipient" @click="$emit('edit', null, rule)">
      <Icon name="plus" :size="17" />添加接收人及通知方式<span>在这里一起完成，无需离开流程</span>
    </button>
  </div>
</template>
