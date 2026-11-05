<script setup>
import { computed, ref } from 'vue';
import HistoryPanel from '../components/HistoryPanel.vue';
import Icon from '../components/Icon.vue';
const props = defineProps({ config: Object });
defineEmits(['edit']);
const selected = ref(props.config.monitors[0]?.id || '');
const monitor = computed(
  () => props.config.monitors.find((m) => m.id === selected.value) || props.config.monitors[0]
);
</script>
<template>
  <main class="page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">让变化有迹可循</div>
        <h1>数据记录</h1>
        <p>查看历史、观察趋势，或把数据导出后继续分析。</p>
      </div>
      <select v-if="config.monitors.length" v-model="selected" aria-label="选择数据监控">
        <option v-for="m in config.monitors" :key="m.id" :value="m.id">{{ m.name }}</option>
      </select>
    </div>
    <section v-if="monitor" class="panel data-panel">
      <HistoryPanel :monitor="monitor" :time-zone="config.settings.timeZone" /><button
        v-if="!monitor.storage.history"
        class="btn"
        @click="$emit('edit', monitor)"
      >
        编辑监控，开启保存历史
      </button>
    </section>
    <section v-else class="panel empty">
      <Icon name="data" :size="36" />
      <h3>还没有监控任务</h3>
      <p>新建监控并开启保存历史后，数据会出现在这里。</p>
    </section>
  </main>
</template>
