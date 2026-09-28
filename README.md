# CampusClaw 班级材料与可溯源检索

这是前三课材料系统及第四课检索作业的演示实现：服务端 Session 登录、A/B 班材料隔离、教师上传 `.txt`/`.md`、鉴权下载，以及本班知识库关键字检索和来源溯源。前端为原生 HTML/CSS/JS，后端为 Go，数据保存在 MySQL 8.0。

## 启动

1. 安装 Docker Desktop（Linux 容器模式）或可运行 Compose 的 Docker Engine。
2. 复制 `.env.example` 为 `.env`，将四个密码变量替换成各不相同的强密码。`.env` 已被 Git 忽略。
3. 在此目录执行 `docker compose up --build -d`。
4. 打开 `http://localhost:8080`。若修改 `WEB_PORT`，使用对应端口。

如果 Docker Engine 安装在 WSL Ubuntu 中，可在 Ubuntu 终端进入本项目对应的 `/mnt/c/.../campusclaw` 路径后执行 `docker compose up --build -d`；若当前用户没有 Docker 权限，可用 `sudo`。

本机 WSL 在没有前台进程时可能自动退出，导致 `127.0.0.1:8080` 暂时出现 `ERR_CONNECTION_REFUSED`。做浏览器演示或截图时，请在单独的 Ubuntu 终端进入项目目录并运行 `docker compose up`，保持该窗口开启；结束演示后可按 Ctrl+C。登录前 `/api/me` 返回 401 是预期行为。

预置账号：`teacher_a`（A 班教师）、`student_a1`（A 班学生）、`student_b1`（B 班学生）。密码分别取自 `.env` 的三个用户密码变量。首次启动会创建两班各一份示例材料。已有数据不会被覆盖。修改 `.env` 中的种子密码不会更改已存在账号的密码。

查看状态：`docker compose ps`、`docker compose logs api --tail=100`。停止服务：`docker compose down`。卷 `database` 和 `uploads` 会保留；`docker compose down -v` 会删除本项目卷中的数据，日常操作不要加 `-v`。

## 接口

所有浏览器请求使用同源地址 `http://localhost:8080`。Nginx 将 `/api/` 和 `/health` 转发给内部 `api:8081`，只有 `web` 的宿主机端口被映射。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/login` | JSON `username`、`password`；成功设置 `session` Cookie |
| POST | `/api/logout` | 删除服务端会话，清 Cookie |
| GET | `/api/me` | 当前用户身份 |
| GET | `/api/materials` | 本班材料列表 |
| GET | `/api/search?q=短语` | 本班材料切片检索，返回来源信息；`q` 为 2–200 个字符 |
| GET | `/api/materials/{id}` | 本班材料正文 |
| POST | `/api/materials` | 教师上传 multipart `file` |
| GET | `/api/materials/{id}/file` | 鉴权下载原文件 |
| GET | `/health` | 进程存活检查 |

上传文件必须是非空 UTF-8 文本，扩展名仅 `.txt` 或 `.md`，默认上限 5 MiB。可用 `MAX_FILE_BYTES` 和 `SESSION_HOURS` 设置后端限制（默认为 5 MiB、8 小时）。身份、角色和班级均由服务端会话及数据库决定，客户端提交同名字段不会改变权限。

## 第四课检索变更

OpenSpec 变更位于 [`openspec/changes/add-traceable-vector-retrieval/`](openspec/changes/add-traceable-vector-retrieval/)：[`proposal.md`](openspec/changes/add-traceable-vector-retrieval/proposal.md)、[`design.md`](openspec/changes/add-traceable-vector-retrieval/design.md)、[`tasks.md`](openspec/changes/add-traceable-vector-retrieval/tasks.md) 和 [`knowledge-retrieval` 增量规约](openspec/changes/add-traceable-vector-retrieval/specs/knowledge-retrieval/spec.md)。

上传后按约 800 个 Unicode 字符切片，相邻切片重叠 80 字；原文和切片留在 MySQL。检索使用 ngram 全文索引排序，并要求完整短语出现在切片原文中。每条结果含材料标题、切片序号、原文字符区间 `[char_start,char_end)`、摘录及可打开的材料地址。检索仅使用 Session 中的班级编号；请求携带的 `class_id` 无效。无命中返回 HTTP 200、空 `hits` 和「资料中未找到相关内容」。

本次课后基本要求采用 MySQL 关键字／短语检索；向量库是可选技术路线。课件示例中的 Qdrant、`vector`／`hybrid` 模式和模型问答尚未实现。页面输入适合原文关键词或短语，不支持同义改写的语义匹配。

## 开发与验证

`go test ./...` 运行单元测试。源码按 `MYSQL_DSN`、`UPLOAD_DIR` 和三个用户密码环境变量启动。使用专用数据库验证，避免影响其他项目。实际课堂证据在 `deliverables/`；每份说明会标注已实测与待截图内容。

OpenSpec 校验：`npx --yes @fission-ai/openspec validate add-traceable-vector-retrieval --strict`。第四课端到端验收应使用独立 Compose 项目，例如在 WSL 项目目录运行 `WEB_PORT=18080 docker compose -p campusclaw_lesson4_test up --build -d`，再在 Windows 项目目录运行 `python scripts/verify_search.py --base-url http://127.0.0.1:18080`。验收脚本会上传一份测试材料，仅对专用测试栈运行。完成后在 WSL 项目目录运行 `docker compose -p campusclaw_lesson4_test down -v` 清理该测试栈的卷。

前三课端到端验收：`python scripts/verify.py`。数据库失败清理验证：在 WSL Ubuntu 的项目目录执行 `bash scripts/verify_rollback.sh`，脚本只对本项目 Compose 数据库建立临时触发器，退出时删除。网页截图脚本在 Windows Edge 上运行：先执行 `npm install --no-save --package-lock=false playwright-core`，再执行 `node scripts/capture.mjs`。这些脚本不是生产运行依赖。

本仓库是课堂作业的可运行示例；第三课截图证据留在本地 `deliverables/`，不随公开仓库发布。
