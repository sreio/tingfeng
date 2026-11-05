<script setup>
import { inject, ref } from 'vue';
import Icon from '../components/Icon.vue';
import PersonEditor from '../components/PersonEditor.vue';
import ChannelEditor from '../components/ChannelEditor.vue';
import DeliveriesPanel from '../components/DeliveriesPanel.vue';
import { clone, typeInfo } from '../lib/model.js';
const props = defineProps({ config: Object });
const notice = inject('notice'),
  confirm = inject('confirm'),
  saveConfig = inject('saveConfig');
const tab = ref('people'),
  person = ref(null),
  personOpen = ref(false),
  channel = ref(null),
  channelOpen = ref(false),
  busy = ref(false);
const channelById = (id) => props.config.channels.find((c) => c.id === id);
function openPerson(p = null) {
  person.value = p;
  personOpen.value = true;
}
function openChannel(c = null) {
  channel.value = c;
  channelOpen.value = true;
}
async function removePerson(p) {
  if (
    props.config.monitors.some((m) =>
      m.rules.some((r) => r.selections.some((k) => k.startsWith(p.id + ':')))
    )
  ) {
    notice('这个接收人仍被监控使用，请先更改相应提醒。');
    return;
  }
  if (!(await confirm('删除接收人？', '将删除「' + p.name + '」及其通知方式，渠道保留。'))) return;
  try {
    const c = clone(props.config);
    c.recipients = c.recipients.filter((x) => x.id !== p.id);
    await saveConfig(c);
  } catch (e) {
    notice(e.message);
  }
}
async function removeChannel(ch) {
  if (props.config.recipients.some((p) => p.methods.some((m) => m.channelId === ch.id))) {
    notice('这个渠道仍被接收人使用，请先移除相关通知方式。');
    return;
  }
  if (!(await confirm('删除通知渠道？', '将删除「' + ch.name + '」及其服务器中保存的密钥。')))
    return;
  try {
    const c = clone(props.config);
    c.channels = c.channels.filter((x) => x.id !== ch.id);
    await saveConfig(c);
  } catch (e) {
    notice(e.message);
  }
}
async function toggle(ch) {
  if (
    ch.enabled &&
    !(await confirm(
      '停用这个渠道？',
      '关联提醒会停止使用这个渠道，其他渠道仍可接收。已开始的投递无法撤回。'
    ))
  )
    return;
  busy.value = true;
  try {
    const c = clone(props.config);
    c.channels.find((x) => x.id === ch.id).enabled = !ch.enabled;
    await saveConfig(c);
    notice('渠道状态已更新。');
  } catch (e) {
    notice(e.message);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main class="page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">一次配置，随处使用</div>
        <h1>通知管理</h1>
        <p>先配置发给谁，再为他们搭配合适的通知方式。</p>
      </div>
      <button v-if="tab === 'people'" class="btn primary" @click="openPerson()">
        <Icon name="plus" :size="18" />添加接收人</button
      ><button v-if="tab === 'channels'" class="btn primary" @click="openChannel()">
        <Icon name="plus" :size="18" />添加渠道
      </button>
    </div>
    <div class="tabs section-tabs">
      <button
        v-for="t in [
          { id: 'people', name: '接收人' },
          { id: 'channels', name: '通知渠道' },
          { id: 'deliveries', name: '发送记录' }
        ]"
        :key="t.id"
        :class="{ active: tab === t.id }"
        @click="tab = t.id"
      >
        {{ t.name }}
      </button>
    </div>
    <template v-if="tab === 'people'"
      ><div class="info-line">
        <Icon name="info" :size="16" />一个人可以有多种通知方式；不同提醒可以选择不同渠道。
      </div>
      <div v-if="!config.recipients.length" class="panel empty">
        <Icon name="people" :size="34" />
        <h3>先把自己加进来</h3>
        <p>添加「我自己」，选一个常用的企业微信或邮箱，就可以开始接收提醒。</p>
        <button class="btn" @click="openPerson()">添加接收人</button>
      </div>
      <div class="recipient-grid">
        <article v-for="p in config.recipients" :key="p.id" class="panel recipient-card">
          <div class="recipient-card-head">
            <span class="avatar" :class="{ group: p.kind === 'group' }"
              ><Icon :name="p.kind === 'group' ? 'people' : 'user'"
            /></span>
            <div>
              <h3>{{ p.name }}</h3>
              <small
                >{{ p.kind === 'group' ? '群组' : '个人' }} ·
                {{ p.methods.length }} 种通知方式</small
              >
            </div>
            <button class="icon-btn" :aria-label="'编辑' + p.name" @click="openPerson(p)">
              <Icon name="edit" :size="17" />
            </button>
          </div>
          <p class="fine">{{ p.note || '暂无备注' }}</p>
          <div class="person-methods">
            <div v-for="m in p.methods" :key="m.id">
              <span class="channel-dot" :class="typeInfo(channelById(m.channelId)?.type).color">{{
                typeInfo(channelById(m.channelId)?.type).mark
              }}</span>
              <div>
                <strong>{{ channelById(m.channelId)?.name || '渠道已移除' }}</strong
                ><small>{{ m.address || '发送至机器人或推送终端' }}</small>
              </div>
              <span class="tag" v-if="!channelById(m.channelId)?.enabled">已停用</span
              ><span class="tag" v-else-if="m.default">默认</span>
            </div>
          </div>
          <div class="card-footer">
            <button class="text-btn" @click="openPerson(p)">管理通知方式</button
            ><button class="icon-btn" :aria-label="'删除' + p.name" @click="removePerson(p)">
              <Icon name="trash" :size="15" />
            </button>
          </div>
        </article></div
    ></template>
    <template v-else-if="tab === 'channels'"
      ><div class="info-line">
        <Icon
          name="info"
          :size="16"
        />渠道负责“怎么发”。密钥在服务器加密保存，列表不显示完整地址与授权信息。
      </div>
      <div v-if="!config.channels.length" class="panel empty">
        <h3>还没有通知渠道</h3>
        <p>可以直接添加，也可以在创建接收人时一起配置。</p>
        <button class="btn" @click="openChannel()">添加渠道</button>
      </div>
      <div class="channel-grid">
        <article v-for="ch in config.channels" :key="ch.id" class="panel channel-card">
          <div class="channel-card-head">
            <span class="channel-type-icon" :class="typeInfo(ch.type).color">{{
              typeInfo(ch.type).mark
            }}</span
            ><span class="status" :class="ch.enabled ? 'normal' : 'paused'"
              ><i />{{ ch.enabled ? '已启用' : '已停用' }}</span
            >
          </div>
          <h3>{{ ch.name }}</h3>
          <p>{{ typeInfo(ch.type).name }}</p>
          <small class="fine"
            >{{
              config.recipients.filter((p) => p.methods.some((m) => m.channelId === ch.id)).length
            }}
            位接收人使用</small
          >
          <div class="card-footer">
            <button class="text-btn" @click="openChannel(ch)">配置 / 测试</button
            ><button class="text-btn" :disabled="busy" @click="toggle(ch)">
              {{ ch.enabled ? '停用' : '启用' }}</button
            ><button class="icon-btn" :aria-label="'删除渠道' + ch.name" @click="removeChannel(ch)">
              <Icon name="trash" :size="15" />
            </button>
          </div>
        </article></div
    ></template>
    <section v-else class="panel data-panel">
      <DeliveriesPanel :time-zone="config.settings.timeZone" />
    </section>
    <PersonEditor
      v-if="personOpen"
      :person="person"
      :config="config"
      @close="personOpen = false"
      @saved="personOpen = false"
    /><ChannelEditor
      v-if="channelOpen"
      :channel="channel"
      :config="config"
      @close="channelOpen = false"
      @saved="channelOpen = false"
    />
  </main>
</template>
