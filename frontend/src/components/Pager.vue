<script setup>
defineProps({ page: Number, total: Number, size: { type: Number, default: 25 }, busy: Boolean });
defineEmits(['change']);
</script>
<template>
  <div class="pager">
    <small>共 {{ total }} 条 · 第 {{ page }} / {{ Math.max(1, Math.ceil(total / size)) }} 页</small
    ><button class="btn small" :disabled="busy || page <= 1" @click="$emit('change', page - 1)">
      上一页</button
    ><button
      class="btn small"
      :disabled="busy || page * size >= total"
      @click="$emit('change', page + 1)"
    >
      下一页
    </button>
  </div>
</template>
