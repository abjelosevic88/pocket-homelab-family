// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds a busy family):
//
//   DATA_DIR=/tmp/family-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME="Emir Kovač" PORT=18257 ./pocket-family
//   CDP=http://127.0.0.1:9336 BASE=http://127.0.0.1:18257 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18257'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1200) {
  await page.goto(base + path, { waitUntil: 'domcontentloaded' })
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name, full = false) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png', fullPage: full }) }
const firstLetter = page => page.evaluate(() => fetch('/api/v1/letters').then(r => r.json()).then(l => l.find(x => x.title.includes('Jahorina')).id))

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1800)
  let html = await page.content()
  assert.ok(html.includes("Who's picking up"), 'Today shows the runs')
  assert.ok(html.includes('need someone') || html.includes('needs someone'), 'runs nobody has taken show')
  await shot(page, 'desktop-today')
  await go(page, '/week')
  assert.ok((await page.content()).includes('Football training'), 'the week shows a weekly plan')
  await shot(page, 'desktop-week')
  await go(page, '/pickups')
  html = await page.content()
  assert.ok(html.includes('Every week') && html.includes("I'll do it"), 'pick-ups with plans and take buttons')
  await shot(page, 'desktop-pickups')
  await go(page, '/chores')
  assert.ok((await page.content()).includes('Stars this week'), 'stars show')
  await shot(page, 'desktop-chores')
  await go(page, '/shopping')
  assert.ok((await page.content()).includes("I'm at the shop"), 'shopping list')
  await shot(page, 'desktop-shopping')
  await go(page, '/school')
  assert.ok((await page.content()).includes('Jahorina'), 'letters show')
  await shot(page, 'desktop-school')
  await go(page, `/school/letters/${await firstLetter(page)}`)
  await shot(page, 'desktop-letter')
  await go(page, '/board')
  await shot(page, 'desktop-board')
  await go(page, '/family')
  assert.ok((await page.content()).includes('What everyone should know'), 'family info')
  await shot(page, 'desktop-family')
  await go(page, '/family/sheet')
  await shot(page, 'desktop-sheet')
  await go(page, '/settings/apps')
  await shot(page, 'desktop-settings-apps')
  await page.goto(base + '/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1200)
  await page.keyboard.press('l')
  await page.waitForTimeout(500)
  assert.ok(await page.locator('.modal').count() > 0, 'L opens the letter dialog')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/', 1800)
  await shot(page, 'desktop-today-dark')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1800)
  await shot(page, 'phone-today')
  await go(page, '/pickups')
  await shot(page, 'phone-pickups')
  await go(page, '/shopping')
  await shot(page, 'phone-shopping')
  await go(page, '/chores')
  await shot(page, 'phone-chores')
  await go(page, '/week')
  await shot(page, 'phone-week')
  await go(page, '/family')
  await shot(page, 'phone-family')
  await go(page, '/')
  await page.locator('.fab').click()
  await page.waitForTimeout(400)
  await shot(page, 'phone-add')
  await page.keyboard.press('Escape')
  await page.locator('.topbar .btn.icon-only').first().click()
  await page.waitForTimeout(500)
  await shot(page, 'phone-menu')
  const wide = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
  assert.ok(!wide, 'nothing is wider than the phone')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
