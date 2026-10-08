// SPDX-License-Identifier: AGPL-3.0-or-later
import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, rm, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { createServer } from 'node:net';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

const binary = process.env.SONGSTEAD_BINARY || fileURLToPath(new URL('../../bin/songstead', import.meta.url));
const directory = await mkdtemp(resolve(tmpdir(), 'songstead-browser-'));
const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('SONGSTEAD_')));
const password = 'a-browser-test-password';
let app, browser;
let checks = 0;
const check = (condition, message) => { assert.ok(condition, message); checks++; };
try {
  for (const [command, username] of [['create-owner', 'owner'], ['create-user', 'alice']]) {
    const result = spawnSync(binary, [command, '--data-dir', directory, '--username', username, '--password-stdin'],
      { input: `${password}\n`, env: environment, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
  }
  const listener = createServer();
  listener.listen(0, '127.0.0.1');
  await once(listener, 'listening');
  const port = listener.address().port;
  await new Promise((done, reject) => listener.close(error => error ? reject(error) : done()));
  const base = `http://127.0.0.1:${port}`;
  app = spawn(binary, ['serve', '--addr', `127.0.0.1:${port}`, '--data-dir', directory], { env: environment, stdio: ['ignore', 'ignore', 'pipe'] });
  let errors = '';
  app.stderr.on('data', chunk => { errors += chunk; });
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    if (app.exitCode !== null) throw new Error(`Server exited: ${errors}`);
    try { ready = (await fetch(`${base}/healthz`)).ok; } catch {}
    if (ready) break;
    await new Promise(done => setTimeout(done, 100));
  }
  check(ready, 'Server became healthy');
  browser = await chromium.launch({ executablePath: process.env.CHROMIUM || undefined });
  const ownerContext = await browser.newContext({ viewport: { width: 1280, height: 960 } });
  const page = await ownerContext.newPage();
  const failures = [];
  page.on('pageerror', error => failures.push(error.message));
  await page.goto(`${base}/login`);
  await page.getByLabel('Username', { exact: true }).fill('owner');
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL('**/shelf');
  await page.getByRole('link', { name: 'Admin', exact: true }).click();
  await page.waitForURL('**/admin/settings');
  await page.getByLabel('Site name', { exact: true }).fill('Music room');
  await page.getByLabel('Welcome heading', { exact: true }).fill('Welcome to the music room');
  await page.getByLabel('House rules', { exact: true }).fill('Be kind to your friends.');
  await page.getByLabel('Owner contact').fill('Contact your host for help.');
  await page.getByLabel('Public Songstead address').fill(base);
  await page.getByLabel('Witmoot address').fill('https://boards.example.org');
  await page.getByRole('button', { name: 'Save settings', exact: true }).click();
  await page.waitForURL('**/admin/settings?saved=1');
  check((await page.getByRole('status').innerText()).includes('already in effect'), 'Settings saved notice');
  for (const [width, theme] of [[1280, 'light'], [390, 'dark']]) {
    await page.setViewportSize({ width, height: 960 });
    await page.getByLabel('Color theme', { exact: true }).selectOption(theme);
    for (const route of ['/admin/settings', '/admin/users', '/invites', '/account', '/about']) {
      await page.goto(base + route);
      check(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `No horizontal overflow on ${route} at ${width}`);
      const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
      assert.deepEqual(result.violations.map(v => ({ id: v.id, nodes: v.nodes.length })), [], `Accessibility on ${route}`); checks++;
    }
  }
  await page.goto(`${base}/invites`);
  await page.getByLabel('Label', { exact: true }).fill('Browser guest');
  await page.getByRole('button', { name: 'Create invitation', exact: true }).click();
  const invite = await page.getByLabel('Private link', { exact: true }).inputValue();
  check(new URL(invite).origin === base, 'Invitation uses configured origin');
  const guestContext = await browser.newContext();
  const guest = await guestContext.newPage();
  await guest.goto(invite);
  await guest.getByLabel('Username', { exact: true }).fill('guest');
  await guest.getByLabel('New password', { exact: true }).fill(password);
  await guest.getByLabel('Confirm password', { exact: true }).fill(password);
  await guest.getByRole('button', { name: 'Create account', exact: true }).click();
  await guest.waitForURL('**/login?joined=1');
  await guest.getByLabel('Username', { exact: true }).fill('guest');
  await guest.getByLabel('Password', { exact: true }).fill(password);
  await guest.getByRole('button', { name: 'Sign in', exact: true }).click();
  await guest.waitForURL('**/shelf');
  check(await guest.getByRole('link', { name: 'Admin', exact: true }).count() === 0, 'Guest has no owner navigation');
  await guest.goto(invite);
  check((await guest.getByRole('alert').innerText()).includes('unavailable'), 'Used invite rejected');
  await page.goto(`${base}/admin/users`);
  const account = page.locator('section').filter({ has: page.getByRole('heading', { name: 'guest', exact: true }) });
  check((await account.innerText()).includes('Invited by owner'), 'Inviter attributed');
  await account.getByRole('button', { name: 'Create password recovery link', exact: true }).click();
  const recovery = await page.getByLabel('Private link', { exact: true }).inputValue();
  await guest.goto(recovery);
  await guest.getByLabel('New password', { exact: true }).fill('a-new-browser-password');
  await guest.getByLabel('Confirm password', { exact: true }).fill('a-new-browser-password');
  await guest.getByRole('button', { name: 'Reset password', exact: true }).click();
  await guest.waitForURL('**/login?reset=1');
  await guest.goto(recovery);
  check((await guest.getByRole('alert').innerText()).includes('unavailable'), 'Recovery is single use');
  await page.goto(`${base}/admin/settings`);
  await page.keyboard.press('Tab');
  check(await page.evaluate(() => document.activeElement?.tagName === 'A'), 'Skip link is keyboard reachable');
  if (process.env.SONGSTEAD_SCREENSHOT_DIR) {
    await mkdir(process.env.SONGSTEAD_SCREENSHOT_DIR, { recursive: true });
    await page.screenshot({ path: resolve(process.env.SONGSTEAD_SCREENSHOT_DIR, 'settings-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1280, height: 960 });
    await page.screenshot({ path: resolve(process.env.SONGSTEAD_SCREENSHOT_DIR, 'settings-desktop.png'), fullPage: true });
  }
  assert.deepEqual(failures, [], 'No browser runtime errors');
  console.log(`Passed ${checks} administration browser checks.`);
} finally {
  if (browser) await browser.close();
  if (app && app.exitCode === null) {
    app.kill('SIGTERM');
    await Promise.race([once(app, 'exit'), new Promise(done => setTimeout(done, 5000))]);
    if (app.exitCode === null) { app.kill('SIGKILL'); await once(app, 'exit'); }
  }
  await rm(directory, { recursive: true, force: true });
}
