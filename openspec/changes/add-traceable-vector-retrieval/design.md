## Context

现有 Go 服务使用服务端 Session 查出用户和 `class_id`；`materials` 与 `knowledge_entries` 保存在 MySQL，上传仅允许教师。检索必须沿用同一身份与班级边界，并把命中片段准确指回材料正文。

## Goals / Non-Goals

**Goals**

- 师生能按短语检索本班已入库的 `.txt`／`.md` 材料。
- 返回可核验的切片来源；跨班材料既不返回正文，也不暴露其存在。
- 无依据时返回空结果；旧材料启动后自动可检索。

**Non-Goals**

- 当前变更不实现语义向量、Qdrant、混合排序、对话模型回答或文件删除／重建入口。
- 不改变第三课的上传文件类型、认证与角色规则。

## Decisions

### 1. MySQL 保存正文和检索索引

`knowledge_chunks` 使用自增 `id`、`material_id`、`class_id`、从 1 开始的 `chunk_index`、`char_start`、`char_end` 和 `content`。字符区间为 Unicode 字符下标 `[start,end)`，可直接从 `knowledge_entries.content` 切出同一段原文。`content` 建立 MySQL ngram FULLTEXT 索引。约 800 字一片、相邻重叠 80 字，尽量在邻近的换行或句末断开；短文本仍形成一个切片。上传时材料、原文和切片在同一个 MySQL 事务写入。启动时只对没有切片的存量材料补建，重复启动不会重复插入。

使用同一个数据库可以减少课后最小实现的部署负担。将来添加 Qdrant 时，可把 `knowledge_chunks.id` 作为向量点 ID，并在 payload 保留 `class_id`／`material_id`；向量命中后仍以 Session 班级回 MySQL 取正文。向量库不应成为摘录来源。

### 2. 先按全文相关度取候选，再核对原文短语

接口接受 `q` 为 2–200 个 Unicode 字符。MySQL `MATCH ... AGAINST` 排序，`LOCATE(q, content)>0` 要求原文中存在完整短语，避免 ngram 因共有字词将不同班级短语误判为相关。该选择适合关键词和短语，不承诺改写问句的语义匹配。最多返回 10 条，按得分降序，`id` 作稳定次序。

### 3. 班级只来自 Session

调用检索前使用现有 `requireUser`。SQL 对切片和材料同时加上会话 `class_id` 条件；请求中的 `class_id` 不参与查询。来源链接指向现有 `/api/materials/{id}`，该接口也在 SQL 层按会话班级过滤。无跨班命中时响应 200 与空 `hits`，不会返回跨班材料 ID 或标题。

### 4. 来源字段由 MySQL 切片生成

每条结果包含 `chunk_id`、`material_id`、`title`、`chunk_index`、`char_start`、`char_end`、最多 240 字的 `excerpt`、`material_url` 和关键词得分。摘录只从本班 MySQL 切片正文取得。前端使用 `textContent` 显示，不把材料正文当 HTML 执行；点击结果调用受保护的材料详情接口。

## Migration and Rollback

新表由服务启动时创建，随后回填旧材料；回填一份材料使用一个事务。原 `materials` 与 `knowledge_entries` 不被改写。回滚应用版本时可保留新增切片表，旧版本不会读取它；需要重新构建索引时，可在维护窗口仅清理本项目的 `knowledge_chunks` 并重启新版服务，原文仍在 MySQL。现有第三课数据库若已由其他实验创建了不同结构的同名表，需要先做专门迁移，本次不自动破坏未知表。

## Risks and Tradeoffs

- `LOCATE` 保证短语精确性，但无法匹配同义改写；大规模材料会需要重新评估全文索引与候选过滤方式。
- 若班级资料在多个切片边界附近，80 字重叠可降低漏检，却会产生少量重复结果。
- MySQL ngram 默认分词大小为 2；若部署环境修改了该配置，短语候选表现可能变化。上线前应运行端到端验收。
