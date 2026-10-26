## Why

第四课已有班级隔离、材料入库和可追溯检索。阶段验收要求在同一个 CampusClaw 上增加多个 AI 应用、提示词模板、上传技能和外部工具，并用实际请求解释模型输入与执行过程。

## What Changes

- 增加班级智能体与提示词模板、技能文本解析与上传、两类绑定和确定性路由。
- 增加归属到当前用户的多轮会话、受限历史、基于绑定材料的检索与 SSE。
- 增加同一 Compose 工程下的 MCP 服务，以及登记、握手、发现、工具候选集、参数校验和有上限的调用循环。
- 增加仅当前用户可见的 LLM 请求详情、班级调用审计、OpenAPI/Swagger 与按 URL 组织的接口脚本。

## Capabilities

### New Capabilities
- `learning-assistants`: 多助手、提示词、技能、绑定、会话与流式问答。
- `mcp-tooling`: 自建工具服务、宿主编排与可追溯调用。

### Modified Capabilities
- `knowledge-retrieval`: 对话内再限定为会话智能体绑定材料；第四课检索接口保持兼容。

## Impact

新增 MySQL 表，保留旧材料与账号。认证使用已有未提交成果中的 Bearer JWT。密钥不写入仓库或镜像。网页仍是原生 HTML/JS，新增 `/assistant.html`，原材料页保留。
