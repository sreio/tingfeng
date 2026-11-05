<script setup>
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue';
import Icon from './Icon.vue';
const props = defineProps({ open: Boolean, title: String, wide: Boolean, busy: Boolean });
const emit = defineEmits(['close']);
const dialog = ref(null),
  id = useId();
watch(
  () => props.open,
  async (open) => {
    await nextTick();
    if (!dialog.value) return;
    if (open && !dialog.value.open) dialog.value.showModal();
    else if (!open && dialog.value.open) dialog.value.close();
  },
  { immediate: true }
);
onBeforeUnmount(() => dialog.value?.close());
function close() {
  if (!props.busy) emit('close');
}
</script>
<template>
  <dialog
    ref="dialog"
    class="drawer"
    :class="{ 'detail-drawer': wide }"
    :aria-labelledby="id"
    @cancel.prevent="close"
  >
    <div class="drawer-header">
      <div>
        <span class="eyebrow">听风工作空间</span>
        <h2 :id="id">{{ title }}</h2>
      </div>
      <button
        type="button"
        class="icon-btn"
        :aria-label="'关闭' + title"
        :disabled="busy"
        @click="close"
      >
        <Icon name="close" />
      </button>
    </div>
    <div class="drawer-body"><slot /></div>
    <div v-if="$slots.footer" class="drawer-footer"><slot name="footer" /></div>
  </dialog>
</template>
