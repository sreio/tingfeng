export const uid = (p) => p + '_' + crypto.randomUUID().replaceAll('-', '');
export const clone = (v) => JSON.parse(JSON.stringify(v));
export const sourceTypes = [
  { id: 'api', name: 'HTTP 接口', description: '从 JSON 接口提取数据', icon: 'code' },
  { id: 'page', name: '网页', description: '用 CSS 选择器关注页面内容', icon: 'globe' },
  { id: 'rss', name: 'RSS / Atom', description: '只接收新的文章与版本更新', icon: 'rss' }
];
export const channelTypes = [
  { id: 'wecom', name: '企业微信', mark: '企', color: 'blue' },
  { id: 'dingtalk', name: '钉钉', mark: '钉', color: 'sky' },
  { id: 'feishu', name: '飞书', mark: '飞', color: 'purple' },
  { id: 'telegram', name: 'Telegram（电报）', mark: 'TG', color: 'sky' },
  { id: 'email', name: '邮箱', mark: '@', color: 'orange' },
  { id: 'serverchan', name: 'Server 酱', mark: 'S', color: 'green' },
  { id: 'webhook', name: 'Webhook', mark: '{}', color: 'slate' }
];
export const typeInfo = (t) => channelTypes.find((c) => c.id === t) || channelTypes.at(-1);
export const sourceInfo = (t) => sourceTypes.find((s) => s.id === t) || sourceTypes[0];
export const newRule = () => ({
  id: uid('rule'),
  trigger: 'change',
  field: '',
  operator: 'gt',
  threshold: 130,
  cooldown: 30,
  every: 1,
  unit: 'hour',
  timing: 'always',
  windowStart: '09:00',
  windowEnd: '18:00',
  time: '09:00',
  selections: [],
  template: ''
});
export function defaults(person, config) {
  return (person?.methods || [])
    .filter((m) => m.default && config.channels.some((c) => c.id === m.channelId && c.enabled))
    .map((m) => person.id + ':' + m.id);
}
export function newMonitor(config) {
  const s = config.settings,
    r = newRule(),
    me = config.recipients.find((p) => p.name === '我自己') || config.recipients[0];
  r.selections = defaults(me, config);
  return {
    id: uid('monitor'),
    name: '',
    enabled: true,
    draft: false,
    needsReview: false,
    generation: 0,
    interval: 5,
    source: {
      type: 'api',
      url: '',
      method: 'GET',
      path: '',
      selector: '',
      attribute: '',
      timePath: '',
      unit: '',
      fields: [newField()],
      query: {},
      headers: {},
      body: ''
    },
    rules: [r],
    storage: {
      history: true,
      days: s.historyDays,
      maxRows: s.maxRows,
      logLevel: s.logLevel,
      logDays: s.logDays,
      stages: { fetch: true, evaluate: true, send: true }
    }
  };
}
export const newChannel = () => ({
  id: uid('channel'),
  name: '',
  type: 'wecom',
  enabled: true,
  url: '',
  secret: '',
  host: '',
  port: 465,
  user: '',
  password: '',
  sender: '',
  tls: 'tls',
  headers: {},
  method: 'POST',
  bodyTemplate: ''
});
export const newPerson = () => ({
  id: uid('person'),
  name: '',
  kind: 'person',
  note: '',
  methods: []
});
export function targets(config, keys) {
  return [...new Set(keys || [])].flatMap((key) => {
    const [pid, mid] = key.split(':'),
      p = config.recipients.find((p) => p.id === pid),
      m = p?.methods.find((m) => m.id === mid),
      c = config.channels.find((c) => c.id === m?.channelId && c.enabled);
    return p && m && c ? [{ key, person: p, method: m, channel: c }] : [];
  });
}
export function destination(config, r) {
  const groups = new Map();
  for (const t of targets(config, r.selections)) {
    if (!groups.has(t.person.name)) groups.set(t.person.name, []);
    groups
      .get(t.person.name)
      .push(
        t.channel.name.startsWith(t.person.name + ' · ')
          ? t.channel.name.slice(t.person.name.length + 3)
          : t.channel.name
      );
  }
  return (
    [...groups].map(([name, ways]) => name + ' · ' + ways.join('、')).join('；') ||
    '尚未选择通知方式'
  );
}
export function ruleText(r) {
  if (r.trigger === 'periodic')
    return (
      '每 ' +
      r.every +
      ' ' +
      ({ minute: '分钟', hour: '小时', day: '天' }[r.unit] || '') +
      '发送一次' +
      (r.timing === 'window' ? '（' + r.windowStart + '–' + r.windowEnd + '）' : '')
    );
  if (r.trigger === 'scheduled') return '每天 ' + r.time + ' 发送最新结果';
  if (r.trigger === 'threshold')
    return (r.operator === 'gt' ? '向上' : '向下') + '越过 ' + r.threshold + ' 时通知';
  return r.trigger === 'new' ? '发现新条目时通知' : '数据变化时通知';
}
export const timed = (r) => ['periodic', 'scheduled'].includes(r.trigger);
export const fmt = (at, zone = 'Asia/Shanghai') =>
  at
    ? new Intl.DateTimeFormat('zh-CN', {
        timeZone: zone,
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      }).format(at)
    : '暂无记录';
export const size = (b) =>
  b >= 1048576 ? (b / 1048576).toFixed(1) + ' MB' : (b / 1024).toFixed(1) + ' KB';
export const estimate = (m) =>
  m.storage.history && m.interval > 0
    ? Math.min(m.storage.maxRows, Math.ceil((m.storage.days * 1440) / m.interval))
    : 0;
export const stageNames = { fetch: '采集与提取', evaluate: '规则判断', send: '消息发送' };
export function parsePairs(text, separator = ':') {
  const result = {};
  for (const line of text.split('\n')) {
    if (!line.trim()) continue;
    const i = line.indexOf(separator);
    if (i < 1) throw Error('每行请用「名称' + separator + '值」填写。');
    const key = line.slice(0, i).trim();
    if (Object.hasOwn(result, key)) throw Error('名称重复：' + key);
    Object.defineProperty(result, key, {
      value: line.slice(i + 1).trim(),
      enumerable: true,
      writable: true,
      configurable: true
    });
  }
  return result;
}
export const pairsText = (v, sep = ': ') =>
  Object.entries(v || {})
    .map(([k, v]) => k + sep + v)
    .join('\n');
export function monitorStatus(m, rt, config) {
  if (m.draft) return 'draft';
  if (m.needsReview) return 'review';
  if (!m.enabled) return 'paused';
  if (
    m.rules.some((r) => !targets(config, r.selections).length) ||
    (rt?.incident && !rt.incident.resolved)
  )
    return 'attention';
  return 'normal';
}
export const statusLabels = {
  normal: '运行中',
  attention: '待处理',
  paused: '已暂停',
  draft: '草稿',
  review: '待检查'
};

export const newField = () => ({
  key: 'value',
  label: '数值',
  path: '',
  selector: '',
  attribute: '',
  unit: '',
  calculation: null
});
export const primaryUnit = (source) =>
  source.fields?.length ? source.fields[0].unit : source.unit;
export function enableFields(source) {
  if (!source.fields?.length)
    source.fields = [
      {
        ...newField(),
        path: source.path || '',
        selector: source.selector || '',
        attribute: source.attribute || '',
        unit: source.unit || ''
      }
    ];
}
export function addSourceField(source, path) {
  enableFields(source);
  if (source.fields.some((f) => f.path === path) || source.fields.length >= 16) return;
  let key = path
    .split('/')
    .at(-1)
    .replaceAll('~1', '_')
    .replaceAll('~0', '_')
    .replace(/[^A-Za-z0-9_]/g, '_');
  if (!/^[A-Za-z]/.test(key)) key = 'field_' + key;
  if (['constructor', 'prototype'].includes(key)) key = 'field_' + key;
  key = key.slice(0, 28) || 'field';
  const base = key;
  let n = 2;
  while (source.fields.some((f) => f.key === key)) key = base + '_' + n++;
  const blank = source.fields.length === 1 && source.fields[0].path === '';
  if (blank) Object.assign(source.fields[0], { key, label: key, path });
  else source.fields.push({ ...newField(), key, label: key, path });
}
export function messageTemplate(source) {
  const fields = source.fields || [];
  const lines = fields.length
    ? fields.map((f) => f.label + '：{{.Fields.' + f.key + '}} {{.Units.' + f.key + '}}')
    : ['当前值：{{.Value}}', '上次值：{{.Previous}}'];
  return ['{{.Title}}', ...lines, '采集时间：{{.CollectedTime}}', '来源：{{.SourceURL}}'].join(
    '\n'
  );
}
