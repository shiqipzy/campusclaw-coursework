import { chromium } from 'playwright-core';
import { readFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const env = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8').replace(/^\uFEFF/, '').split(/\r?\n/).filter(line => line.includes('=')).map(line => line.split(/=(.*)/s).slice(0, 2)));
const base = `http://127.0.0.1:${env.WEB_PORT || '8080'}`;
const browser = await chromium.launch({ executablePath: 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe', headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  let bearerSeen = false;
  page.on('request', request => {
    if (request.url().includes('/api/materials') && request.headers().authorization?.startsWith('Bearer ')) bearerSeen = true;
  });
  await page.goto(base);
  await page.locator('#username').fill('student_a1');
  await page.locator('#password').fill(env.STUDENT_A1_PASSWORD);
  await page.locator('#login-form button').click();
  await page.locator('#workspace').waitFor({ state: 'visible' });
  await page.locator('#ask-question').fill('A班示例材料是什么？');
  const answerResponse = page.waitForResponse(response => response.url().includes('/api/ask'));
  await page.locator('#ask-form button').click();
  const answer = await (await answerResponse).json();
  await page.locator('#ask-sources button').first().waitFor();
  if (!bearerSeen || answer.mode !== 'ai' || !answer.sources.length || !(await page.locator('#ask-answer').textContent()).includes('[1]')) throw new Error('Bearer request or grounded DeepSeek answer missing');
  const out = resolve(root, 'deliverables', 'screenshots');
  mkdirSync(out, { recursive: true });
  await page.screenshot({ path: resolve(out, '知识库问答-DeepSeek.png'), fullPage: true });
  console.log('PASS: browser Bearer request and sourced DeepSeek answer');
} finally {
  await browser.close();
}
