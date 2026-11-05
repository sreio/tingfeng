<script setup>
import { ref, useId } from 'vue';
import Icon from './Icon.vue';
defineProps({
  modelValue: String,
  label: String,
  autocomplete: { type: String, default: 'current-password' }
});
defineEmits(['update:modelValue']);
const show = ref(false),
  id = useId();
</script>
<template>
  <div class="field password-field">
    <label :for="id">{{ label }}</label>
    <div class="password-input">
      <input
        :id="id"
        :type="show ? 'text' : 'password'"
        :value="modelValue"
        @input="$emit('update:modelValue', $event.target.value)"
        :autocomplete="autocomplete"
        minlength="8"
        maxlength="128"
        required
      /><button
        type="button"
        class="icon-btn"
        :aria-label="(show ? '隐藏' : '显示') + label"
        :aria-pressed="show"
        @click="show = !show"
      >
        <Icon :name="show ? 'eyeOff' : 'eye'" :size="18" />
      </button>
    </div>
  </div>
</template>
