<script setup>
import { computed, inject, ref } from 'vue';
import Dialog from './Dialog.vue';
import ChannelEditor from './ChannelEditor.vue';
import TelegramChatInput from './TelegramChatInput.vue';
import Icon from './Icon.vue';
import { clone, newPerson, uid } from '../lib/model.js';
const props = defineProps({ person: Object, config: Object });
const emit = defineEmits(['close', 'saved']);
const saveConfig = inject('saveConfig'),
  notice = inject('notice');
const form = ref(clone(props.person || newPerson())),
  pending = ref([]),
  busy = ref(false),
  error = ref(''),
  channelOpen = ref(false),
  methodIndex = ref(-1);
const channels = computed(() => [...props.config.channels, ...pending.value]);
const channelType = (id) => channels.value.find((c) => c.id === id)?.type;
function selectChannel(method, id) {
  if (channelType(method.channelId) !== channelType(id)) method.address = '';
  method.channelId = id;
}
function addMethod() {
  form.value.methods.push({ id: uid('method'), channelId: '', address: '', default: true });
}
function newRobot() {
  addMethod();
  methodIndex.value = form.value.methods.length - 1;
  channelOpen.value = true;
}
function bind(channel) {
  pending.value.push(channel);
  selectChannel(form.value.methods[methodIndex.value], channel.id);
  channelOpen.value = false;
}
async function save() {
  error.value = '';
  if (!form.value.name.trim()) {
    error.value = '请输入接收人或群组名称。';
    return;
  }
  if (
    !form.value.methods.length ||
    form.value.methods.some((m) => !channels.value.some((c) => c.id === m.channelId))
  ) {
    error.value = '请至少配置一种通知方式。';
    return;
  }
  if (
    form.value.methods.some(
      (m) => channelType(m.channelId) === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(m.address)
    )
  ) {
    error.value = '请输入有效的接收邮箱。';
    return;
  }
  busy.value = true;
  try {
    const next = clone(props.config),
      p = clone(form.value),
      i = next.recipients.findIndex((x) => x.id === p.id);
    next.channels.push(...pending.value.filter((c) => p.methods.some((m) => m.channelId === c.id)));
    if (i < 0) next.recipients.push(p);
    else next.recipients[i] = p;
    await saveConfig(next);
    notice('接收人及通知方式已保存。');
    emit('saved', p);
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <Dialog open :title="person ? '编辑接收人' : '添加接收人'" :busy="busy" @close="$emit('close')"
    ><form id="person-form" @submit.prevent="save">
      <p class="fine">一个人可以同时拥有企业微信、邮箱和其他通知方式，不需要后台账号。</p>
      <div class="segmented">
        <button
          type="button"
          :class="{ selected: form.kind === 'person' }"
          @click="form.kind = 'person'"
        >
          个人</button
        ><button
          type="button"
          :class="{ selected: form.kind === 'group' }"
          @click="form.kind = 'group'"
        >
          群组
        </button>
      </div>
      <div class="field">
        <label for="person-name">{{ form.kind === 'group' ? '群组名称' : '接收人名称' }}</label
        ><input
          id="person-name"
          v-model.trim="form.name"
          placeholder="例如 我自己、项目小组"
          maxlength="100"
          required
        />
      </div>
      <div class="field">
        <label for="person-note">备注（可选）</label
        ><input id="person-note" v-model="form.note" maxlength="200" />
      </div>
      <button class="text-btn" type="button" @click="newRobot">
        <Icon name="plus" :size="16" />快捷添加企业微信机器人
      </button>
      <div class="label-row methods-label">
        <h3>通知方式</h3>
        <span class="fine">可添加多个</span>
      </div>
      <div v-for="(m, i) in form.methods" :key="m.id" class="method-form">
        <div class="label-row">
          <strong>方式 {{ i + 1 }}</strong
          ><button
            class="icon-btn"
            type="button"
            :aria-label="'移除通知方式 ' + (i + 1)"
            @click="form.methods.splice(i, 1)"
          >
            <Icon name="trash" :size="16" />
          </button>
        </div>
        <div class="field">
          <label :for="'person-channel-' + i">通知渠道</label
          ><select
            :id="'person-channel-' + i"
            :value="m.channelId"
            @change="selectChannel(m, $event.target.value)"
          >
            <option value="" disabled>选择已配置的渠道</option>
            <option v-for="c in channels" :key="c.id" :value="c.id">
              {{ c.name }}{{ c.enabled ? '' : '（已停用）' }}
            </option></select
          ><button
            type="button"
            class="text-btn small inline-create"
            @click="
              methodIndex = i;
              channelOpen = true;
            "
          >
            没有合适的？新建渠道
          </button>
        </div>
        <div v-if="channelType(m.channelId) === 'email'" class="field">
          <label :for="'person-email-' + i">接收邮箱</label
          ><input
            :id="'person-email-' + i"
            v-model.trim="m.address"
            type="email"
            placeholder="me@example.com"
            required
          />
        </div>
        <TelegramChatInput
          v-else-if="channelType(m.channelId) === 'telegram'"
          v-model="m.address"
          label="接收 Chat ID"
          required
        />
        <label class="checkbox-row"
          ><input type="checkbox" v-model="m.default" /><span>作为这个人的默认通知方式</span></label
        >
      </div>
      <button type="button" class="add-recipient" @click="addMethod">
        <Icon name="plus" :size="16" />再添加一种通知方式
      </button>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </form>
    <template #footer
      ><button class="btn" :disabled="busy" @click="$emit('close')">取消</button
      ><button class="btn primary" type="submit" form="person-form" :disabled="busy">
        {{ busy ? '保存中…' : '保存接收人' }}
      </button></template
    ></Dialog
  ><ChannelEditor
    v-if="channelOpen"
    :config="config"
    :initial-name="form.name ? form.name + ' · 企业微信' : '我的企业微信'"
    defer
    @close="channelOpen = false"
    @saved="bind"
  />
</template>
