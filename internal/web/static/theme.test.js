const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = readFileSync(join(__dirname, 'theme.js'), 'utf8');
function page(saved, blocked = false) {
  const attributes = new Map(), storage = new Map([['songstead-theme', saved]]);
  const events = {}, windowEvents = {}, label = { hidden: true };
  const control = { value: '', closest: () => label, matches: value => value === '[data-theme-select]' };
  vm.runInNewContext(source, {
    document: {
      documentElement: { setAttribute: (k,v) => attributes.set(k,v), removeAttribute: k => attributes.delete(k) },
      querySelectorAll: () => [control], addEventListener: (k,v) => { events[k] = v; },
    },
    window: { addEventListener: (k,v) => { windowEvents[k] = v; } },
    localStorage: {
      getItem: k => { if (blocked) throw Error('blocked'); return storage.get(k); },
      setItem: (k,v) => { if (blocked) throw Error('blocked'); storage.set(k,v); },
      removeItem: k => { if (blocked) throw Error('blocked'); storage.delete(k); },
    },
  });
  return { attributes, storage, events, windowEvents, control, label };
}
test('theme applies before rendering, survives swaps, and uses only recognized values', () => {
  for (const saved of ['light', 'dark', 'system', '<script>', undefined]) {
    const p = page(saved); const expected = ['light','dark'].includes(saved) ? saved : 'system';
    assert.equal(p.attributes.get('data-theme'), expected === 'system' ? undefined : expected);
    p.events.DOMContentLoaded(); assert.equal(p.control.value, expected); assert.equal(p.label.hidden, false);
    p.events['htmx:afterSwap'](); assert.equal(p.control.value, expected);
  }
});
test('changing and clearing a theme persists; cross-tab changes synchronize', () => {
  const p = page('light'); p.events.DOMContentLoaded();
  p.control.value = 'dark'; p.events.change({target:p.control}); assert.equal(p.storage.get('songstead-theme'), 'dark');
  p.control.value = 'system'; p.events.change({target:p.control}); assert.equal(p.storage.has('songstead-theme'), false);
  p.windowEvents.storage({key:'songstead-theme', newValue:'light'}); assert.equal(p.control.value, 'light');
  p.windowEvents.storage({key:'unrelated', newValue:'dark'}); assert.equal(p.control.value, 'light');
  p.windowEvents.storage({key:null, newValue:null}); assert.equal(p.control.value, 'system');
});
test('blocked storage preserves usable controls', () => {
  const p = page('light',true); p.events.DOMContentLoaded(); p.control.value='dark';
  p.events.change({target:p.control}); assert.equal(p.attributes.get('data-theme'),'dark');
});
