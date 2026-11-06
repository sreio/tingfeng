<script setup>
import { useId } from 'vue';
defineProps({ label: String, required: Boolean });
const address = defineModel({ type: String, default: '' });
const id = useId();
</script>
<template>
  <div class="field">
    <label :for="id">{{ label }}</label>
    <input
      :id="id"
      v-model.trim="address"
      :required="required"
      :aria-describedby="id + '-help'"
      type="text"
      autocomplete="off"
      spellcheck="false"
      placeholder="数字 Chat ID 或 @公开频道用户名"
    />
    <small :id="id + '-help'"
      >私聊请先向机器人发送 /start。群组或频道请先添加机器人并授予发消息权限。</small
    >
  </div>
</template>
