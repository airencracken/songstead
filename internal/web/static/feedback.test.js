const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

test('successful saves are focused after navigation and swaps; edits clear stale confirmation', () => {
  const events = {}, statuses = [];
  let focused = 0, removed = 0;
  const status = { focus: () => { focused++; }, remove: () => { removed++; } };
  vm.runInNewContext(readFileSync(join(__dirname, 'feedback.js'), 'utf8'), {
    document: { addEventListener: (name, fn) => { events[name] = fn; }, querySelector: () => statuses[0] },
  });
  events.DOMContentLoaded(); assert.equal(focused, 0);
  statuses.push(status);
  events.DOMContentLoaded(); events['htmx:afterSettle'](); assert.equal(focused, 2);
  for (const name of ['input','change']) {
    events[name]({ target: { closest: () => ({ querySelectorAll: () => [status] }) } });
    events[name]({ target: { closest: () => null } });
  }
  assert.equal(removed, 2);
});
