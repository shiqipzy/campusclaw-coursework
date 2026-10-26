# CampusClaw · 班级学习工作台

在第4课班级材料与可追溯检索上增量实现第5/6课：多个AI助手、提示词模板、知识库引用、技能上传、多轮流式会话，以及独立MCP工具服务。原材料页和单次问答接口继续可用。前端为原生HTML/CSS/JS，后端为Go，MySQL 8.0保存业务数据，Nginx提供同源访问。本次实际使用DeepSeek进行验收。

## 快速启动

1. 准备Docker Engine / Docker Desktop（Linux容器）和Compose。
2. 复制 `.env.example` 为 `.env`，填写数据库密码、账号口令、至少32字符的 `JWT_SECRET`、至少24字符的 `MCP_AUTH_TOKEN`，以及 `AI_API_KEY`。
3. 运行 `docker compose up --build -d`。
4. 打开 [材料页](http://localhost:8080)、[学习工作台](http://localhost:8080/assistant.html) 或 [Swagger](http://localhost:8080/swagger/)。

预置 `teacher_a`（A班教师）、`student_a1`（A班学生）、`student_b1`（B班学生）。设置 `TEACHER_B_PASSWORD` 后增加B班教师 `teacher_b`。口令来自本机 `.env`，不写在仓库和报告中。修改种子环境变量不会覆盖已有账号口令。

教师上传数学讲义，在“MCP工具”连接预置 `campus-tools`，再把技能和材料绑定到助手。也可运行 `python scripts/setup_demo.py`：幂等创建一份数学示例讲义，将已有解题助手和内置技能接到真实工具，不覆盖已有材料。

Compose包含web、api、db、mcp-demo四个服务，仅web映射 `127.0.0.1:${WEB_PORT:-8080}`；其余服务仅内部互通。数据库和上传卷持久化；日常使用 `docker compose down` 停止，不要附带删除数据卷的 `-v`。

Windows可以在WSL项目目录执行Compose，也可运行 `powershell -ExecutionPolicy Bypass -File scripts/start-local.ps1`；脚本以隐藏进程保持WSL运行，停止用 `scripts/stop-local.ps1`。手工演示可保持 `docker compose up` 前台窗口开启。重新创建API容器后若代理仍缓存旧地址，执行 `docker compose exec web nginx -s reload`。

## 第5课：助手、提示词、技能与流式会话

- 内置解题助手、作业诊断助手、学习计划助手，以及3份提示词模板。
- 内置数学解题引导、作业批改、错题讲解；支持教师编写/上传 `.md`、`.txt` 技能，格式见 [样板](examples/skills/数学工具引导.md)。
- 每次只在助手绑定且启用的技能里按适用条件计分，至多选一项；同分较小ID优先；0分不选择。启停和绑定下一轮立即生效。
- 会话新建时锁定本班已启用助手；助手停用后已有会话仍可继续，客户端不能通过请求体换助手或班级。
- 回答前检索本班且绑定材料，最多4片、每片800字；最近6条历史每条500字。角色与选中技能说明进system，资料进user。
- 无命中仍请求真实模型，引用为空并过滤无效编号。沿用MySQL ngram词面全文检索，尚无向量/混合检索。
- 最终正文采用上游真实SSE；meta、tool_calls先于delta，保存消息后才发done。Nginx关闭代理缓冲。

技能是课程自定义配置文本，不执行上传文件中的代码，也不等同于Agent Skills标准目录。名称含“解题引导”时加入不给最终答案的约束；本次真实题目通过不等于所有输出都能形式化保证。

## 第6课：MCP与工具编排

`mcp-server/` 独立构建，提供 `GET /health` 及带Bearer认证的 `POST /mcp`，支持JSON-RPC 2.0的initialize、tools/list、tools/call。这是课程要求的教学子集，不宣称覆盖完整MCP能力和全部传输。

- formula_hint：方程、去括号、通分、面积和行程的下一步检查提示。
- unit_convert：同维度的长度、质量、时间单位换算；不兼容维度返回工具错误。

连接依次握手和发现，成功整体替换清单；重复连接不翻倍。失败或断开清空清单并禁止调用。技能工具字段支持 `knowledge_search` 与 `tool_name@server-name`，保存前验证格式、发现清单和连接状态。

选中技能限定候选集，模型通过tools/tool_choice=auto自主选择。宿主重新检查连接与授权，按同一JSON Schema验证参数，再携带服务端令牌执行真正的tools/call。结果整条过滤指令式句式、截断至2000字，以role=tool回传。工具决策stream=false，最终正文stream=true。

默认 `MCP_MAX_ROUNDS=4`，`MCP_MAX_CALLS=12`（当前会话累计可归属记录）；超限后去掉工具并基于已有信息作答。工具失败不阻断检索和生成。成功、被拒、出错均追加记录，含工具、用户、会话、轮次和时间；教师仅见本班。准备阶段拒绝round=0，模型循环从1开始。被删/改名服务器无法归属的新事件只进SSE，历史记录保留。

令牌只在服务端，AES-GCM加密保存，接口仅回has_token。加密密钥由JWT_SECRET派生，更换后须重新登记MCP令牌。地址仅允许服务端MCP_ALLOWED_HOSTS中的主机，禁HTTP重定向。示例工具无外部写操作。

## API与实际LLM请求

完整入参、出参、认证和错误码见 [OpenAPI](http://localhost:8080/openapi.json) 和 [Swagger](http://localhost:8080/swagger/)。`api_docs.go` 的apiOperation是代码内的可执行文档注解，生成35项操作。Swagger静态资源随仓库提供，无运行时CDN依赖。

| 功能 | 主要入口 |
|---|---|
| 登录 | `POST /api/login`、`GET /api/me`、`POST /api/logout` |
| 材料与旧问答 | `/api/materials`、`/api/search?q=…`、`POST /api/ask` |
| 助手与模板 | `/api/agents`、`/api/agents/{id}`、`/api/prompt-templates` |
| 技能 | `/api/skills`、`/api/skills/{id}`、`POST /api/skills/upload` |
| 会话 | `/api/conversations`、`/api/conversations/{id}` |
| 流式消息 | `POST /api/conversations/{id}/messages` |
| 本人请求详情 | `GET /api/conversations/{id}/traces` |
| MCP管理 | `/api/mcp/servers`、`/api/mcp/servers/{id}`及connect/disconnect/tools |
| 调用记录 | `GET /api/mcp/calls?limit=50` |

后续请求用 `Authorization: Bearer <登录token>`，浏览器保存在sessionStorage。用户和班级由签名token及数据库决定；跨班和不存在404一致，同班其他用户也不能读取别人的会话/请求详情。

“查看请求详情”展示实际LLM请求JSON、实际返回JSON或SSE数据帧，不记录鉴权请求头，仅本人可读。用户问题和检索片段也可能包含隐私。公开 [LLM实测证据](docs/evidence/llm-exchanges.json) 仅使用人工构造的课堂样本。

## 验证与阶段验收

以下脚本会创建材料、技能与会话，并暂时停止该验收栈的MCP服务测试故障，应在独立验收环境运行：

```sh
WEB_PORT=18081 MCP_MAX_ROUNDS=1 MCP_MAX_CALLS=2 docker compose -p campusclaw_acceptance up --build -d
python apitest/run_all.py
node scripts/lesson56_ui.mjs
go test ./...
go vet ./...
npx --yes @fission-ai/openspec validate add-skills-streaming-mcp --strict
```

`apitest/cases/` 按URL/HTTP方法组织，一个接口一个脚本，核对真实数据库与返回内容。生成的 `deliverables/lesson56/api-report.html` 依课程要求不随作业提交。Windows数据库检查使用WSL Compose，Linux直接运行。UI脚本用Edge及playwright-core：`npm install --no-save --package-lock=false playwright-core`。

本次26/26接口脚本通过；真实模型、MCP、SSE、停机错误、断开拒绝、小配额终止和隔离已验证。详见 [验收说明](docs/阶段验收说明.md)、[验证结果](docs/evidence/acceptance-summary.json) 和 [增量规约](openspec/changes/add-skills-streaming-mcp/)。PDF由 `scripts/build_acceptance_report.py` 根据真实证据生成，本机在output/pdf，公开版本在 [阶段性验收报告](docs/CampusClaw阶段性验收报告.pdf)。

本次交付按个人项目阶段性验收整理，权限隔离由接口脚本分别使用A、B班教师和学生账号验证。报告正文及可编辑版本见 [个人项目报告](docs/个人项目阶段性验收报告.md)。
