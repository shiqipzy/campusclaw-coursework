package main

import (
	"net/http"
	"strings"
)

// apiOperation is the executable documentation annotation for one HTTP operation.
// Parameters and authentication are emitted into OpenAPI on every /openapi.json request.
type apiOperation struct {
	Method, Path, Summary, Input, Success string
	Public                                bool
}

var apiOperations = []apiOperation{
	{"GET", "/health", "进程存活", "", "string", true},
	{"POST", "/api/login", "登录并取得 Bearer JWT", "Login", "LoginResult", true},
	{"POST", "/api/logout", "撤销当前令牌", "", "", false}, {"GET", "/api/me", "当前身份", "", "Identity", false},
	{"GET", "/api/materials", "本班材料列表", "", "MaterialList", false}, {"POST", "/api/materials", "教师上传 UTF-8 txt/md 材料", "MaterialUpload", "Material", false},
	{"GET", "/api/materials/{id}", "本班材料正文", "", "Material", false}, {"GET", "/api/materials/{id}/file", "鉴权下载原文件", "", "string", false},
	{"GET", "/api/search", "本班知识库检索；q 为2-200字", "", "SearchResult", false}, {"POST", "/api/ask", "第四课兼容单次问答", "Question", "AskResult", false},
	{"GET", "/api/prompt-templates", "内置提示词模板", "", "TemplateList", false},
	{"GET", "/api/agents", "本班智能体；学生仅见已启用项", "", "AgentList", false}, {"POST", "/api/agents", "教师创建智能体和两份绑定", "AgentInput", "Agent", false},
	{"GET", "/api/agents/{id}", "智能体详情；跨班/学生读取停用项均404", "", "Agent", false}, {"PUT", "/api/agents/{id}", "教师修改智能体及绑定；下一轮生效", "AgentInput", "Agent", false},
	{"GET", "/api/skills", "本班技能列表", "", "SkillList", false}, {"GET", "/api/skills/{id}", "本班技能与源文件", "", "Skill", false},
	{"POST", "/api/skills", "教师新建技能；保存前校验工具引用", "SkillInput", "Skill", false}, {"PUT", "/api/skills/{id}", "教师修改技能", "SkillInput", "Skill", false}, {"DELETE", "/api/skills/{id}", "教师删除技能与绑定", "", "", false}, {"POST", "/api/skills/upload", "教师上传 .md/.txt 技能文件", "SkillUpload", "Skill", false},
	{"GET", "/api/conversations", "仅当前用户的会话，最多100项", "", "ConversationList", false}, {"POST", "/api/conversations", "锁定本班已启用智能体，新建会话", "ConversationInput", "Conversation", false},
	{"GET", "/api/conversations/{id}", "只读自己的会话及消息", "", "ConversationDetail", false}, {"POST", "/api/conversations/{id}/messages", "检索、路由、模型工具决策、流式回答；meta/tool_calls先于delta，done后结束", "ChatInput", "SSE", false},
	{"GET", "/api/conversations/{id}/traces", "本人会话的真实 LLM 请求和返回；不含鉴权请求头", "", "TraceList", false},
	{"GET", "/api/mcp/servers", "教师列出本班 MCP 服务；仅回 has_token", "", "ServerList", false}, {"POST", "/api/mcp/servers", "教师登记；令牌加密存储，不回传", "ServerInput", "Server", false},
	{"GET", "/api/mcp/servers/{id}", "教师查看本班服务器", "", "Server", false}, {"PUT", "/api/mcp/servers/{id}", "教师修改；空 token 保留原令牌，修改后须重连", "ServerInput", "Server", false}, {"DELETE", "/api/mcp/servers/{id}", "教师删除服务器，历史调用记录保留", "", "", false},
	{"POST", "/api/mcp/servers/{id}/connect", "initialize + tools/list；整体替换发现清单", "", "Server", false}, {"POST", "/api/mcp/servers/{id}/disconnect", "清空清单并立即禁止调用", "", "Server", false}, {"GET", "/api/mcp/servers/{id}/tools", "教师查看发现的工具及输入结构", "", "ToolList", false}, {"GET", "/api/mcp/calls", "教师查看本班追加式调用记录，默认50，limit为1-200", "", "CallList", false},
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func strSchema() map[string]any { return map[string]any{"type": "string"} }
func refSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func docSchemas() map[string]any {
	str := strSchema()
	num := map[string]any{"type": "integer", "format": "int64"}
	boolean := map[string]any{"type": "boolean"}
	ids := map[string]any{"type": "array", "items": num}
	schemas := map[string]any{
		"Error":             objectSchema(map[string]any{"error": str}, "error"),
		"Login":             objectSchema(map[string]any{"username": str, "password": map[string]any{"type": "string", "format": "password", "writeOnly": true}}, "username", "password"),
		"Identity":          objectSchema(map[string]any{"id": num, "username": str, "role": map[string]any{"type": "string", "enum": []string{"teacher", "student"}}, "class_id": num, "class_name": str}),
		"LoginResult":       objectSchema(map[string]any{"token": str, "token_type": str, "expires_in": num, "user": refSchema("Identity")}),
		"Question":          objectSchema(map[string]any{"question": map[string]any{"type": "string", "minLength": 2, "maxLength": 200}}, "question"),
		"ChatInput":         objectSchema(map[string]any{"question": map[string]any{"type": "string", "minLength": 2, "maxLength": 500}, "agent_id": map[string]any{"type": "integer", "description": "忽略；权限只取自会话"}, "class_id": map[string]any{"type": "integer", "description": "忽略；权限只取自登录身份与会话"}}, "question"),
		"AgentInput":        objectSchema(map[string]any{"name": map[string]any{"type": "string", "maxLength": 64}, "prompt": map[string]any{"type": "string", "maxLength": 2000}, "enabled": boolean, "skill_ids": ids, "material_ids": ids}, "name", "enabled"),
		"Agent":             objectSchema(map[string]any{"id": num, "name": str, "prompt": str, "enabled": boolean, "skill_ids": ids, "material_ids": ids}),
		"SkillInput":        objectSchema(map[string]any{"source_text": map[string]any{"type": "string", "maxLength": 8000}}, "source_text"),
		"Skill":             objectSchema(map[string]any{"id": num, "name": str, "trigger_text": str, "subject_text": str, "description": str, "tools": map[string]any{"type": "array", "items": str}, "source_text": str, "enabled": boolean}),
		"ConversationInput": objectSchema(map[string]any{"agent_id": num}, "agent_id"),
		"Conversation":      objectSchema(map[string]any{"id": num, "agent_id": num, "title": str, "created_at": str, "updated_at": str}),
		"ServerInput":       objectSchema(map[string]any{"name": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_-]{0,31}$"}, "base_url": map[string]any{"type": "string", "format": "uri"}, "token": map[string]any{"type": "string", "format": "password", "writeOnly": true}}, "name", "base_url"),
		"Tool":              objectSchema(map[string]any{"name": str, "description": str, "inputSchema": map[string]any{"type": "object"}}),
		"Server":            objectSchema(map[string]any{"id": num, "name": str, "base_url": str, "has_token": boolean, "connected": boolean, "last_error": str, "tools": map[string]any{"type": "array", "items": refSchema("Tool")}}),
		"Material":          objectSchema(map[string]any{"id": num, "title": str, "filename": str, "class_id": num, "created_at": str, "content": str}),
		"Citation":          objectSchema(map[string]any{"chunk_id": num, "material_id": num, "title": str, "chunk_index": num, "char_start": num, "char_end": num, "excerpt": str, "material_url": str, "score": map[string]any{"type": "number"}}),
		"Template":          objectSchema(map[string]any{"name": str, "prompt": str}),
		"Message":           objectSchema(map[string]any{"id": num, "role": str, "content": str, "citations": map[string]any{"type": "array", "items": refSchema("Citation")}, "skill_name": str, "created_at": str}),
		"Call":              objectSchema(map[string]any{"id": num, "tool_ref": str, "status": map[string]any{"type": "string", "enum": []string{"ok", "denied", "error"}}, "username": str, "conversation_id": num, "round": num, "detail": str, "created_at": str}),
		"Trace":             objectSchema(map[string]any{"id": num, "question_message_id": num, "call_index": num, "request": map[string]any{"type": "object"}, "response": map[string]any{"type": "object"}, "created_at": str}),
		"AskResult":         map[string]any{"type": "object", "properties": map[string]any{"answer": str, "mode": str, "sources": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
	}
	schemas["SearchResult"] = objectSchema(map[string]any{"hits": map[string]any{"type": "array", "items": refSchema("Citation")}, "message": str})
	schemas["ConversationDetail"] = objectSchema(map[string]any{"conversation": refSchema("Conversation"), "messages": map[string]any{"type": "array", "items": refSchema("Message")}})
	for list, item := range map[string]string{"AgentList": "Agent", "SkillList": "Skill", "MaterialList": "Material", "ConversationList": "Conversation", "ServerList": "Server", "ToolList": "Tool", "TemplateList": "Template", "CallList": "Call", "TraceList": "Trace"} {
		schemas[list] = map[string]any{"type": "array", "items": refSchema(item)}
	}
	schemas["MaterialUpload"] = objectSchema(map[string]any{"file": map[string]any{"type": "string", "format": "binary"}}, "file")
	schemas["SkillUpload"] = schemas["MaterialUpload"]
	return schemas
}
func openAPIDocument() map[string]any {
	paths := map[string]any{}
	for _, annotation := range apiOperations {
		op := map[string]any{"summary": annotation.Summary, "security": []any{map[string]any{"bearerAuth": []string{}}}, "tags": []string{strings.Split(strings.TrimPrefix(annotation.Path, "/api/"), "/")[0]}}
		if annotation.Public {
			op["security"] = []any{}
		}
		params := []any{}
		if strings.Contains(annotation.Path, "{id}") {
			params = append(params, map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "minimum": 1}})
		}
		if annotation.Path == "/api/search" {
			params = append(params, map[string]any{"name": "q", "in": "query", "required": true, "schema": map[string]any{"type": "string", "minLength": 2, "maxLength": 200}})
		}
		if annotation.Path == "/api/mcp/calls" {
			params = append(params, map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		if annotation.Input != "" {
			kind := "application/json"
			if strings.HasSuffix(annotation.Input, "Upload") {
				kind = "multipart/form-data"
			}
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{kind: map[string]any{"schema": refSchema(annotation.Input)}}}
		}
		responses := map[string]any{}
		status := "200"
		if annotation.Method == "POST" && (annotation.Path == "/api/agents" || annotation.Path == "/api/skills" || annotation.Path == "/api/skills/upload" || annotation.Path == "/api/conversations" || annotation.Path == "/api/mcp/servers" || annotation.Path == "/api/materials") {
			status = "201"
		}
		if annotation.Success == "" {
			status = "204"
		}
		success := map[string]any{"description": "成功"}
		if annotation.Success != "" {
			kind := "application/json"
			schema := refSchema(annotation.Success)
			if annotation.Success == "SSE" {
				kind = "text/event-stream"
				schema = strSchema()
				schema["description"] = "event: meta | tool_calls | delta | done | error；data为JSON；delta.text是新增文本"
			}
			if annotation.Success == "string" {
				kind = "text/plain"
				schema = strSchema()
			}
			success["content"] = map[string]any{kind: map[string]any{"schema": schema}}
		}
		responses[status] = success
		for code, description := range map[string]string{"400": "请求格式、技能字段或工具引用错误", "401": "未登录或令牌无效", "403": "非教师尝试管理操作", "404": "资源不存在或不在当前范围内（响应一致）", "409": "同一会话已有回答进行中", "413": "上传超过上限", "502": "模型或 MCP 服务失败", "503": "数据库或模型配置不可用"} {
			responses[code] = map[string]any{"description": description, "content": map[string]any{"application/json": map[string]any{"schema": refSchema("Error")}}}
		}
		op["responses"] = responses
		p, ok := paths[annotation.Path].(map[string]any)
		if !ok {
			p = map[string]any{}
			paths[annotation.Path] = p
		}
		p[strings.ToLower(annotation.Method)] = op
	}
	return map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "CampusClaw 第5/6课 API", "version": "1.0.0", "description": "Bearer JWT、班级隔离、本人会话、技能与MCP。SSE开始后错误以error事件返回。"}, "servers": []any{map[string]string{"url": "/"}}, "paths": paths, "components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}, "schemas": docSchemas()}}
}
func (a *app) docRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, openAPIDocument()) })
	m.HandleFunc("GET /swagger/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>CampusClaw API</title><link rel="stylesheet" href="/vendor/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="/vendor/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:'/openapi.json',dom_id:'#swagger-ui',persistAuthorization:false,deepLinking:true});</script></body></html>`))
	})
}
