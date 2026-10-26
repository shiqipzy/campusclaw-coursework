"""Coursework report: five assignment questions, measured JSON and eight application figures."""
from pathlib import Path
from html import escape
import json
import shutil
import re
from datetime import datetime, timezone, timedelta

from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.utils import ImageReader
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Image, Flowable, KeepTogether, PageBreak
from pypdf import PdfReader

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'deliverables' / 'lesson56' / 'report-evidence'
EVIDENCE = ROOT / 'docs' / 'evidence' / 'report'
OUTPUT = ROOT.parent / 'output' / 'pdf'
PDF = OUTPUT / 'CampusClaw个人项目阶段性验收报告.pdf'
EVIDENCE.mkdir(parents=True, exist_ok=True)
OUTPUT.mkdir(parents=True, exist_ok=True)
e = json.loads((SOURCE / 'evidence.json').read_text(encoding='utf-8'))
checks = json.loads((ROOT / 'deliverables/lesson56/api-results.json').read_text(encoding='utf-8'))
assert e['status'] == 'PASS' and e['original_assistant_restored']
assert len(e['figures']) == 8 and all(c['status'] == 'PASS' for c in checks['cases'])
for name in e['figures']:
    shutil.copyfile(SOURCE / name, EVIDENCE / name)
shutil.copyfile(SOURCE / 'evidence.json', EVIDENCE / 'evidence.json')
first, final = e['first_exchange'], e['final_exchange']
first_response = first['response']['body']
model_call = first_response['choices'][0]['message']['tool_calls'][0]
meta = next(item['data'] for item in e['events'] if item['event'] == 'meta')
last_meta = [item['data'] for item in e['events'] if item['event'] == 'meta'][-1]
answer = e['conversation']['messages'][-1]['content']
skill = meta['skill']
disabled = e['disabled_events'][0]['data']
local_time = datetime.fromisoformat(e['captured_at'].replace('Z', '+00:00')).astimezone(timezone(timedelta(hours=8)))

pdfmetrics.registerFont(TTFont('Song', 'C:/Windows/Fonts/simsun.ttc', subfontIndex=0))
pdfmetrics.registerFont(TTFont('Hei', 'C:/Windows/Fonts/simhei.ttf'))
pdfmetrics.registerFontFamily('Song', normal='Song', bold='Hei', italic='Song', boldItalic='Hei')
WIDTH = A4[0] - 120
BODY = ParagraphStyle('body', fontName='Song', fontSize=12, leading=21,
                      firstLineIndent=24, rightIndent=6, wordWrap='CJK', spaceAfter=6,
                      allowWidows=0, allowOrphans=0)
HEADING = ParagraphStyle('heading', fontName='Hei', fontSize=13, leading=22,
                         spaceBefore=11, spaceAfter=8, keepWithNext=True, wordWrap='CJK')
SUB = ParagraphStyle('sub', fontName='Hei', fontSize=11.5, leading=20,
                     spaceBefore=7, spaceAfter=5, keepWithNext=True, wordWrap='CJK')
TITLE = ParagraphStyle('title', fontName='Hei', fontSize=17, leading=26,
                       alignment=1, spaceAfter=18)
CODE = ParagraphStyle('code', fontName='Song', fontSize=10, leading=14,
                      leftIndent=14, rightIndent=14, wordWrap='CJK', spaceAfter=10,
                      allowWidows=0, allowOrphans=0)
CAPTION = ParagraphStyle('caption', fontName='Song', fontSize=10.5, leading=16,
                         alignment=1, spaceBefore=6, spaceAfter=10, wordWrap='CJK')
story, markdown = [], []
figures = []

def p(text, together=False):
    item = Paragraph(escape(str(text)), BODY)
    story.append(KeepTogether([item]) if together else item)
    markdown.append(str(text) + '\n')

def h(text, new_page=False):
    if new_page:
        story.append(PageBreak())
    story.append(Paragraph(escape(text), HEADING))
    markdown.append('## ' + text + '\n')

def sub(text):
    story.append(Paragraph(escape(text), SUB))
    markdown.append('### ' + text + '\n')

def compact_json(value):
    text = json.dumps(value, ensure_ascii=False, indent=2)
    pattern = r'\[\s*((?:"[^"\n]*"|-?\d+(?:\.\d+)?)(?:,\s*(?:"[^"\n]*"|-?\d+(?:\.\d+)?))*)\s*\]'
    return re.sub(pattern, lambda m: '[' + ', '.join(x.strip() for x in m.group(1).split(',')) + ']', text)

def code(value, language='json'):
    text = value if isinstance(value, str) else compact_json(value)
    html = escape(text).replace(' ', '&nbsp;').replace('\n', '<br/>')
    block = [Paragraph(html, CODE)]
    if story and isinstance(story[-1], Paragraph) and story[-1].style.name == 'body':
        block.insert(0, story.pop())
    if story and isinstance(story[-1], Paragraph) and story[-1].style.name in ['heading', 'sub']:
        block.insert(0, story.pop())
    story.append(KeepTogether(block))
    markdown.append('```' + language + '\n' + text + '\n```\n')

class CroppedScreenshot(Flowable):
    def __init__(self, path, crop, maximum):
        super().__init__()
        self.path, self.crop = path, crop
        x0, y0, x1, y1 = crop
        self.scale = min(WIDTH / (x1-x0), maximum / (y1-y0))
        self.width, self.height = (x1-x0)*self.scale, (y1-y0)*self.scale
    def draw(self):
        x0, y0, x1, y1 = self.crop
        iw, ih = ImageReader(str(self.path)).getSize()
        self.canv.saveState()
        clip = self.canv.beginPath()
        clip.rect(0, 0, self.width, self.height)
        self.canv.clipPath(clip, stroke=0, fill=0)
        self.canv.drawImage(str(self.path), -x0*self.scale, -(ih-y1)*self.scale,
                           width=iw*self.scale, height=ih*self.scale)
        self.canv.restoreState()

def figure(name, caption, maximum=320, crop=None):
    path = EVIDENCE / name
    iw, ih = ImageReader(str(path)).getSize()
    scale = min(WIDTH / iw, maximum / ih)
    number = len(figures) + 1
    label = f'图{number}  {caption}'
    image = CroppedScreenshot(path, crop, maximum) if crop else Image(str(path), width=iw * scale, height=ih * scale, hAlign='CENTER')
    block = [Spacer(1, 4), image, Paragraph(escape(label), CAPTION)]
    if story and isinstance(story[-1], Paragraph) and story[-1].style.name == 'body':
        block.insert(0, story.pop())
    if story and isinstance(story[-1], Paragraph) and story[-1].style.name in ['heading', 'sub']:
        block.insert(0, story.pop())
    story.append(KeepTogether(block))
    markdown.append(f'![{label}](evidence/report/{name})\n')
    figures.append({'number': number, 'filename': name, 'caption': caption})

TITLE_TEXT = 'CampusClaw 个人项目阶段性验收报告'
story.append(Paragraph(TITLE_TEXT, TITLE))
markdown.append('# ' + TITLE_TEXT + '\n')

h('一、项目简介与功能展示')
p('本项目在原有 CampusClaw 班级材料与知识库问答系统上继续开发，主要完成第5课的提示词、技能和流式会话，以及第6课的 MCP 工具接入。前端使用原生 HTML、CSS 和 JavaScript，后端使用 Go，数据保存在 MySQL 中。材料检索、身份认证和班级权限沿用原项目，新增功能仍在同一套系统中运行。')
p('项目 Git 仓库地址为 https://github.com/shiqipzy/campusclaw-coursework。系统内置解题助手、作业诊断助手和学习计划助手，分别配置角色提示词、技能和材料；教师可以套用提示词模板，上传或编辑技能文件，再将知识库及外部工具接入助手。图1展示三个内置应用，后文按作业原顺序分别验证五个问题。')
figure('figure01-applications.png', '三个内置 AI 应用及各自的技能、材料绑定')
p('此次取证使用“解题助手”，为它套用“循序引导”模板，绑定上传的数学引导技能和《数学自学笔记》。提问统一使用“' + e['question'] + '”。这样可以在同一次请求中观察模板、知识库、技能和 MCP 如何参与回答。验证时间为' + local_time.strftime('%Y年%m月%d日 %H:%M') + '，结束后已恢复原助手配置。')
p('原26个接口脚本均已通过，并与数据库记录核对。本报告新增的8张图及对应原始请求、返回保存在 docs/evidence/report/，后文第2至6节依次回答截图中的五个验证问题。')

h('二、验证1：LLM 请求、返回与提示词模板', new_page=True)
p('教师选择“循序引导”模板后，模板正文填入角色提示词，并保存在 tutor_agents.prompt 中。图2是本次保存前的配置：提示词、上传技能和数学材料均已选定。回答时，systemPrompt 读取保存的 prompt，将它放进 system 消息；传递的是模板套用后的文字，不是整个模板列表。')
figure('figure02-template-bindings.png', '本次验证采用的模板、上传技能和材料配置', 300)
p('学生通过 POST /api/conversations/{id}/messages 发送 question。sendMessage 校验用户和会话后组装模型输入，再请求 DeepSeek 的 /chat/completions。下面是本次首轮请求中除 messages、tools 外的实际参数；这两个数组分别在本节及后面的技能、MCP验证中展开。')
code({key: first['request'][key] for key in ['model', 'max_tokens', 'thinking', 'stream', 'tool_choice']})
p('messages 的第一条为 system，包含角色提示词、选中技能的名称及说明和回答约束；随后最多加入6条历史消息，每条截取500字；最后一条 user 放入当前问题和检索资料。本次是新会话，首轮没有历史消息。图3中可以直接看到“你是耐心的数学学习助手。先询问学生已经尝试的步骤，再给一条具体提示。”出现在 system.content 中，与图2保存的模板文字一致。')
figure('figure04-llm-request.png', '请求详情中的 system 片段，包含与图2一致的模板正文', 200, crop=(43, 355, 1609, 735))
p('本次提问实际产生了' + str(len(e['traces'])) + '次 LLM 请求：首轮决定调用工具；工具返回后再次询问，模型不再提出调用；最后以 stream=true 生成正文。下面是实际 choices[0] 的工具选择字段节选，保留原有嵌套关系。')
code({'finish_reason': first_response['choices'][0]['finish_reason'],
      'message': {'tool_calls': [{'function': model_call['function']}]}})
usage = first_response['usage']
p('首轮 usage 中，prompt_tokens 为' + str(usage['prompt_tokens']) + '，completion_tokens 为' + str(usage['completion_tokens']) + '，total_tokens 为' + str(usage['total_tokens']) + '。最终请求不再携带 tools，后端读取 choices[0].delta.content，转成多次 delta 事件发给浏览器。所有增量拼接后与保存的助手消息一致，实际回答见图6。配置与请求对照表明，模板正文会传递给 LLM。')

h('三、验证2：绑定知识库后是否全量发送', new_page=True)
p('绑定知识库不会使每次请求都携带库中的全部内容。材料原文和切片仍保存在 MySQL 中，agentKnowledge 只检索当前班级、当前助手绑定的材料。程序先按 ngram 全文匹配分数取最多24个候选，再检查连续文字重合和技能的资料范围，最后最多保留4个切片，每片最多800个 Unicode 字符。')
p('本次绑定《数学自学笔记》，命中1个切片，字符位置为0至103。图4为来源按钮打开的原文。该材料只有103字，命中切片恰好覆盖全文；对更长材料，程序仍按相关性检索并限制片数，不是固定拼接整个知识库。')
figure('figure05-source-material.png', '从回答来源打开《数学自学笔记》原文')
p('本轮 user 消息中的资料部分如下。可以看到它包含来源标题、切片编号和片段正文，没有包含其他材料，也没有把资料放入 system。')
user_text = first['request']['messages'][-1]['content']
code(user_text, 'text')
p('验证还检查了跨班级搜索：B班账号即使传入 class_id=1，也无法搜索到A班材料中的专有词。没有命中资料时，新对话接口仍按角色提示词生成回答，但引用列表为空，不保留无依据的编号。知识库的全部内容不随请求发送，范围和片段数量均由后端控制。')

h('四、验证3：绑定技能后传给 LLM 的内容', new_page=True)
p('系统内置数学解题引导、作业批改和错题讲解，并支持上传 .md 或 .txt 文件。此次通过技能上传接口保存了“数学解题引导（上传验证）”。图5是上传后从技能库打开的源文件，元数据与空行以下的指导正文均被保留。')
figure('figure03-uploaded-skill.png', '上传技能的源文件：元数据与指导正文', 295)
p('整份文件保存在 skills.source_text，但一次请求不会把文件原文及全部技能一并传给 LLM。程序只从当前助手绑定且启用的技能中选择至多一项，把选中技能的名称和完整说明正文放入 system。适用条件用于路由，“资料”用于检索筛选，“工具”用于限定候选集，这些字段由宿主分别处理。')
p('从本次实际 system 消息中提取出的技能部分如下。可以与图5对照：指导正文完整出现，原文件中的“适用条件”“资料”“启用”等元数据行没有作为源文件一起发送。')
system = first['request']['messages'][0]['content']
start = system.index('当前技能：')
end = system.index('\n硬性要求：', start)
code(system[start:end], 'text')
p('本次记录中的选中技能编号为' + str(skill['id']) + '，名称为“' + skill['name'] + '”。请求 JSON 中没有 source_text 字段，也不包含作业批改、错题讲解等其他技能的正文。由此可以区分“整份技能文件入库”与“选中说明参与模型请求”这两个过程。')

h('五、验证4：具体技能怎样执行', new_page=True)
p('parseSkill 解析元数据与正文并入库，绑定时检查班级范围及工具有效性。提问后，routeSkill 只查看本助手绑定且启用的技能，将问题和条件转为小写并按分隔符切词，每命中1词得1分；最高分胜出，同分取较小ID，零分不选。')
p('本题命中“去括号”和“系数”，因此选中上传技能。systemPrompt 随后加入说明正文和不给最终答案的约束；知识库筛选及可用工具也按该技能处理。这是宿主按照配置编排一次回答，不会执行上传文件里的代码。')
p('向同一会话连续提交相同问题，中间只停用该技能。两次均命中1条资料，而 meta 中的 skill 字段如下，其他字段省略。停用后立即变为 null，说明每次提问都会重新读取技能状态。')
code('启用时：\n' + json.dumps({'skill': {'id': skill['id'], 'name': skill['name']}}, ensure_ascii=False, separators=(',', ':')) + '\n\n停用时：\n' + json.dumps({'skill': disabled['skill']}, ensure_ascii=False, separators=(',', ':')), 'text')
p('图6对应启用时的回答：下方显示命中的技能、成功调用的公式工具和资料来源。回答只给下一步检查建议。技能带来的是本轮提示规则、资料筛选和工具权限的变化，不是直接把一份文件原样交给模型自行执行。')
figure('figure06-skill-and-tool-result.png', '技能启用时的回答及技能、工具、来源标记')

h('六、验证5：MCP 接入后传给 LLM 的参数', new_page=True)
p('MCP 服务由同一工程中的 mcp-server 独立运行，Base URL 为 http://mcp-demo:8090。宿主连接时先执行 initialize，再执行 tools/list，取得名称、说明和 inputSchema，成功后整体替换发现清单。图7展示本次连接的服务器及两个工具 formula_hint、unit_convert。接口只显示“已配置令牌”，不回传令牌原文。')
figure('figure07-mcp-discovery.png', '已连接的 MCP 服务及发现的两个工具')
sub('1. 发送给 LLM 的工具声明')
p('本轮选中技能声明了两个外部工具，宿主据此限定候选集，再将发现结果转换为 tools 数组。formula_hint@campus-tools 映射为模型函数名 formula_hint__campus_tools。下面是该工具实际发送给模型的完整声明，保持 JSON 的嵌套关系；另一项 unit_convert 使用相同结构，参数为 value、from 和 to。')
tool = next(t for t in first['request']['tools'] if t['function']['name'] == model_call['function']['name'])
f = tool['function'];schema = f['parameters']
code({'type': tool['type'], 'function': {'name': f['name'], 'description': f['description'],
      'parameters': {'type': schema['type'], 'properties': schema['properties'],
                     'required': schema['required'], 'additionalProperties': schema['additionalProperties']}}})
p('模型得到的是上述名称、说明和输入结构，不会获得 MCP 地址或访问令牌。本次模型返回的参数字符串为' + model_call['function']['arguments'] + '。谁决定调用与谁实际执行在这里是分开的：模型选择工具，宿主负责校验并发请求。')
sub('2. 宿主执行工具，再把结果回传给模型')
p('executeTool 在调用前重新核对连接、发现清单和权限，并用同一份 inputSchema 校验模型参数，随后携带服务端令牌向 MCP 发送以下 JSON-RPC 报文。这是宿主与工具服务之间的请求，不是发送给 LLM 的 tools 声明。')
code('{\n  "jsonrpc":"2.0", "id":1, "method":"tools/call",\n  "params":{"name":"formula_hint",\n            "arguments":' + json.dumps(json.loads(model_call['function']['arguments']), ensure_ascii=False, separators=(',', ':')) + '}\n}')
p('工具返回“检查括号前的系数是否乘到括号内每一项，负号是否正确分配。”宿主先过滤指令式句式，正常文本最多保留2000字，再用 role=tool 放入下一轮 messages，通过 tool_call_id 对应先前调用。下面是本次回传的工具消息，结果以数据角色参与回答，没有进入 system。')
code(next(m for m in final['request']['messages'] if m['role'] == 'tool'))
p('图8中最新一条记录对应本次会话#' + str(e['conversation_id']) + '，状态为成功，触发者为 student_a1，包含工具引用、轮次和时间。它与模型提出的工具、返回内容和图6的页面状态可以相互核对。')
figure('figure08-mcp-calls.png', '调用记录中本次会话的工具、状态、触发者与时间', crop=(0, 0, 1572, 403))

(ROOT / 'docs' / '个人项目阶段性验收报告.md').write_text('\n'.join(markdown), encoding='utf-8')
(EVIDENCE / 'figure-index.json').write_text(json.dumps(figures, ensure_ascii=False, indent=2), encoding='utf-8')

def page_number(canvas, doc):
    canvas.setFont('Song', 9)
    canvas.drawCentredString(A4[0] / 2, 29, str(doc.page))

doc = SimpleDocTemplate(str(PDF), pagesize=A4, leftMargin=60, rightMargin=60,
                        topMargin=52, bottomMargin=52, title=TITLE_TEXT, author='',
                        subject='按个人项目阶段验收五项问题说明实现与验证')
doc.build(story, onFirstPage=page_number, onLaterPages=page_number)
for target in [ROOT / 'output/pdf/CampusClaw阶段性验收报告.pdf', ROOT / 'docs/CampusClaw阶段性验收报告.pdf']:
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(PDF, target)
reader = PdfReader(str(PDF));text = '\n'.join(p.extract_text() for p in reader.pages)
assert len(figures) == 8
assert all(f'验证{i}' in text for i in range(1, 6))
assert all(x not in text for x in ['同伴', '签认', 'function.parameters:', 'function.name:'])
print(f'Saved {PDF}; {len(reader.pages)} pages, {len(figures)} figures')
