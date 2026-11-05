<script setup>
import Icon from './Icon.vue';
defineProps({ item: Object, preview: Boolean });
</script>
<template>
  <div class="message-preview rich-message">
    <div class="message-brand">
      <span class="tiny-brand"><Icon name="wind" :size="13" /></span>听风通知<span v-if="preview"
        >消息预览</span
      >
    </div>
    <h3>{{ item.title }}</h3>
    <p class="message-text">
      {{
        item.text?.startsWith(item.title + '\n')
          ? item.text.slice(item.title.length + 1)
          : item.text
      }}
    </p>
    <a
      v-if="item.sourceURL"
      :href="item.sourceURL"
      target="_blank"
      rel="noopener noreferrer"
      class="text-btn"
      ><Icon name="globe" :size="14" />查看来源</a
    >
  </div>
</template>
