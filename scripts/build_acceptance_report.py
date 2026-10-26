"""Build the Chinese stage-acceptance PDF from actual API evidence and UI screenshots."""
import json, html, textwrap, shutil
from pathlib import Path
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, Image, PageBreak, KeepTogether
from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.lib.utils import ImageReader
from pypdf import PdfReader

ROOT=Path(__file__).resolve().parents[1]
SOURCE=ROOT/'deliverables/lesson56'
OUT=ROOT/'output/pdf';OUT.mkdir(parents=True,exist_ok=True)
EVIDENCE=ROOT/'docs/evidence';EVIDENCE.mkdir(parents=True,exist_ok=True)
data=json.loads((SOURCE/'api-results.json').read_text(encoding='utf-8'))
ui=json.loads((SOURCE/'ui-results.json').read_text(encoding='utf-8'))
assert len(data['cases'])==26 and all(c['status']=='PASS' for c in data['cases']) and ui['status']=='PASS'
e=data['evidence'];traces=sorted(e['llm_traces'],key=lambda t:t['call_index']);first=traces[0];final=traces[-1]
repo='https://github.com/shiqipzy/campusclaw-coursework'

# Only demonstrative synthetic classroom content is published, never bearer headers.
(EVIDENCE/'llm-exchanges.json').write_text(json.dumps(traces,ensure_ascii=False,indent=2),encoding='utf-8')
summary={'timestamp':data['timestamp'],'environment':data['project'],'api_cases':data['cases'],'ui':ui,'round_limit':e['round_limit'],'cumulative_limit':e.get('cumulative_limit'),'mcp_direct':e['mcp_direct'],'isolation':e['isolation'],'tool_call_logs':e['call_logs'],'answers':{key:e[key]['answer'] for key in ['grounded_chat','ungrounded_chat','disabled_skill_chat','disconnected_chat','tool_error_chat']},'human_peer_review':'pending; automated separate-account verification passed'}
(EVIDENCE/'acceptance-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8')
for name in ['01-apps.png','04-mcp.png','05-call-logs.png','07-conversation.png']:
    shutil.copyfile(SOURCE/'screenshots'/name,EVIDENCE/name)

pdfmetrics.registerFont(TTFont('YaHei','C:/Windows/Fonts/msyh.ttc',subfontIndex=0))
pdfmetrics.registerFont(TTFont('YaHeiBold','C:/Windows/Fonts/msyhbd.ttc',subfontIndex=0))
green=colors.HexColor('#245c46');muted=colors.HexColor('#68796c');line=colors.HexColor('#dce5d7');cream=colors.HexColor('#f3f6ef')
styles={
 'title':ParagraphStyle('title',fontName='YaHeiBold',fontSize=25,leading=36,textColor=green,spaceAfter=14),
 'h1':ParagraphStyle('h1',fontName='YaHeiBold',fontSize=17,leading=26,textColor=green,spaceAfter=14,keepWithNext=True),
 'h2':ParagraphStyle('h2',fontName='YaHeiBold',fontSize=11,leading=19,textColor=green,spaceBefore=10,spaceAfter=5,keepWithNext=True),
 'body':ParagraphStyle('body',fontName='YaHei',fontSize=9,leading=16.5,spaceAfter=7,textColor=colors.HexColor('#293d31'),wordWrap='CJK'),
 'small':ParagraphStyle('small',fontName='YaHei',fontSize=7.6,leading=12,textColor=muted,spaceAfter=6,wordWrap='CJK'),
 'cell':ParagraphStyle('cell',fontName='YaHei',fontSize=8,leading=13,textColor=colors.HexColor('#293d31'),wordWrap='CJK'),
 'code':ParagraphStyle('code',fontName='YaHei',fontSize=7.3,leading=11,textColor=colors.HexColor('#294534'),wordWrap='CJK'),
}
story=[]
def p(text,kind='body'):
    story.append(Paragraph(html.escape(str(text)).replace('\n','<br/>'),styles[kind]))
def h(text):p(text,'h2')
def page(title,number):
    while story and isinstance(story[-1],Spacer):story.pop()
    if story:story.append(PageBreak())
    p(f'VERIFICATION / {number:02d}','small');p(title,'h1')
def table(rows,widths=None,dense=False):
    cells=[[Paragraph(html.escape(str(c)).replace('\n','<br/>'),styles['cell']) for c in row] for row in rows]
    t=Table(cells,colWidths=widths or [150,345],repeatRows=1,hAlign='LEFT')
    pad=4 if dense else 7
    t.setStyle(TableStyle([('BACKGROUND',(0,0),(-1,0),cream),('LINEBELOW',(0,0),(-1,0),.7,line),('LINEBELOW',(0,1),(-1,-1),.35,line),('VALIGN',(0,0),(-1,-1),'TOP'),('LEFTPADDING',(0,0),(-1,-1),9),('RIGHTPADDING',(0,0),(-1,-1),9),('TOPPADDING',(0,0),(-1,-1),pad),('BOTTOMPADDING',(0,0),(-1,-1),pad)]));story.append(t);story.append(Spacer(1,8))
def code(text):
    if not isinstance(text,str):
        text=json.dumps(text,ensure_ascii=False,indent=2)
        import re
        text=re.sub(r'\[\s*((?:"[^"\n]*"|-?\d+(?:\.\d+)?)(?:,\s*(?:"[^"\n]*"|-?\d+(?:\.\d+)?))*)\s*\]',lambda m:'['+', '.join(x.strip() for x in m.group(1).split(','))+']',text)
    lines=[]
    for raw in text.splitlines():
        # Wrap long JSON string values on the page without dropping content.
        lines.extend(textwrap.wrap(raw,width=84,replace_whitespace=False,drop_whitespace=False) or [' '])
    cells=[[Paragraph(html.escape(x).replace(' ','&#160;'),styles['code'])] for x in lines]
    panel=Table(cells,colWidths=[495]);panel.setStyle(TableStyle([('BACKGROUND',(0,0),(-1,-1),cream),('BOX',(0,0),(-1,-1),.5,line),('LEFTPADDING',(0,0),(-1,-1),12),('RIGHTPADDING',(0,0),(-1,-1),12),('TOPPADDING',(0,0),(-1,-1),0),('BOTTOMPADDING',(0,0),(-1,-1),0)]));story.append(panel);story.append(Spacer(1,8))
def screenshot(name,caption,max_height=340):
    path=SOURCE/'screenshots'/name;w,hh=ImageReader(str(path)).getSize();scale=min(495/w,max_height/hh)
    story.append(Image(str(path),width=w*scale,height=hh*scale,hAlign='CENTER'));p(caption,'small')

p('CAMPUSCLAW / STAGE ACCEPTANCE','small');p('个人项目阶段性验收报告','title')
p('第 5 课：提示词、AI 技能与流式会话\n第 6 课：MCP 与工具接入','h2')
p('仓库作者：shiqipzy  |  验证日期：2026-10-27（北京时间）','small')
p('Git 仓库：'+repo)
p('本项目在已有 CampusClaw 上继续改进，保留班级材料、Bearer JWT 和可追溯检索，补齐教学助手平台。教师配置多个助手、提示词、技能和工具，学生在自己的会话中提问并查看引用。')
table([['阶段验收要求','本次交付'],['多个 AI 应用','解题助手、作业诊断助手、学习计划助手；同一用户可选择不同助手'],['提示词模板 / 知识库','3份内置模板，可编辑角色提示词；限定班级及绑定材料检索'],['内置及上传 Skill','3项内置技能，支持 .md/.txt 上传、编辑、启停与绑定'],['外部工具 / MCP','独立容器提供 formula_hint、unit_convert，模型选择，宿主执行'],['验证材料','26个接口脚本全通过；9张实际截图；真实 LLM 请求、SSE 和3类调用记录']],[125,370])
screenshot('01-apps.png','图1 真实运行的同一 CampusClaw 学习工作台。下方额外助手由独立验收用例创建。',210)

page('验证 1：每次对话传给 LLM 什么？',1)
p('结论：会传递角色提示词；教师套用模板后，模板文字成为智能体 prompt，下一轮作为 system 内容发出。不会额外发送模板列表或模板名称。普通作答与工具决策采用不同的请求参数。')
table([['字段','本次实际值与含义'],['请求端点','POST https://api.deepseek.com/chat/completions'],['model / max_tokens','deepseek-flash / 1200'],['thinking','{"type":"disabled"}'],['messages','system + 最近至多6条历史（每条500字）+ 当前问题及检索资料；调用工具后追加 assistant.tool_calls 和 role=tool'],['stream','工具决策 false；最终正文 true（读取上游真正的 SSE，不是生成后模拟流式）'],['tools / tool_choice','仅工具决策请求携带，来自本次选中技能的已发现工具；tool_choice=auto'],['鉴权','HTTP Authorization: Bearer <服务端 AI_API_KEY>。原文不写入数据库证据或前端。'],['未设置的字段','本链路没有显式设置 temperature、top_p、seed。']],[126,369])
h('真实首轮返回（节选，非模拟）')
response=first['response']['body']
code({'http_status':first['response']['http_status'],'model':response['model'],'finish_reason':response['choices'][0]['finish_reason'],'message':response['choices'][0]['message'],'usage':{k:response['usage'][k] for k in ['prompt_tokens','completion_tokens','total_tokens']}})

page('验证 2：绑定知识库后会全量传送吗？',2)
p('结论：不会发送整个知识库。每次从当前会话锁定的智能体绑定材料中检索，班级取自服务端身份。先以 MySQL ngram 全文索引排序，再作词面相关性与技能资料范围过滤，最多保留4个切片，每片最多800个Unicode字符。')
table([['信息','本次实际观察'],['知识库存储','原材料、正文及约800字/重叠80字的切片存于MySQL，不随每次请求全量读取给模型'],['当前资料命中',f"{len(e['grounded_chat']['events'][0]['data']['citations'])}个切片；标题、切片号和字符区间随引用返回"],['权限与绑定','SQL同时限定class_id和tutor_agent_materials；请求体伪造class_id/agent_id不改变范围'],['无命中','仍按角色调用真实模型，引用为空；流式输出移除没有证据支持的[n]编号'],['检索能力边界','沿用第4课的词面全文检索，没有实现向量/混合检索；同义改写的召回能力有限']],[126,369])
h('本轮实际 user 消息（完整）')
code(first['request']['messages'][-1]['content'])
h('没有命中知识库时的真实回答')
p(e['ungrounded_chat']['answer'])
p('自动化证据：B班用户传class_id=1搜索A班专有词“蓝杉数学”，结果仍为空。引用仅包含当前助手绑定的材料；引用内容不放入system。')

page('验证 3、4：Skill 传哪些内容，怎样执行？',3)
p('结论：保存时整份技能文件入库；每次请求只从智能体绑定且启用的技能中选择至多一项。被选中技能的名称和完整“说明”正文进入system；未命中、停用、未绑定技能均不进入该轮system。文件原文、全部元数据、所有技能列表不会一并发送。')
table([['处理阶段','真实执行逻辑'],['解析与保存','中文元数据 + 空行 + 说明；名称、适用条件、启用必填；拒绝重复/未知字段与非法工具引用；原文保留'],['路由','提问和适用条件转小写；中英文逗号、顿号、分号、竖线切词；每命中1词得1分；最高分胜出，同分取ID较小者；0分不选'],['技能生效','按选中说明和边界组织system；名称含“解题引导”时追加不输出最终答案、选项、得数或完整解法的约束'],['资料与工具','资料字段筛选材料标题；工具字段划定候选集；实际调用由模型决定、宿主校验执行'],['执行边界','本项目技能是课程约定的配置文本，不运行上传文件中的shell、Python或其他代码，不等同Agent Skills标准目录'],['配置更新','会话只锁定智能体ID；技能启停、绑定与提示词下一轮重读，无须重建会话']],[114,381])
h('进入本轮 system 的选中技能说明')
selected=e['grounded_chat']['events'][0]['data']['skill'];p(selected['description'])
h('实际开关对照')
table([['状态','观察'],['启用时',f'命中“{selected["name"]}”；给下一步检查提示，并引用[1]'],['同一会话停用后','meta.skill=null；实际回答：'+e['disabled_skill_chat']['answer']]], [95,400])
p('“不直接给最终答案”依赖模型遵循提示词。本次真实题目通过；不能据此声称对所有题目都能形式化保证。','small')

page('验证 5：MCP 集成后，LLM 收到哪些参数？',4)
p('结论：模型收到候选工具的function名称、说明、输入JSON Schema；收不到MCP地址、服务器访问令牌，也不负责发送JSON-RPC报文。工具声明直接来自initialize后tools/list发现的清单。')
table([['映射','实际值'],['业务工具引用','formula_hint@campus-tools'],['发给模型的名称','formula_hint__campus_tools（另有unit_convert__campus_tools）'],['候选集','当前技能声明且当前服务器已连接、工具在发现清单中；未绑定技能中的工具不发送'],['参数校验','同一inputSchema既供模型填写参数，也由宿主JSON Schema验证器检查；工具服务再验证一次'],['模型选择结果',first['response']['body']['choices'][0]['message']['tool_calls'][0]['function']['arguments']]], [115,380])
h('实际发送给模型的第一个工具声明（完整）')
code(first['request']['tools'][0])
h('宿主向 MCP 发送的业务报文')
code({'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':'formula_hint','arguments':json.loads(first['response']['body']['choices'][0]['message']['tool_calls'][0]['function']['arguments'])}})
p('Base URL来自服务端配置http://mcp-demo:8090，访问令牌仅在宿主到MCP的Authorization请求头中出现。报告中的JSON没有鉴权头；服务端列表只回has_token=true。','small')

page('工具结果回传、流式输出与终止',5)
p('调用链：身份与会话校验 → 读取助手配置 → 路由技能 → 检索绑定材料 → meta → 模型工具决策 → 宿主校验执行 → tool_calls事件 → tool消息回传 → 最终真实SSE → 保存消息 → done。')
h('第二次 LLM 请求中的真实工具消息（完整）')
tool_messages=[m for m in final['request']['messages'] if m['role']=='tool'];code(tool_messages)
p('最终请求stream=true，不再携带tools。上游的content增量直接转换成浏览器delta；过滤无效引用后，所有delta拼接须与数据库assistant.content完全一致。')
h('真实最终回答')
p(e['grounded_chat']['answer'])
table([['限制 / 失败路径','已验证行为'],['轮次与次数','默认最多4轮、每会话累计12条可归属记录；验收栈设为1轮/2条。第1轮调用后limited=true；同一会话累计2条后继续提问不再调用，记录仍为2条，照常作答'],['断开后','清空发现清单；保存引用技能400。已有绑定明确产生denied记录，知识库与生成继续'],['发现后停机','实际停止验收栈的mcp-demo；本次调用error并入库；错误通过tool消息告知模型，仍返回正文'],['结果过滤','命中已知指令式句式整条丢弃；正常结果超过2000字截断；始终以tool数据角色回传'],['可归属记录','成功/被拒/出错均追加；服务被删除/改名而无归属时只在本轮SSE中显式拒绝，不新增无法归属记录']],[115,380])
p('说明：准备阶段发现未连接时，记录round=0；模型调用循环从round=1开始。累计次数限制以数据库可归属记录计数。','small')

page('实际运行截图：工具与对话',6)
screenshot('04-mcp.png','图2 教师端真实MCP发现清单、工具说明和输入结构；仅显示“已配置令牌”。',310)
screenshot('07-conversation.png','图3 学生本人会话。回答引用绑定材料，可打开原文，也可查看实际LLM请求详情。',310)

page('实际运行截图：三种调用结果',7)
screenshot('05-call-logs.png','图4 实际数据库中的成功、被拒和出错记录。包含工具引用、触发者、会话、轮次和时间。',340)
h('权限、令牌与持久化证据')
table([['检查','实际结果'],['A/B班隔离','B班教师的服务器与调用记录列表为空；按A班服务器ID直接访问与访问不存在ID的404内容完全一致'],['同班用户会话隔离','teacher_a不能读取student_a1的会话或LLM详情，同样返回404'],['学生写权限','上传材料/技能、创建助手、查看工具管理与调用记录均403'],['令牌不回传','整份服务器JSON中无MCP令牌原文、无token/token_cipher字段，只有has_token；密文只存服务端'],['证据一致性','接口脚本核对材料正文、切片、绑定、会话、消息、连接清单和调用记录；不只断言状态码']], [115,380])

page('26个接口脚本：执行结果',8)
p(f'执行环境：{data["project"]}，{data["base_url"]}；时间：{data["timestamp"]}（北京时间）。验收使用独立数据库和上传卷，未替换原有材料。26/26 PASS。')
rows=[['接口脚本（按URL组织）','结果 / 秒']]
for case in data['cases']:rows.append([case['case'].replace('cases\\','').replace('\\','/'),f'{case["status"]} / {case["seconds"]}'])
table(rows,[405,90],dense=True)
p('另已执行go test ./...、go vet ./...、OpenSpec严格校验；Edge检查模板套用、技能/MCP/审计页面、Swagger真实登录试调、本人会话和请求详情、390px手机布局。UI无脚本错误，保存9张实际截图。HTML测试报告按课程要求仅保留本地，不随作业提交。','small')

page('复现入口、课程核对与待办',9)
h('项目与启动')
p('Git仓库：'+repo)
code('复制 .env.example 为 .env，填写本机凭据\ndocker compose up --build -d\n浏览器：http://localhost:8080\n学习工作台：http://localhost:8080/assistant.html\nSwagger：http://localhost:8080/swagger/\npython scripts/setup_demo.py  # 幂等绑定数学演示材料和工具')
p('预置teacher_a、student_a1、student_b1；配置TEACHER_B_PASSWORD后增加teacher_b。所有口令均来自本机.env，报告和公开仓库不附真实口令。原材料页、检索和第四课单次问答继续可用。')
h('独立验收')
code('WEB_PORT=18081 MCP_MAX_ROUNDS=1 MCP_MAX_CALLS=2 \\\n  docker compose -p campusclaw_acceptance up --build -d\npython apitest/run_all.py\nnode scripts/lesson56_ui.mjs\ngo test ./...\ngo vet ./...\nnpx --yes @fission-ai/openspec validate add-skills-streaming-mcp --strict')
h('课程来源与对应关系')
for title,url in [('第5课任务清单','https://devops.hello1023.com/课程任务清单-第5课.html'),('第6课任务清单','https://devops.hello1023.com/课程任务清单-第6课.html'),('第5课实现教程','https://devops.hello1023.com/课件/第5课-课件-提示词AI技能与流式会话/index.html'),('第6课实现教程','https://devops.hello1023.com/课件/第6课-课件-MCP与工具接入/index.html')]:
    story.append(Paragraph(f'<link href="{html.escape(url,quote=True)}" color="#245c46">{title}</link>',styles['small']))
p('已按截图五项功能与五个验证问题逐项实现取证。课程第5/6课要求在原成果上增量实现，本次没有新建独立平台；MCP服务仍由同一工程Compose管理。')
h('必须由实际参与者补齐的内容')
p('真人同伴交叉验证尚未签认。自动化已分别使用A/B班教师与学生账号验证隔离、令牌不回传、断开拒绝和记录范围，但不冒充第二名真人操作。请同伴用本人账号在同一环境复核，并保留时间、账号范围、请求/响应及签名。')
p('本次未部署公网地址；原项目仍在本地运行。阶段验收截图要求提交Git地址与PDF，本报告均提供。若按学期最终验收，还需要公网部署与真人同伴记录。作业平台提交需由本人完成。','small')

page('附录：本轮消息原文与证据位置',10)
p('以下JSON保留真实首轮请求的顶层参数与messages原文，tools声明另见验证5及完整证据文件。完整两次请求、实际响应JSON及每条SSE数据帧在docs/evidence/llm-exchanges.json；该文件保留两项工具的完整说明与JSON Schema。')
compact={k:v for k,v in first['request'].items() if k!='tools'}
code(compact)
p('工具声明名称：'+', '.join(t['function']['name'] for t in first['request']['tools']))
h('证据文件索引')
table([['位置','内容'],['docs/evidence/llm-exchanges.json','真实完整LLM请求、首轮模型工具选择返回、最终流式SSE每帧'],['docs/evidence/acceptance-summary.json','26项用例结果、限制与隔离验证、工具直连返回、3类调用审计及真实回答'],['docs/evidence/*.png','4张实际运行截图；其余截图保留本地deliverables/lesson56/screenshots'],['apitest/cases/','26个接口脚本，按URL和HTTP方法组织'],['openspec/changes/add-skills-streaming-mcp/','本次增量的proposal、design、tasks和specs']],[180,315])

def footer(canvas,doc):
    w,h=doc.pagesize;canvas.setStrokeColor(line);canvas.line(50,42,w-50,42);canvas.setFont('YaHei',7);canvas.setFillColor(muted);canvas.drawString(50,29,'CampusClaw | 阶段性验收 | 2026-10-27');canvas.drawRightString(w-50,29,str(doc.page))
path=OUT/'CampusClaw阶段性验收报告.pdf'
doc=SimpleDocTemplate(str(path),pagesize=(595.28,841.89),rightMargin=50,leftMargin=50,topMargin=45,bottomMargin=55,title='CampusClaw个人项目阶段性验收报告',author='shiqipzy')
doc.build(story,onFirstPage=footer,onLaterPages=footer)
reader=PdfReader(str(path));assert len(reader.pages)>=10
print(f'Created {path}; {len(reader.pages)} pages')
