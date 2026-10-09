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
const directory = await mkdtemp(resolve(tmpdir(), 'songstead-music-browser-'));
const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('SONGSTEAD_')));
const password = 'a-browser-test-password';
let app, browser, checks = 0;
const check = (condition, message) => { assert.ok(condition, message); checks++; };
const failures = [], external = [];
try {
  for (const [command, username] of [['create-owner','alice'], ['create-user','bobby']]) {
    const result = spawnSync(binary, [command,'--data-dir',directory,'--username',username,'--password-stdin'],
      { input: `${password}\n`, env: environment, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
  }
  const listener = createServer(); listener.listen(0,'127.0.0.1'); await once(listener,'listening');
  const port = listener.address().port;
  await new Promise((done,reject) => listener.close(error => error ? reject(error) : done()));
  const base = `http://127.0.0.1:${port}`;
  app = spawn(binary, ['serve','--addr',`127.0.0.1:${port}`,'--data-dir',directory], { env: environment, stdio:['ignore','ignore','pipe'] });
  let errors = ''; app.stderr.on('data',chunk => { errors += chunk; });
  let ready = false;
  for (let attempt=0; attempt<100; attempt++) {
    if (app.exitCode!==null) throw new Error(`Server exited: ${errors}`);
    try { ready=(await fetch(`${base}/healthz`)).ok; } catch {}
    if (ready) break;
    await new Promise(done => setTimeout(done,100));
  }
  check(ready,'Server became healthy');
  browser = await chromium.launch({ executablePath: process.env.CHROMIUM || undefined });
  async function session(username, javaScriptEnabled=true) {
    const context = await browser.newContext({ javaScriptEnabled, viewport:{width:1280,height:1000} });
    await context.route('**/*',route => {
      if (!route.request().url().startsWith(base)) { external.push(route.request().url()); return route.abort(); }
      return route.continue();
    });
    const page = await context.newPage(); page.on('pageerror',error => failures.push(error.message));
    await page.goto(`${base}/login`);
    await page.getByLabel('Username',{exact:true}).fill(username);
    await page.getByLabel('Password',{exact:true}).fill(password);
    await page.getByRole('button',{name:'Sign in',exact:true}).click();
    await page.waitForURL('**/shelf');
    return page;
  }
  const alice = await session('alice'), bobby = await session('bobby');
  async function share(title, genre, tags, audience='members', raw=title.toLowerCase().replaceAll(' ','-')) {
    await alice.goto(`${base}/recommendations/new`);
    await alice.getByLabel('Music link',{exact:true}).fill(`https://music.example/${raw}`);
    await alice.getByLabel('Title, if you know it',{exact:true}).fill(title);
    await alice.getByLabel('Artist',{exact:true}).fill('Demo ensemble');
    await alice.getByLabel('Genre',{exact:true}).fill(genre);
    await alice.getByLabel('Tags',{exact:true}).fill(tags);
    await alice.getByRole('combobox',{name:'Who can see this?',exact:true}).selectOption(audience);
    await alice.getByLabel('A note, if you like',{exact:true}).fill('Something for a quiet moment.');
    await alice.getByRole('button',{name:'Share recommendation',exact:true}).click();
    await alice.waitForURL(/\/recommendations\/\d+$/);
    return new URL(alice.url()).pathname;
  }
  const favorite = await share('Porchlight sessions','Jazz','Instrumental, warm');
  const excluded = await share('Heavy weather','Metal','Live');
  const plain = await share('Guitars after rain','Folk','Acoustic');
  const empty = await share('Late evening sketches','Ambient','Late-night');
  const privatePath = await share('Private music note','Private genre','Secret tag','person:2','porchlight-sessions');
  // Local, fictional artwork fixtures make image tests independent of provider
  // uptime. Production artwork goes through the tested fetch/normalization path.
  const seeded = spawnSync('python3',['-c',`
import sqlite3,struct,zlib,sys
from pathlib import Path
def chunk(kind,data):
    return struct.pack('!I',len(data))+kind+data+struct.pack('!I',zlib.crc32(kind+data)&0xffffffff)
def cover(colors):
    width=320; height=240
    rows=[]
    for y in range(height):
        row=bytearray()
        for x in range(width):
            band=((x//64)+(y//60))%len(colors)
            row.extend(colors[band])
        rows.append(b'\\0'+row)
    return b'\\x89PNG\\r\\n\\x1a\\n'+chunk(b'IHDR',struct.pack('!2I5B',width,height,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(b''.join(rows)))+chunk(b'IEND',b'')
with sqlite3.connect(Path(sys.argv[1])/'songstead.db') as db:
    for title,colors in [('Porchlight sessions',[(46,71,94),(213,177,125),(156,92,62)]),('Heavy weather',[(51,56,67),(109,126,147),(144,67,68)]),('Guitars after rain',[(51,82,68),(185,204,180),(139,148,115)])]:
        db.execute('INSERT INTO media_artwork(media_id,content) SELECT id,? FROM media WHERE title=?',(cover(colors),title))
`,directory],{encoding:'utf8'});
  assert.equal(seeded.status,0,seeded.stderr);
  await bobby.goto(`${base}/recent`);
  check(await bobby.locator('.recommendations.chips').count()===1,'Chips is the default');
  check(await bobby.locator('.tile-artwork').count()===0,'Default compact view omits large artwork');
  await bobby.getByRole('combobox',{name:'Layout',exact:true}).selectOption('tiles');
  await bobby.getByRole('button',{name:'Browse',exact:true}).click();
  await bobby.waitForURL('**/recent?**');
  await bobby.locator('.recommendations.tiles').waitFor();
  check(await bobby.locator('.tile-artwork').count()===4,'Tile view preserves all shared music');
  check(!(await bobby.locator('main').innerText()).includes('Private genre'),'Private labels stay outside Recent');
  check(await bobby.locator('.tile-artwork img[src$="/thumbnail"]').count()===3,'Tiles use local cached artwork');
  check(await bobby.locator('.artwork-placeholder').count()===1,'Missing artwork has a fallback');
  for (const image of await bobby.locator('.tile-artwork img[src$="/thumbnail"]').all()) {
    await image.scrollIntoViewIfNeeded();
    await image.evaluate(async element => { await element.decode(); });
  }
  check(await bobby.locator('.tile-artwork img[src$="/thumbnail"]').evaluateAll(images=>images.every(img=>img.complete && img.naturalWidth===320)),'Cached covers render');
  await bobby.getByRole('link',{name:'Account',exact:true}).click();
  await bobby.waitForURL('**/account');
  await bobby.getByRole('combobox',{name:'Spoiler preference',exact:true}).selectOption('spoiler-free');
  await bobby.getByRole('button',{name:'Save spoiler preference',exact:true}).click();
  await bobby.waitForURL('**/account?saved=annotations*');
  check((await bobby.getByRole('status').innerText()).includes('Spoiler preference saved'),'Account spoiler save confirmed');
  await bobby.getByLabel('Exclude genres',{exact:true}).fill('metal');
  await bobby.getByLabel('Prefer tags',{exact:true}).fill('instrumental');
  await bobby.getByRole('button',{name:'Save discovery preferences',exact:true}).click();
  await bobby.waitForURL('**/account?saved=discovery*');
  check((await bobby.getByRole('status').innerText()).includes('Discovery preferences saved'),'Discovery save confirmed');
  await bobby.goto(`${base}/recent?layout=tiles`);
  check(!(await bobby.locator('main').innerText()).includes('Heavy weather'),'Excluded genre disappears');
  check((await bobby.locator('.recommendations h3').first().innerText())==='Porchlight sessions','Preferred tag surfaces an older share');
  await bobby.getByRole('combobox',{name:'Discovery',exact:true}).selectOption('all');
  await bobby.getByRole('button',{name:'Browse',exact:true}).click();
  await bobby.waitForURL('**/recent?**');
  await bobby.getByRole('heading',{name:'Heavy weather',exact:true}).first().waitFor();
  check((await bobby.locator('.recommendations h3').first().innerText())==='Late evening sketches','All music restores chronology');
  await bobby.goto(base+favorite);
  check(await bobby.getByRole('combobox',{name:'Spoiler preference',exact:true}).inputValue()==='spoiler-free','Account preference applies on music pages');
  await bobby.getByRole('combobox',{name:'Listening status',exact:true}).selectOption('listened');
  await bobby.getByRole('combobox',{name:'What did you think?',exact:true}).selectOption('1');
  await bobby.getByRole('textbox',{name:'Personal notes',exact:true}).fill('Private listening thought');
  await bobby.getByRole('button',{name:'Save your reaction',exact:true}).click();
  await bobby.waitForURL(`**${favorite}?saved=feedback*`);
  await bobby.getByRole('status').waitFor();
  check((await bobby.getByRole('status').innerText())==='Listening feedback saved.','Listening save visibly confirmed');
  check(await bobby.getByRole('status').evaluate(el=>{ const r=el.getBoundingClientRect(); return r.top>=0 && r.bottom<=innerHeight; }),'Confirmation is in the viewport');
  await bobby.getByRole('textbox',{name:'Personal notes',exact:true}).fill('Unsaved edit');
  check(await bobby.getByRole('status').count()===0,'Editing clears stale save confirmation');
  await bobby.goto(base+favorite);
  check(await bobby.getByRole('textbox',{name:'Personal notes',exact:true}).inputValue()==='Private listening thought','Feedback persists without saving the draft');
  await alice.goto(base+favorite);
  check(await alice.getByRole('textbox',{name:'Personal notes',exact:true}).inputValue()==='','Private feedback is absent for sender');
  await alice.getByLabel('Genre',{exact:true}).fill('New jazz');
  await alice.getByRole('button',{name:'Save genre and tags',exact:true}).click();
  await alice.waitForURL(`**${favorite}?saved=labels*`);
  check((await alice.getByRole('status').innerText())==='Genre and tags saved.','Label edits confirmed');
  const native = await session('bobby',false);
  await native.goto(base+plain);
  await native.getByRole('textbox',{name:'Personal notes',exact:true}).fill('Saved without JavaScript');
  await native.getByRole('button',{name:'Save your reaction',exact:true}).click();
  await native.waitForURL(`**${plain}?saved=feedback*`);
  check((await native.getByRole('status').innerText())==='Listening feedback saved.','Native feedback confirmation works');
  await native.goto(`${base}/account`);
  await native.getByRole('combobox',{name:'Spoiler preference',exact:true}).selectOption('hidden');
  await native.getByRole('button',{name:'Save spoiler preference',exact:true}).click();
  await native.waitForURL('**/account?saved=annotations*');
  check(await native.getByRole('combobox',{name:'Spoiler preference',exact:true}).inputValue()==='hidden','Native account preference persists');
  for (const [width,theme] of [[1280,'light'],[390,'dark']]) {
    await bobby.setViewportSize({width,height:1000});
    await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
    await bobby.getByLabel('Color theme',{exact:true}).selectOption(theme);
    for (const route of ['/recent?layout=tiles&discovery=all','/recent',favorite,'/account']) {
      await bobby.goto(base+route);
      check(await bobby.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`No overflow on ${route} at ${width}`);
      const result=await new AxeBuilder({page:bobby}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa']).analyze();
      assert.deepEqual(result.violations.map(v=>({id:v.id,nodes:v.nodes.length})),[],`Accessibility on ${route} at ${width}`); checks++;
    }
  }
  if (process.env.SONGSTEAD_SCREENSHOT_DIR) {
    await mkdir(process.env.SONGSTEAD_SCREENSHOT_DIR,{recursive:true});
    await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
    await bobby.screenshot({path:resolve(process.env.SONGSTEAD_SCREENSHOT_DIR,'recent-tiles-mobile.png'),fullPage:true});
    await bobby.setViewportSize({width:1280,height:1000});
    await bobby.getByLabel('Color theme',{exact:true}).selectOption('light');
    await bobby.screenshot({path:resolve(process.env.SONGSTEAD_SCREENSHOT_DIR,'recent-tiles-desktop.png'),fullPage:true});
  }
  check(external.length===0,`Artwork makes no external browser requests: ${external}`);
  assert.deepEqual(failures,[],'No browser runtime errors');
  console.log(`Passed ${checks} music and preferences browser checks.`);
} finally {
  if (browser) await browser.close();
  if (app && app.exitCode===null) {
    app.kill('SIGTERM');
    await Promise.race([once(app,'exit'),new Promise(done=>setTimeout(done,5000))]);
    if (app.exitCode===null) { app.kill('SIGKILL'); await once(app,'exit'); }
  }
  await rm(directory,{recursive:true,force:true});
}
