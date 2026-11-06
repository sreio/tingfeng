import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  addSourceField,
  barkServerOrigin,
  enableFields,
  messageTemplate,
  primaryUnit
} from './model.js';
import {
  defaults,
  targets,
  parsePairs,
  estimate,
  monitorStatus,
  newMonitor,
  newRule,
  ruleText
} from './model.js';
const config = {
  settings: { historyDays: 7, maxRows: 10000, logLevel: 'error', logDays: 3 },
  channels: [
    { id: 'ch', enabled: true },
    { id: 'disabled', enabled: false }
  ],
  recipients: [
    {
      id: 'me',
      name: '我自己',
      methods: [
        { id: 'one', channelId: 'ch', default: true },
        { id: 'two', channelId: 'disabled', default: true }
      ]
    }
  ]
};
test('Bark accepts a server domain and port; credentials and push URLs do not become server settings', () => {
  assert.equal(barkServerOrigin('Bark.example:8443'), 'https://bark.example:8443');
  assert.equal(barkServerOrigin('https://bark.example/'), 'https://bark.example');
  assert.equal(barkServerOrigin('http://localhost:8080'), 'http://localhost:8080');
  for (const value of [
    '',
    'bark.example/key',
    'bark.example?device_key=test',
    'https://user@bark.example',
    'ftp://bark.example',
    'bark.example#info'
  ]) {
    assert.equal(barkServerOrigin(value), '');
  }
});
test('recipient defaults, disabled channels and duplicate targets', () => {
  assert.deepEqual(defaults(config.recipients[0], config), ['me:one']);
  assert.equal(targets(config, ['me:one', 'me:one', 'me:two', 'gone:one']).length, 1);
  const m = newMonitor(config);
  assert.deepEqual(m.rules[0].selections, ['me:one']);
  assert.equal(m.storage.history, true);
  assert.equal(estimate(m), 2016);
  m.storage.history = false;
  assert.equal(estimate(m), 0);
  assert.equal(monitorStatus(m, {}, config), 'normal');
  m.rules[0].selections = ['me:two'];
  assert.equal(monitorStatus(m, {}, config), 'attention');
});
test('header and parameter parsing preserves values without prototype mutation', () => {
  assert.deepEqual(parsePairs('Authorization: Bearer a:b\nX-Name: value'), {
    Authorization: 'Bearer a:b',
    'X-Name': 'value'
  });
  assert.equal(parsePairs('q=a=b', '=').q, 'a=b');
  assert.throws(() => parsePairs('broken'));
  assert.throws(() => parsePairs('x:1\nx:2'));
  const a = parsePairs('__proto__: safe');
  assert.equal(a.__proto__, 'safe');
  assert.equal({}.safe, undefined);
});
test('timed notifications show interval and window separately from sampling', () => {
  const r = newRule();
  r.trigger = 'periodic';
  r.every = 1;
  r.unit = 'hour';
  r.timing = 'window';
  assert.equal(ruleText(r), '每 1 小时发送一次（09:00–18:00）');
});

test('multiple response selections preserve existing fields and produce distinct safe variables', () => {
  const source = { path: '', unit: '', type: 'api' };
  addSourceField(source, '/pronoinfo/0/buyprice');
  addSourceField(source, '/pronoinfo/0/sellprice');
  addSourceField(source, '/pronoinfo/0/buyprice');
  addSourceField(source, '/another/buyprice');
  addSourceField(source, '/constructor');
  assert.deepEqual(
    source.fields.map((f) => f.key),
    ['buyprice', 'sellprice', 'buyprice_2', 'field_constructor']
  );
  source.fields[0].unit = '元/克';
  assert.equal(primaryUnit(source), '元/克');
  assert.match(messageTemplate(source), /Fields\.buyprice/);
  assert.match(messageTemplate(source), /Units\.sellprice/);
});
test('legacy single-field configuration upgrades only when explicitly selected', () => {
  const source = { path: 'data.price', unit: '元', selector: '', attribute: '' };
  enableFields(source);
  assert.equal(source.fields[0].path, 'data.price');
  assert.equal(source.fields[0].unit, '元');
  addSourceField(source, '/data/second');
  assert.equal(source.fields.length, 2);
  assert.equal(source.fields[0].key, 'value');
});
