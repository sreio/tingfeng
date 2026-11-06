<script setup>
import { computed, inject, ref, watch } from 'vue';
import Icon from '../components/Icon.vue';
import PasswordInput from '../components/PasswordInput.vue';
import { api, download } from '../lib/api.js';
import { clone, fmt, size } from '../lib/model.js';
const props = defineProps({ config: Object, system: Object, auth: Object });
const emit = defineEmits(['passwordChanged']);
const notice = inject('notice'),
  saveConfig = inject('saveConfig'),
  refresh = inject('refreshStatus'),
  confirm = inject('confirm');
const defaults = ref(clone(props.config.settings)),
  busy = ref(false),
  error = ref(''),
  current = ref(''),
  password = ref(''),
  again = ref(''),
  passwordError = ref(''),
  changing = ref(false),
  showPassword = ref(false);
const runtimeSaving = ref(''),
  runtimeErrors = ref({});
const privateEnabled = computed(
  () => props.config.settings.allowPrivate ?? props.system?.allowPrivate ?? false
);
const restoreOpen = ref(false),
  backupText = ref(''),
  restorePreview = ref(null),
  backupFile = ref(null),
  restoreError = ref(''),
  fileInput = ref(null);
watch(
  () => props.config.settings,
  (v) => (defaults.value = clone(v))
);
async function saveDefaults() {
  error.value = '';
  busy.value = true;
  try {
    const next = clone(props.config);
    next.settings = clone(defaults.value);
    await saveConfig(next);
    notice('设置已保存。默认保留策略用于新建监控。');
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function saveRuntimeSetting(key, value) {
  if (runtimeSaving.value || busy.value) return;
  runtimeSaving.value = key;
  runtimeErrors.value[key] = '';
  const draftDefaults = clone(defaults.value);
  try {
    const next = clone(props.config);
    next.settings[key] = value;
    await saveConfig(next);
    defaults.value = { ...draftDefaults, [key]: value };
    if (key === 'deliveryConcurrency') {
      notice('通知发送并发数已调整为 ' + value + '。');
    } else {
      notice(
        value ? '内部网络访问已开启，立即生效。' : '内部网络访问已关闭，新请求仅允许公网地址。'
      );
    }
  } catch (e) {
    defaults.value[key] = props.config.settings[key];
    runtimeErrors.value[key] = e.message;
  } finally {
    runtimeSaving.value = '';
  }
}
async function changePassword() {
  passwordError.value = '';
  if (password.value !== again.value) {
    passwordError.value = '两次新密码不一致。';
    return;
  }
  changing.value = true;
  try {
    await api('/auth/password', {
      method: 'POST',
      body: { current: current.value, password: password.value }
    });
    current.value = '';
    password.value = '';
    again.value = '';
    emit('passwordChanged');
    notice('密码已修改，所有旧登录已失效，请重新登录。');
  } catch (e) {
    passwordError.value = e.message;
  } finally {
    changing.value = false;
  }
}
async function cleanup() {
  busy.value = true;
  try {
    const r = await api('/maintenance/cleanup', { method: 'POST', body: {} });
    await refresh();
    notice('清理完成，删除 ' + r.removed + ' 条到期或超限记录。');
  } catch (e) {
    notice(e.message);
  } finally {
    busy.value = false;
  }
}
async function exportBackup() {
  try {
    await download('/backup', '听风-配置备份.json');
  } catch (e) {
    notice(e.message);
  }
}
function changed() {
  restorePreview.value = null;
  backupFile.value = null;
  restoreError.value = '';
}
async function readFile(e) {
  const file = e.target.files?.[0];
  if (!file) return;
  try {
    if (file.size > 2 * 1024 * 1024) throw Error('备份文件不能超过 2 MB。');
    backupText.value = await file.text();
    changed();
    await preview();
  } catch (e) {
    restoreError.value = e.message;
  } finally {
    e.target.value = '';
  }
}
async function preview() {
  restoreError.value = '';
  restorePreview.value = null;
  try {
    if (backupText.value.length > 2 * 1024 * 1024) throw Error('备份内容超过大小限制。');
    const b = JSON.parse(backupText.value);
    restorePreview.value = await api('/restore', {
      method: 'POST',
      body: { backup: b, preview: true, revision: props.config.revision }
    });
    backupFile.value = b;
  } catch (e) {
    restoreError.value = e instanceof SyntaxError ? '备份不是有效的 JSON。' : e.message;
  }
}
async function restore() {
  if (
    !backupFile.value ||
    !(await confirm(
      '恢复配置备份？',
      '同标识的配置会覆盖，其余保留。导入的监控暂停、渠道停用；请补充密钥并测试后启用。管理员密码不受影响。'
    ))
  )
    return;
  busy.value = true;
  try {
    const restored = await api('/restore', {
      method: 'POST',
      body: { backup: backupFile.value, preview: false, revision: props.config.revision }
    });
    props.config.revision = restored.revision;
    Object.assign(props.config, restored);
    await refresh();
    restoreOpen.value = false;
    backupText.value = '';
    changed();
    notice('配置已恢复，请在监控和渠道中完成检查后启用。');
  } catch (e) {
    restoreError.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main class="page settings-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">轻量运行，按需保留</div>
        <h1>系统设置</h1>
        <p>管理访问密码、默认保留策略和配置备份。</p>
      </div>
    </div>
    <section class="panel settings-panel">
      <div class="security-section-heading">
        <h2>访问安全</h2>
        <span class="tag">单管理员</span>
      </div>
      <div class="setting-row">
        <div>
          <strong>管理员密码</strong>
          <p>最近更新：{{ fmt(auth.passwordUpdatedAt, config.settings.timeZone) }}</p>
          <p>修改后，所有已登录的浏览器都需要重新登录。</p>
        </div>
        <button class="btn" @click="showPassword = !showPassword">
          <Icon name="lock" :size="16" />修改密码
        </button>
      </div>
      <form v-if="showPassword" class="password-change-form" @submit.prevent="changePassword">
        <PasswordInput v-model="current" label="当前密码" /><PasswordInput
          v-model="password"
          label="新密码"
          autocomplete="new-password"
        /><PasswordInput v-model="again" label="确认新密码" autocomplete="new-password" />
        <p v-if="passwordError" class="error" role="alert">{{ passwordError }}</p>
        <button class="btn primary" :disabled="changing">
          {{ changing ? '正在修改…' : '确认修改密码' }}
        </button>
      </form>
    </section>
    <section class="panel settings-panel">
      <h2>默认与保留策略</h2>
      <p class="fine">
        默认历史和日志设置只用于新建监控；时区、发送记录保留和空间预算对整个实例生效。
      </p>
      <form @submit.prevent="saveDefaults">
        <div class="setting-row">
          <div>
            <strong>时间显示与定时发送</strong>
            <p>修改时区会重新计算定时通知。</p>
          </div>
          <select v-model="defaults.timeZone" aria-label="系统时区">
            <option>Asia/Shanghai</option>
            <option>UTC</option>
            <option>Asia/Hong_Kong</option>
            <option>Asia/Tokyo</option>
            <option>Europe/London</option>
            <option>America/New_York</option>
          </select>
        </div>
        <div class="setting-row">
          <div>
            <strong>默认历史保留</strong>
            <p>实际历史受天数和记录上限共同限制。</p>
          </div>
          <label class="compact-number"
            ><input
              v-model.number="defaults.historyDays"
              type="number"
              min="1"
              max="365"
              aria-label="默认历史保留天数"
              required
            />天</label
          >
        </div>
        <div class="setting-row">
          <strong>每个监控的默认记录上限</strong
          ><input
            class="compact-input"
            v-model.number="defaults.maxRows"
            type="number"
            min="1"
            max="100000"
            aria-label="默认记录上限"
            required
          />
        </div>
        <div class="setting-row">
          <strong>默认过程日志</strong
          ><select v-model="defaults.logLevel" aria-label="默认过程日志">
            <option value="off">关闭</option>
            <option value="error">仅记录错误</option>
            <option value="summary">记录执行摘要</option>
          </select>
        </div>
        <div class="setting-row">
          <strong>默认日志保留</strong
          ><label class="compact-number"
            ><input
              v-model.number="defaults.logDays"
              type="number"
              min="1"
              max="30"
              aria-label="默认日志保留天数"
              required
            />天</label
          >
        </div>
        <div class="setting-row">
          <strong>发送记录保留</strong
          ><label class="compact-number"
            ><input
              v-model.number="defaults.deliveryDays"
              type="number"
              min="1"
              max="90"
              aria-label="发送记录保留天数"
              required
            />天</label
          >
        </div>
        <div class="setting-row">
          <div>
            <strong>历史与日志的存储预算</strong>
            <p>接近预算时暂停积累历史，优先保证采集、最新状态与投递。</p>
          </div>
          <label class="compact-number"
            ><input
              v-model.number="defaults.diskLimitMB"
              type="number"
              min="32"
              max="102400"
              aria-label="存储预算 MB"
              required
            />MB</label
          >
        </div>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <button class="btn primary" :disabled="busy || !!runtimeSaving">
          {{ busy ? '保存中…' : '保存默认设置' }}
        </button>
      </form>
    </section>
    <section class="panel settings-panel">
      <h2>运行与存储</h2>
      <div v-if="system" class="resource-grid">
        <div>
          <small>数据库与 WAL</small><strong>{{ size(system.databaseBytes) }}</strong>
        </div>
        <div>
          <small>Go 堆内存</small><strong>{{ size(system.heapBytes) }}</strong>
        </div>
        <div>
          <small>采集并发数</small><strong>{{ system.workers }}</strong>
        </div>
      </div>
      <p class="fine">
        堆内存不是进程总内存。每次采集响应最多
        {{ size(system?.maxResponseBytes || 2097152) }}，防止意外占用大量内存。
      </p>
      <div class="setting-row">
        <div>
          <label for="delivery-concurrency"><strong>通知发送并发数</strong></label>
          <p id="delivery-concurrency-help">
            所有监控最多同时发送多少条通知。选择后立即生效，调低时正在发送的通知会继续完成。
          </p>
          <p v-if="runtimeErrors.deliveryConcurrency" class="error" role="alert">
            {{ runtimeErrors.deliveryConcurrency }}
          </p>
        </div>
        <select
          id="delivery-concurrency"
          v-model.number="defaults.deliveryConcurrency"
          aria-describedby="delivery-concurrency-help"
          :disabled="busy || !!runtimeSaving"
          :aria-busy="runtimeSaving === 'deliveryConcurrency'"
          @change="saveRuntimeSetting('deliveryConcurrency', defaults.deliveryConcurrency)"
        >
          <option v-for="n in 20" :key="n" :value="n">
            {{ n }}{{ n === 5 ? '（默认）' : '' }}
          </option>
        </select>
      </div>
      <div class="setting-row">
        <div>
          <strong>自动清理</strong>
          <p>每小时运行；每次采集也会执行该监控的记录上限。</p>
          <p>最近清理：{{ fmt(system?.lastCleanup, config.settings.timeZone) }}</p>
        </div>
        <button class="btn" :disabled="busy" @click="cleanup">立即清理</button>
      </div>
      <div class="setting-row">
        <div>
          <strong>内部网络来源</strong>
          <p>
            {{
              privateEnabled
                ? '已允许采集和通知渠道访问服务器内网、本机及保留地址，请只配置可信目标。'
                : '已阻止内网、回环和保留地址（包括云主机元数据地址），当前仅允许公网请求。'
            }}
          </p>
          <p>
            点击开关自动保存，无需改配置文件或重启。关闭后新请求立即恢复限制，已发出的请求不会中断。
          </p>
          <p v-if="runtimeErrors.allowPrivate" class="error" role="alert">
            {{ runtimeErrors.allowPrivate }}
          </p>
        </div>
        <div class="network-control">
          <button
            type="button"
            role="switch"
            class="switch"
            :class="{ on: privateEnabled }"
            :aria-checked="privateEnabled"
            :aria-busy="runtimeSaving === 'allowPrivate'"
            aria-label="允许访问内部网络"
            :disabled="busy || !!runtimeSaving || !system"
            @click="saveRuntimeSetting('allowPrivate', !privateEnabled)"
          >
            <span />
          </button>
          <small role="status">{{
            runtimeSaving === 'allowPrivate' ? '保存中…' : privateEnabled ? '已开启' : '已关闭'
          }}</small>
        </div>
      </div>
    </section>
    <section class="panel settings-panel">
      <h2>配置备份</h2>
      <p class="fine">
        导出监控、提醒、接收人、渠道及保留策略。不会导出密码、密钥、机器人地址、请求鉴权和历史；完整搬迁可使用命令行
        backup 命令。
      </p>
      <div class="backup-actions">
        <button class="btn" @click="exportBackup">导出配置</button
        ><button
          class="btn"
          @click="
            restoreOpen = !restoreOpen;
            changed();
          "
        >
          恢复备份
        </button>
      </div>
      <div v-if="restoreOpen">
        <div class="field">
          <label for="backup-json">粘贴备份 JSON</label
          ><textarea
            id="backup-json"
            v-model="backupText"
            @input="changed"
            rows="5"
            placeholder="也可以选择本地备份文件"
          />
        </div>
        <input
          ref="fileInput"
          type="file"
          accept=".json,application/json"
          hidden
          @change="readFile"
        />
        <div class="backup-actions">
          <button class="btn" @click="fileInput.click()">选择文件</button
          ><button class="btn" @click="preview">预览恢复内容</button>
        </div>
        <p v-if="restoreError" class="error" role="alert">{{ restoreError }}</p>
        <div v-if="restorePreview" class="restore-summary">
          <strong
            >恢复 {{ restorePreview.monitors }} 个监控、{{ restorePreview.recipients }} 位接收人、{{
              restorePreview.channels
            }}
            个渠道</strong
          >
          <p>{{ restorePreview.message }}</p>
          <button class="btn primary" :disabled="busy" @click="restore">确认恢复配置</button>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.network-control {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  min-width: 56px;
}
</style>
