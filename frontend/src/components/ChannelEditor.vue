<script setup>
import { computed, inject, ref, watch } from 'vue';
import Dialog from './Dialog.vue';
import Icon from './Icon.vue';
import { api } from '../lib/api.js';
import { channelTypes, clone, newChannel, parsePairs, pairsText } from '../lib/model.js';
const webhookExample = '例如：{"content": {{json .Text}}}；使用 json 函数处理引号与换行。';
const props = defineProps({ channel: Object, config: Object, defer: Boolean, initialName: String });
const emit = defineEmits(['close', 'saved']);
const saveConfig = inject('saveConfig'),
  notice = inject('notice');
const form = ref({
    ...newChannel(),
    ...clone(props.channel || {}),
    name: props.channel?.name || props.initialName || ''
  }),
  busy = ref(false),
  testing = ref(false),
  error = ref(''),
  testResult = ref(''),
  address = ref(''),
  testText = ref('');
const headerText = ref(pairsText(form.value.headers)),
  has = (k) => form.value.secretsSet?.includes(k);
const needsURL = computed(() => !['email', 'serverchan'].includes(form.value.type));
watch(
  () => form.value.type,
  () => {
    form.value.url = '';
    form.value.secret = '';
    form.value.password = '';
    form.value.headers = {};
    form.value.secretsSet = [];
    headerText.value = '';
    testResult.value = '';
  }
);
function prepared() {
  form.value.headers = parsePairs(headerText.value);
  return clone(form.value);
}
async function test() {
  error.value = '';
  testResult.value = '';
  let channel;
  try {
    channel = prepared();
  } catch (e) {
    error.value = e.message;
    return;
  }
  testing.value = true;
  try {
    const r = await api('/channels/test', {
      method: 'POST',
      body: { channel, address: address.value, text: testText.value }
    });
    testResult.value = r.message;
  } catch (e) {
    error.value = e.message;
  } finally {
    testing.value = false;
  }
}
async function save() {
  error.value = '';
  let channel;
  try {
    channel = prepared();
    if (!channel.name.trim()) throw Error('请为渠道起个名字。');
    if (needsURL.value && !channel.url && !has('url')) throw Error('请填写通知地址。');
    if (channel.type === 'email' && (!channel.host || !channel.sender))
      throw Error('请填写 SMTP 服务地址和发件邮箱。');
    if (channel.type === 'serverchan' && !channel.secret && !has('secret'))
      throw Error('请填写 SendKey。');
  } catch (e) {
    error.value = e.message;
    return;
  }
  busy.value = true;
  try {
    if (!props.defer) {
      const next = clone(props.config),
        i = next.channels.findIndex((c) => c.id === channel.id);
      if (i < 0) next.channels.push(channel);
      else next.channels[i] = channel;
      await saveConfig(next);
      notice('通知渠道已保存。');
    }
    emit('saved', channel);
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <Dialog
    open
    :title="channel ? '配置通知渠道' : '添加通知渠道'"
    :busy="busy || testing"
    @close="$emit('close')"
    ><form id="channel-form" @submit.prevent="save">
      <div class="field">
        <label for="channel-name">渠道名称</label
        ><input
          id="channel-name"
          v-model.trim="form.name"
          placeholder="例如 我的企业微信、个人邮箱服务"
          maxlength="100"
          required
        />
      </div>
      <div class="field">
        <label for="channel-type">渠道类型</label
        ><select id="channel-type" v-model="form.type">
          <option v-for="t in channelTypes" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </div>
      <div v-if="needsURL" class="field">
        <label for="channel-url">Webhook / 机器人地址</label
        ><input
          id="channel-url"
          v-model.trim="form.url"
          type="password"
          autocomplete="off"
          :placeholder="has('url') ? '已安全保存，留空保持原地址' : '粘贴完整的 HTTP(S) 通知地址'"
        />
      </div>
      <template v-if="form.type === 'email'"
        ><div class="two-fields">
          <div class="field">
            <label for="smtp-host">SMTP 服务器</label
            ><input
              id="smtp-host"
              v-model.trim="form.host"
              placeholder="smtp.example.com"
              required
            />
          </div>
          <div class="field">
            <label for="smtp-port">端口</label
            ><input
              id="smtp-port"
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              required
            />
          </div>
        </div>
        <div class="field">
          <label for="smtp-tls">连接加密</label
          ><select id="smtp-tls" v-model="form.tls">
            <option value="tls">TLS（常见端口 465）</option>
            <option value="starttls">STARTTLS（常见端口 587）</option>
          </select>
        </div>
        <div class="field">
          <label for="smtp-sender">发件邮箱</label
          ><input
            id="smtp-sender"
            v-model.trim="form.sender"
            type="email"
            placeholder="notify@example.com"
            required
          />
        </div>
        <div class="field">
          <label for="smtp-user">SMTP 登录名</label
          ><input id="smtp-user" v-model.trim="form.user" autocomplete="off" />
        </div>
        <div class="field">
          <label for="smtp-password">SMTP 密码 / 授权码</label
          ><input
            id="smtp-password"
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            :placeholder="has('password') ? '已安全保存，留空保持原值' : '填写邮箱授权码'"
          /></div
      ></template>
      <div v-if="['dingtalk', 'feishu', 'serverchan'].includes(form.type)" class="field">
        <label for="channel-secret">{{
          form.type === 'serverchan' ? 'SendKey' : '签名密钥（机器人启用加签时填写）'
        }}</label
        ><input
          id="channel-secret"
          v-model.trim="form.secret"
          type="password"
          autocomplete="new-password"
          :placeholder="
            has('secret')
              ? '已安全保存，留空保持原值'
              : form.type === 'serverchan'
                ? '支持 SCT 与 sctp 开头的 SendKey'
                : '未启用签名时留空'
          "
        />
      </div>
      <details v-if="form.type === 'webhook'" class="advanced">
        <summary>自定义请求格式 <span>可选</span><Icon name="down" :size="16" /></summary>
        <div class="advanced-body">
          <div class="field">
            <label
              >请求方法<select v-model="form.method">
                <option>POST</option>
                <option>PUT</option>
              </select></label
            >
          </div>
          <div class="field">
            <label
              >附加请求头<textarea
                v-model="headerText"
                rows="3"
                :placeholder="has('headers') ? '已有请求头，留空保留' : 'Authorization: Bearer …'"
              />
            </label>
          </div>
          <div class="field">
            <label
              >JSON 消息模板<textarea
                v-model="form.bodyTemplate"
                rows="4"
                placeholder="留空发送完整的标准 JSON 消息"
              /></label
            ><small v-text="webhookExample" />
          </div>
        </div>
      </details>
      <label class="checkbox-row"
        ><input type="checkbox" v-model="form.enabled" /><span>启用此渠道</span></label
      >
      <details class="advanced" open>
        <summary>发送测试消息 <span>会真实发送</span><Icon name="down" :size="16" /></summary>
        <div class="advanced-body">
          <div v-if="form.type === 'email'" class="field">
            <label for="test-email">测试接收邮箱</label
            ><input
              id="test-email"
              v-model.trim="address"
              type="email"
              placeholder="me@example.com"
            />
          </div>
          <div class="field">
            <label for="test-message">测试消息（可选）</label
            ><input
              id="test-message"
              v-model="testText"
              maxlength="2000"
              placeholder="机器人有关键词限制时，在这里写入关键词"
            />
          </div>
          <button type="button" class="btn" :disabled="testing || busy" @click="test">
            <Icon name="send" :size="15" />{{ testing ? '正在发送…' : '发送测试消息' }}
          </button>
          <p v-if="testResult" class="success-note" role="status">{{ testResult }}</p>
        </div>
      </details>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p class="fine">密钥在服务器加密保存，不写入浏览器本地存储。</p>
    </form>
    <template #footer
      ><button type="button" class="btn" :disabled="busy || testing" @click="$emit('close')">
        取消</button
      ><button class="btn primary" form="channel-form" type="submit" :disabled="busy || testing">
        {{ busy ? '保存中…' : defer ? '保存并绑定' : '保存渠道' }}
      </button></template
    ></Dialog
  >
</template>
