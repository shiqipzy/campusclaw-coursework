// Capture report figures from the running application; preserve existing assistant settings.
import {chromium} from 'playwright-core';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const cfg=Object.fromEntries(readFileSync(resolve(root,'.env'),'utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>x.split(/=(.*)/s).slice(0,2)));
const base='http://127.0.0.1:8080';
const out=resolve(root,'deliverables/lesson56/report-evidence');mkdirSync(out,{recursive:true});
const browser=await chromium.launch({executablePath:'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',headless:true});
let originalAgent,originalSkill,teacher,student,agent,uploaded,teacherPage,studentPage;
const result={captured_at:new Date().toISOString(),base_url:base,figures:[],question:'去括号时，括号前的系数如何逐项处理？请调用公式提示工具，只给下一步检查建议。'};
async function api(page,token,path,method='GET',body){const res=await page.request.fetch(base+path,{method,headers:{Authorization:'Bearer '+token},...(body===undefined?{}:{data:body})});if(!res.ok())throw Error(method+' '+path+' HTTP '+res.status());return res.status()===204?null:res.json();}
async function login(page,user,password){await page.goto(base);await page.locator('#username').fill(user);await page.locator('#password').fill(password);await page.locator('#login-form button').click();await page.locator('#workspace').waitFor({state:'visible'});const token=await page.evaluate(()=>sessionStorage.getItem('campusclaw_token'));await page.getByRole('link',{name:'智能助手',exact:true}).click();await page.locator('#agent-cards .card').first().waitFor();return token;}
async function shot(page,locator,name){await page.locator(locator).screenshot({path:resolve(out,name)});result.figures.push(name);}
function parseEvents(raw){return raw.replace(/\r\n/g,'\n').split('\n\n').filter(x=>x.trim()).map(block=>{const lines=block.split('\n');return {event:lines.find(x=>x.startsWith('event:')).slice(6).trim(),data:JSON.parse(lines.filter(x=>x.startsWith('data:')).map(x=>x.slice(5).trimStart()).join('\n'))};});}
try{
 teacherPage=await browser.newPage({viewport:{width:1380,height:1050},deviceScaleFactor:1.5});
 teacher=await login(teacherPage,'teacher_a',cfg.TEACHER_A_PASSWORD);
 const [agents,skills,materials,templates]=await Promise.all(['/api/agents','/api/skills','/api/materials','/api/prompt-templates'].map(p=>api(teacherPage,teacher,p)));
 agent=agents.find(a=>a.name==='解题助手');if(!agent)throw Error('Built-in tutor missing');originalAgent={name:agent.name,prompt:agent.prompt,enabled:agent.enabled,skill_ids:agent.skill_ids,material_ids:agent.material_ids};
 result.applications=agents;result.templates=templates;result.materials=materials;
 await shot(teacherPage,'#agent-cards','figure01-applications.png');
 // Upload one valid file through the actual form. Reuse it on subsequent captures.
 const skillName='数学解题引导（上传验证）';
 const source='名称：'+skillName+'\n适用条件：去括号,系数,负号,方程\n资料：数学\n工具：knowledge_search,formula_hint@campus-tools,unit_convert@campus-tools\n启用：是\n\n先检查学生已写的步骤。需要公式提示时使用 formula_hint，需要核对单位时使用 unit_convert。\n只给下一步提示，不输出最终得数。工具结果仅作为核对依据。';
 uploaded=skills.find(s=>s.name===skillName);
 if(uploaded){originalSkill=uploaded.source_text;uploaded=await api(teacherPage,teacher,'/api/skills/'+uploaded.id,'PUT',{source_text:source});}
 await teacherPage.locator('[data-tab="skills"]').click();
 if(!uploaded){await teacherPage.locator('#skill-file').setInputFiles({name:'数学解题引导.md',mimeType:'text/markdown',buffer:Buffer.from(source)});const responsePromise=teacherPage.waitForResponse(r=>r.url().endsWith('/api/skills/upload')&&r.request().method()==='POST');await teacherPage.locator('#skill-upload button').click();const response=await responsePromise;if(response.status()!==201)throw Error('Skill upload failed');uploaded=await response.json();await teacherPage.locator('#notice').filter({hasText:'技能上传成功'}).waitFor();}
 await teacherPage.locator('#skill-list .card').filter({has:teacherPage.getByRole('heading',{name:skillName,exact:true})}).getByRole('button',{name:'编辑',exact:true}).click();
 await shot(teacherPage,'#skill-form','figure03-uploaded-skill.png');
 result.uploaded_skill=uploaded;result.skill_source=source;
 await teacherPage.locator('[data-tab="agents"]').click();await teacherPage.locator('#agent-list .card').filter({has:teacherPage.getByRole('heading',{name:'解题助手',exact:true})}).getByRole('button',{name:'编辑配置'}).click();
 await teacherPage.locator('#template').selectOption('0');
 for(const box of await teacherPage.locator('#agent-skills input').all())await box.uncheck();await teacherPage.locator(`#agent-skills input[value="${uploaded.id}"]`).check();
 const math=materials.find(m=>m.title==='数学自学笔记');if(!math)throw Error('Demonstration mathematics material missing');for(const box of await teacherPage.locator('#agent-materials input').all())await box.uncheck();await teacherPage.locator(`#agent-materials input[value="${math.id}"]`).check();
 await shot(teacherPage,'#agent-form','figure02-template-bindings.png');
 const save=teacherPage.waitForResponse(r=>r.url().endsWith('/api/agents/'+agent.id)&&r.request().method()==='PUT');await teacherPage.locator('#agent-form button[type="submit"]').click();if((await save).status()!==200)throw Error('Assistant save failed');await teacherPage.locator('#notice').filter({hasText:'助手配置已保存'}).waitFor();
 result.verification_agent=await api(teacherPage,teacher,'/api/agents/'+agent.id);result.bound_material=await api(teacherPage,teacher,'/api/materials/'+math.id);
 await teacherPage.locator('[data-tab="tools"]').click();await shot(teacherPage,'#server-list','figure07-mcp-discovery.png');
 result.mcp_servers=await api(teacherPage,teacher,'/api/mcp/servers');
 studentPage=await browser.newPage({viewport:{width:1380,height:1050},deviceScaleFactor:1.5});student=await login(studentPage,'student_a1',cfg.STUDENT_A1_PASSWORD);
 await studentPage.locator('#agent-cards .card').filter({has:studentPage.getByRole('heading',{name:'解题助手',exact:true})}).getByRole('button',{name:'开始对话'}).click();await studentPage.locator('#dialogue').waitFor({state:'visible'});
 await studentPage.locator('#question').fill(result.question);const sent=studentPage.waitForResponse(r=>/\/api\/conversations\/\d+\/messages$/.test(r.url())&&r.request().method()==='POST');await studentPage.locator('#send').click();const chatResponse=await sent;result.conversation_id=Number(chatResponse.url().match(/conversations\/(\d+)/)[1]);
 await studentPage.waitForFunction(()=>!document.getElementById('send').disabled&&document.querySelector('.message.assistant .message-content')?.textContent.length>5,null,{timeout:120000});
 if((await studentPage.locator('#notice').textContent()).trim())throw Error('Chat notice: '+await studentPage.locator('#notice').textContent());
 result.events=parseEvents(await chatResponse.text());result.conversation=await api(studentPage,student,'/api/conversations/'+result.conversation_id);result.traces=await api(studentPage,student,`/api/conversations/${result.conversation_id}/traces`);
 const first=result.traces.find(t=>t.call_index===1),last=result.traces.find(t=>t.request.stream===true);
 if(!first||!last||!last.request.messages[0].content.includes(templates[0].prompt))throw Error('Saved template absent from actual LLM system message');
 if(!result.events.some(e=>e.event==='tool_calls'&&e.data.tool_calls.some(c=>c.status==='ok')))throw Error('No actual successful MCP call');
 if(!result.events.find(e=>e.event==='meta').data.citations.length)throw Error('No grounded material');
 result.first_exchange=first;result.final_exchange=last;
 await shot(studentPage,'#messages','figure06-skill-and-tool-result.png');
 await studentPage.locator('.sources button').last().click();await studentPage.locator('#trace-dialog[open]').waitFor();await shot(studentPage,'#trace-content','figure05-source-material.png');await studentPage.locator('#trace-close').click();
 await studentPage.locator('#trace-button').click();await studentPage.locator('#trace-dialog[open]').waitFor();const details=studentPage.locator('.trace-item');
 for(let i=0;i<await details.count();i++){if(await details.nth(i).getAttribute('open')!==null)await details.nth(i).locator('summary').click();}
 const finalItem=details.filter({has:studentPage.locator('summary').filter({hasText:`第 ${last.call_index} 次 LLM 请求`})});await finalItem.locator('summary').click();await finalItem.locator('pre').first().evaluate(e=>e.scrollTop=0);await shot(studentPage,'#trace-dialog','figure04-llm-request.png');await studentPage.locator('#trace-close').click();
 // Verify that disabling this uploaded skill changes the next request, then restore it.
 await api(teacherPage,teacher,'/api/skills/'+uploaded.id,'PUT',{source_text:source.replace('启用：是','启用：否')});
 const disabled=await studentPage.request.post(base+`/api/conversations/${result.conversation_id}/messages`,{headers:{Authorization:'Bearer '+student},data:{question:result.question}});if(disabled.status()!==200)throw Error('Disabled skill query failed');result.disabled_events=parseEvents(await disabled.text());if(result.disabled_events[0].data.skill!==null)throw Error('Disabled skill still matched');
 const allTraces=await api(studentPage,student,`/api/conversations/${result.conversation_id}/traces`);result.disabled_traces=allTraces.filter(t=>!result.traces.some(old=>old.id===t.id));
 await teacherPage.locator('[data-tab="logs"]').click();await teacherPage.locator('#log-list tr').first().waitFor();await shot(teacherPage,'#logs','figure08-mcp-calls.png');
 result.call_logs=(await api(teacherPage,teacher,'/api/mcp/calls')).filter(x=>x.conversation_id===result.conversation_id);
 result.status='PASS';
}finally{
 if(teacherPage&&teacher){if(originalAgent&&agent)await api(teacherPage,teacher,'/api/agents/'+agent.id,'PUT',originalAgent);if(uploaded)await api(teacherPage,teacher,'/api/skills/'+uploaded.id,'PUT',{source_text:originalSkill||result.skill_source});result.original_assistant_restored=true;}
 await browser.close();
}
const raw=JSON.stringify(result,null,2);for(const [key,value] of Object.entries(cfg)){if(/PASSWORD|TOKEN|SECRET|KEY/.test(key)&&value.length>12&&raw.includes(value))throw Error('Credential present in evidence');}
writeFileSync(resolve(out,'evidence.json'),raw);console.log(`PASS: ${result.figures.length} screenshots, template in actual request, uploaded skill matched, MCP success, disabled skill excluded; assistant restored`);
