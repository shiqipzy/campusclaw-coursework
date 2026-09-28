// Capture actual Edge pages from the local running Compose stack.
import { chromium } from 'playwright-core';
import { readFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const env = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8').replace(/^\uFEFF/, '').split(/\r?\n/).filter(line => line.includes('=')).map(line => line.split(/=(.*)/s).slice(0, 2)));
const base = `http://localhost:${env.WEB_PORT || '8080'}`;
const out = resolve(root, 'deliverables/screenshots');
mkdirSync(out, { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe', headless: true, args: ['--no-sandbox'] });

async function login(context, username, password) {
  const page = await context.newPage();
  await page.goto(base);
  await page.locator('#username').fill(username);
  await page.locator('#password').fill(password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.locator('#workspace').waitFor({ state: 'visible' });
  return page;
}
async function snap(page, filename) {
  await page.screenshot({ path: resolve(out, filename), fullPage: true });
  console.log(filename);
}

try {
  const teacherContext = await browser.newContext({ viewport: { width: 1365, height: 850 }, deviceScaleFactor: 1 });
  const loginPage = await teacherContext.newPage();
  await loginPage.goto(base);
  await snap(loginPage, '02-登录表单.png');
  await loginPage.close();
  const teacher = await login(teacherContext, 'teacher_a', env.TEACHER_A_PASSWORD);
  await teacher.getByText('A班示例', { exact: true }).first().waitFor();
  await snap(teacher, '01-教师材料页.png');

  await teacher.locator('#file').setInputFiles({ name: 'not-supported.pdf', mimeType: 'application/pdf', buffer: Buffer.from('not a real pdf') });
  await teacher.getByRole('button', { name: '上传并入库' }).click();
  await teacher.locator('#upload-message').getByText('only .txt and .md files are supported').waitFor();
  await snap(teacher, '04-不支持类型被拒绝.png');

  const teacherMe = await teacherContext.newPage();
  await teacherMe.goto(`${base}/api/me`);
  await snap(teacherMe, '03-浏览器访问me.png');

  const teacherList = await teacherContext.newPage();
  await teacherList.goto(`${base}/api/materials`);
  await snap(teacherList, '06-浏览器访问materials.png');

  const bContext = await browser.newContext({ viewport: { width: 1365, height: 850 } });
  await login(bContext, 'student_b1', env.STUDENT_B1_PASSWORD);
  const bList = await (await bContext.request.get(`${base}/api/materials`)).json();
  const bID = bList.find(item => item.filename === 'B班示例.txt').id;

  const aContext = await browser.newContext({ viewport: { width: 1365, height: 850 } });
  const student = await login(aContext, 'student_a1', env.STUDENT_A1_PASSWORD);
  await snap(student, '05-同班学生材料页.png');
  const cross = await aContext.newPage();
  await cross.goto(`${base}/api/materials/${bID}/file`);
  await snap(cross, '05-跨班下载404.png');

  const anonymous = await browser.newContext({ viewport: { width: 1365, height: 850 } });
  const denied = await anonymous.newPage();
  await denied.goto(`${base}/api/materials/${bID}/file`);
  await snap(denied, '05-未登录下载401.png');
} finally {
  await browser.close();
}
