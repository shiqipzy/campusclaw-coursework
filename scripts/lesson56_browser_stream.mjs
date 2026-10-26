import {chromium} from 'playwright-core';
import {readFileSync,writeFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {dirname,resolve} from 'node:path';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const cfg=Object.fromEntries(readFileSync(resolve(root,'.env'),'utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>x.split(/=(.*)/s).slice(0,2)));
const browser=await chromium.launch({executablePath:'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',headless:true});
try{
 const page=await browser.newPage({viewport:{width:1440,height:1000}});await page.goto('http://127.0.0.1:8080/');
 await page.locator('#username').fill('student_a1');await page.locator('#password').fill(cfg.STUDENT_A1_PASSWORD);await page.locator('#login-form button').click();await page.locator('#workspace').waitFor({state:'visible'});
 await page.getByRole('link',{name:'智能助手',exact:true}).click();const card=page.locator('#agent-cards .card').filter({has:page.getByRole('heading',{name:'解题助手',exact:true})});await card.getByRole('button',{name:'开始对话'}).click();await page.locator('#dialogue').waitFor({state:'visible'});
 await page.evaluate(()=>{window.streamUpdates=0;new MutationObserver(records=>{for(const record of records)if(record.target.parentElement?.closest('.message.assistant .message-content')||record.target.closest?.('.message.assistant .message-content'))window.streamUpdates++;}).observe(document.getElementById('messages'),{childList:true,characterData:true,subtree:true});});
 await page.locator('#question').fill('去括号时，括号前的系数如何逐项处理？请调用公式提示工具，只给下一步检查建议。');await page.locator('#send').click();await page.locator('#send').waitFor({state:'visible'});
 await page.waitForFunction(()=>!document.getElementById('send').disabled&&document.querySelector('.message.assistant .message-content')?.textContent.length>5,{timeout:90000});
 const updates=await page.evaluate(()=>window.streamUpdates);if(updates<2)throw Error('Browser did not render multiple stream updates');
 if((await page.locator('#notice').textContent()).trim())throw Error(await page.locator('#notice').textContent());
 if(!((await page.locator('.tool-events').textContent()).includes('成功')))throw Error('Live tool status missing');
 if(await page.locator('.sources button').count()<1)throw Error('Live citations missing');
 const result={status:'PASS',browser_text_updates:updates,main_url:'http://127.0.0.1:8080',actual_form_submission:true,tool_status:'ok',citations:true};
 const file=resolve(root,'deliverables/lesson56/ui-results.json');const ui=JSON.parse(readFileSync(file,'utf8'));ui.live_stream=result;writeFileSync(file,JSON.stringify(ui,null,2));
 await page.screenshot({path:resolve(root,'deliverables/lesson56/screenshots/10-main-live-stream.png')});
 console.log('PASS: main application browser form submitted, real MCP succeeded, citations shown, '+updates+' incremental DOM updates');
}finally{await browser.close();}
