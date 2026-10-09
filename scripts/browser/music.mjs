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
  check(await bobby.locator('.list-artwork').count()===4,'List gives every music entry an artwork holder');
  check(await bobby.locator('.list-artwork img[src$="/thumbnail"]').count()===3,'List uses local cached thumbnails');
  for (const image of await bobby.locator('.list-artwork img').all()) await image.evaluate(img=>img.decode());
  check(await bobby.locator('.list-artwork').evaluateAll(holders=>holders.every(el=>{ const r=el.getBoundingClientRect(); return r.width===112 && r.height===84; })),'List covers stay compact');
  check(await bobby.locator('.artwork-placeholder').count()===1,'List has a missing-artwork fallback');
  check(await bobby.locator('.filter-grid:visible').count()===0,'Advanced dropdowns start collapsed');
  await bobby.getByText('More filters',{exact:true}).click();
  await bobby.getByRole('combobox',{name:'Music kind',exact:true}).selectOption('track');
  await bobby.getByRole('button',{name:'Apply filters',exact:true}).click();
  await bobby.waitForURL('**/recent?**kind=track**');
  await bobby.getByRole('heading',{name:'A quiet shelf.',exact:true}).waitFor();
  check(await bobby.locator('.list-artwork').count()===0,'Advanced kind filter excludes non-track links');
  await bobby.getByText('More filters',{exact:true}).click();
  await bobby.getByRole('combobox',{name:'Music kind',exact:true}).selectOption('');
  await bobby.getByRole('button',{name:'Apply filters',exact:true}).click();
  await bobby.locator('.list-artwork').first().waitFor();
  check(await bobby.locator('.list-artwork').count()===4,'Clearing advanced filters restores list thumbnails');
  check(new URL(bobby.url()).searchParams.get('discovery')==='preferences','Advanced filters preserve discovery mode');
  await bobby.getByRole('link',{name:'Tiles',exact:true}).click();
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
  await bobby.getByRole('link',{name:'Your settings',exact:true}).click();
  await bobby.waitForURL('**/account');
  check(await bobby.getByRole('heading',{name:'Your settings',exact:true}).count()===1,'Settings link leads to clearly named page');
  await bobby.getByRole('navigation',{name:'Your settings sections',exact:true}).getByRole('link',{name:'Genres & tags',exact:true}).click();
  check(new URL(bobby.url()).hash==='#discovery-preferences','Settings shortcuts reach the relevant section');
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
  await bobby.getByRole('link',{name:'All music',exact:true}).click();
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
  await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
  await bobby.getByRole('link',{name:'Acoustic',exact:true}).first().click();
  await bobby.waitForURL('**/recent?**tag=Acoustic**');
  check(await bobby.locator('.recommendations.tiles').count()===1,'Tag picker preserves tile layout');
  check(await bobby.locator('.recommendation-copy').count()===1,'Tag picker filters immediately');
  await bobby.getByRole('link',{name:'List',exact:true}).click();
  await bobby.locator('.recommendations.chips').waitFor();
  check(new URL(bobby.url()).searchParams.get('tag')==='Acoustic','Layout switch preserves tag filter');
  const native = await session('bobby',false);
  await native.goto(`${base}/recent?discovery=all`);
  await native.getByRole('link',{name:'Tiles',exact:true}).click();
  await native.locator('.recommendations.tiles').waitFor();
  check(await native.locator('.tile-artwork').count()===4,'Native layout switch needs no Browse click');
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

  // Only Songstead accounts are provisioned here. Local/Both must let these
  // members participate, while Witmoot mode requires its separate account.
  await bobby.goto(base+favorite);
  await bobby.getByRole('textbox',{name:'Your comment',exact:true}).fill('A conversation before changing modes');
  await bobby.getByRole('button',{name:'Add comment',exact:true}).click();
  await bobby.getByText('A conversation before changing modes',{exact:true}).waitFor();
  for (const mode of ['witmoot','both','songstead']) {
    await alice.goto(`${base}/admin/settings`);
    await alice.getByLabel('Public Songstead address').fill(base);
    await alice.getByLabel('Witmoot address').fill('https://boards.example.org/forum');
    await alice.getByRole('combobox',{name:'Where can people comment?',exact:true}).selectOption(mode);
    await alice.getByRole('button',{name:'Save settings',exact:true}).click();
    await alice.waitForURL('**/admin/settings?saved=1');
    for (const member of [bobby,native]) {
      await member.goto(base+favorite);
      check(await member.getByText('A conversation before changing modes',{exact:true}).count()===1,`Earlier comments remain readable in ${mode}`);
      check(await member.getByRole('button',{name:'Add comment',exact:true}).count()===(mode==='witmoot'?0:1),`Comment form follows ${mode} with JS ${member===bobby}`);
      check(await member.getByRole('button',{name:'Prepare a Witmoot discussion',exact:true}).count()===(mode==='songstead'?0:1),`Handoff follows ${mode} with JS ${member===bobby}`);
      if (mode==='witmoot') {
        check((await member.locator('main').innerText()).includes('Ask your host for an invitation'),'Witmoot-only mode explains access for members without accounts');
      }
    }
    if (mode!=='witmoot') {
      await native.getByRole('textbox',{name:'Your comment',exact:true}).fill(`Local participation in ${mode}`);
      await native.getByRole('button',{name:'Add comment',exact:true}).click();
      await native.getByText(`Local participation in ${mode}`,{exact:true}).waitFor();
      check(true,`No Witmoot account needed to comment in ${mode}`);
    } else {
      await bobby.getByRole('button',{name:'Prepare a Witmoot discussion',exact:true}).click();
      await bobby.getByRole('heading',{name:'A discussion, if you like',exact:true}).waitFor();
      check((await bobby.locator('main').innerText()).includes('A separate Witmoot account is required'),'Draft review explains separate identity');
      check((await bobby.getByRole('link',{name:'Continue to Witmoot',exact:true}).getAttribute('href')).startsWith('https://boards.example.org/forum/share?'),'Handoff respects the configured Witmoot path');
    }
  }

  // Deterministic preview responses exercise the real compose code without
  // relying on an external provider. The server fetcher has separate tests.
  const tinyPNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=','base64');
  await alice.route('**/recommendations/preview',async route => {
    await route.fulfill({contentType:'application/json',body:JSON.stringify({status:'ready',title:'Preview title <script>',artist:'Preview artist',artwork:tinyPNG.toString('base64')})});
  });
  await alice.goto(`${base}/recommendations/new`);
  await alice.getByLabel('Music link',{exact:true}).fill('https://youtu.be/dQw4w9WgXcQ');
  await alice.getByText('Music preview ready.',{exact:true}).waitFor();
  check(await alice.getByLabel('Title, if you know it',{exact:true}).inputValue()==='Preview title <script>','Preview auto-fills optional metadata as text');
  await alice.locator('[data-preview-artwork]').evaluate(img=>img.decode());
  check(await alice.locator('[data-preview-artwork]').evaluate(img=>img.naturalWidth===1),'Pasted link generates thumbnail before sharing');
  await alice.getByLabel('Title, if you know it',{exact:true}).fill('My own title');
  await alice.getByLabel('Music link',{exact:true}).fill('https://youtu.be/aaaaaaaaaaa');
  await alice.getByText('Music preview ready.',{exact:true}).waitFor();
  check(await alice.getByLabel('Title, if you know it',{exact:true}).inputValue()==='My own title','Preview preserves manual edits');
  check(await alice.locator('main script').count()===0,'Provider text never becomes executable HTML');
  await alice.unroute('**/recommendations/preview');
  await alice.route('**/recommendations/preview',async route => {
    const raw=new URLSearchParams(route.request().postData()).get('url');
    if (raw.includes('dQw4w9WgXcQ')) await new Promise(done=>setTimeout(done,1000));
    await route.fulfill({contentType:'application/json',body:JSON.stringify({status:'ready',title:raw.includes('dQw4w9WgXcQ')?'Stale preview':'Current preview',artist:'',artwork:tinyPNG.toString('base64')})}).catch(()=>{});
  });
  await alice.goto(`${base}/recommendations/new`);
  const first=alice.waitForRequest(request=>request.url().endsWith('/recommendations/preview'));
  await alice.getByLabel('Music link',{exact:true}).fill('https://youtu.be/dQw4w9WgXcQ');
  await first;
  await alice.getByLabel('Music link',{exact:true}).fill('https://youtu.be/aaaaaaaaaaa');
  await alice.getByText('Current preview',{exact:true}).waitFor();
  await alice.waitForTimeout(1200);
  check(await alice.getByLabel('Title, if you know it',{exact:true}).inputValue()==='Current preview','Late response cannot replace newer link preview');
  await alice.unroute('**/recommendations/preview');
  let polling=0;
  await bobby.route(`**${empty}/preview`,route => route.fulfill({contentType:'application/json',body:JSON.stringify({title:'Late evening sketches',artist:'Demo ensemble',image:++polling>1?`${favorite}/thumbnail`:'',pending:polling<2})}));
  await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
  await bobby.locator(`[data-music-card=\"${empty.split('/').at(-1)}\"] .tile-artwork img[src$=\"/thumbnail\"]`).waitFor();
  check(polling>=2,'Pending artwork appears without a full-page reload');
  polling=0;
  await bobby.goto(`${base}/recent?discovery=all`);
  await bobby.locator(`[data-music-card="${empty.split('/').at(-1)}"] .list-artwork img[src$="/thumbnail"]`).waitFor();
  check(polling>=2,'Pending artwork also refreshes in list view');
  await bobby.unroute(`**${empty}/preview`);
  const embedSeed=spawnSync('python3',['-c',`import sqlite3,sys
from pathlib import Path
with sqlite3.connect(Path(sys.argv[1])/'songstead.db') as db:
 db.execute("UPDATE media SET video_id='dQw4w9WgXcQ' WHERE title='Late evening sketches'")`,directory],{encoding:'utf8'});
  assert.equal(embedSeed.status,0,embedSeed.stderr);
  let playerReferer;
  await alice.route('https://www.youtube-nocookie.com/embed/**',async route => {
    playerReferer=(await route.request().allHeaders()).referer;
    await route.fulfill({contentType:'text/html',body:'<!doctype html><title>Player fixture</title>'});
  });
  await alice.goto(base+empty);
  await alice.getByText('Show YouTube player',{exact:true}).click();
  await alice.waitForFunction(()=>document.querySelector('.embed').open);
  for (let n=0;n<40 && !playerReferer;n++) await alice.waitForTimeout(100);
  check(playerReferer===base+'/','YouTube receives origin-only referrer even with no-referrer response header');
  check(await alice.getByRole('link',{name:'Open music link',exact:true}).getAttribute('target')==='_blank','Music links open a new tab');
  await alice.unroute('https://www.youtube-nocookie.com/embed/**');
  await alice.goto(`${base}/account`);
  const oneGIF=Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7','base64');
  const animatedGIF=Buffer.concat([oneGIF.subarray(0,19),oneGIF.subarray(19,-1),oneGIF.subarray(19,-1),Buffer.from([0x3b])]);
  await alice.getByLabel('Picture',{exact:true}).setInputFiles({name:'avatar.gif',mimeType:'image/gif',buffer:animatedGIF});
  await alice.getByRole('button',{name:'Save profile picture',exact:true}).click();
  await alice.waitForURL('**/account?saved=picture*');
  let picture=await bobby.request.get(`${base}/users/1/picture`);
  check(picture.headers()['content-type']==='image/gif','Animated account picture is preserved');
  await bobby.goto(`${base}/account`);
  await bobby.getByLabel('Play animated profile pictures',{exact:true}).uncheck();
  await bobby.getByRole('button',{name:'Save animation preference',exact:true}).click();
  await bobby.waitForURL('**/account?saved=animation*');
  picture=await bobby.request.get(`${base}/users/1/picture`);
  check(picture.headers()['content-type']==='image/png','Disabling animation serves the still image');
  const reduced=await alice.request.get(`${base}/users/1/picture?still=1`);
  check(reduced.headers()['content-type']==='image/png','Reduced-motion rendition remains available');
  await alice.emulateMedia({reducedMotion:'reduce'});
  await alice.reload();
  check(await alice.locator('#profile-picture .avatar img').evaluate(img=>img.currentSrc.endsWith('picture?still=1')),'Browser reduced-motion setting selects the still picture');
  await alice.emulateMedia({reducedMotion:'no-preference'});
  await alice.getByRole('button',{name:'Remove profile picture',exact:true}).click();
  await alice.waitForURL('**/account?saved=picture*');
  picture=await bobby.request.get(`${base}/users/1/picture`);
  check(picture.headers()['content-type']==='image/png','Removing the picture restores a still fallback');

  for (const [width,theme] of [[1280,'light'],[390,'dark']]) {
    await bobby.setViewportSize({width,height:1000});
    await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
    await bobby.getByLabel('Color theme',{exact:true}).selectOption(theme);
    for (const route of ['/recent?layout=tiles&discovery=all','/recent',favorite,'/account']) {
      await bobby.goto(base+route);
      check(await bobby.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`No overflow on ${route} at ${width}`);
      if (route==='/recent') {
        check(await bobby.locator('.list-artwork').evaluateAll(holders=>holders.every(el=>el.getBoundingClientRect().width===(innerWidth<=760?72:112))),`List covers adapt at ${width}`);
        await bobby.getByText('More filters',{exact:true}).click();
        check(await bobby.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`Expanded filters fit at ${width}`);
      }
      const result=await new AxeBuilder({page:bobby}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa']).analyze();
      assert.deepEqual(result.violations.map(v=>({id:v.id,nodes:v.nodes.length})),[],`Accessibility on ${route} at ${width}`); checks++;
    }
  }
  if (process.env.SONGSTEAD_SCREENSHOT_DIR) {
    await mkdir(process.env.SONGSTEAD_SCREENSHOT_DIR,{recursive:true});
    await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
    await bobby.screenshot({path:resolve(process.env.SONGSTEAD_SCREENSHOT_DIR,'recent-tiles-mobile.png'),fullPage:true});
    await bobby.goto(`${base}/recent?discovery=all`);
    await bobby.screenshot({path:resolve(process.env.SONGSTEAD_SCREENSHOT_DIR,'recent-list-mobile.png'),fullPage:true});
    await bobby.setViewportSize({width:1280,height:1000});
    await bobby.getByLabel('Color theme',{exact:true}).selectOption('light');
    await bobby.getByText('More filters',{exact:true}).click();
    await bobby.screenshot({path:resolve(process.env.SONGSTEAD_SCREENSHOT_DIR,'recent-list-desktop.png'),fullPage:true});
    await bobby.goto(`${base}/recent?layout=tiles&discovery=all`);
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
