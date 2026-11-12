import { chromium } from 'playwright-core'
import { spawn } from 'node:child_process'
const [exe, bin, cfg, claudecfg, A, shots] = process.argv.slice(2)
const url = 'http://127.0.0.1:18470'
const svc = spawn(bin, ['--config', cfg], { stdio: 'ignore', env: { ...process.env, CLAUDE_CONFIG_DIR: claudecfg } })
for (let i = 0; i < 300; i++) { try { if ((await fetch(`${url}/api/workspace`)).ok) break } catch {} await new Promise((r) => setTimeout(r, 100)) }
const b = await chromium.launch({ executablePath: exe })
for (const scheme of ['light', 'dark']) {
  const page = await (await b.newContext({ viewport: { width: 1280, height: 700 }, colorScheme: scheme })).newPage()
  await page.goto(`${url}/#/c/${A}`)
  await page.locator('.earlier').waitFor()
  await page.waitForTimeout(500)
  await page.evaluate(() => { document.querySelector('.main').scrollTop = 0 })
  await page.waitForTimeout(200)
  await page.screenshot({ path: `${shots}/bar-${scheme}.png` })
}
await b.close()
svc.kill('SIGINT')
