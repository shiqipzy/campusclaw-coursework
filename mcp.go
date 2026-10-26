package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}
type mcpServer struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	HasToken  bool      `json:"has_token"`
	Connected bool      `json:"connected"`
	Tools     []mcpTool `json:"tools"`
	LastError string    `json:"last_error"`
	Cipher    string    `json:"-"`
}
type toolEvent struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
	Text   string `json:"text"`
	Round  int    `json:"round"`
}
type candidate struct {
	Ref       string
	Server    mcpServer
	Tool      mcpTool
	ModelName string
}

func tokenAEAD() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(os.Getenv("JWT_SECRET")))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func sealToken(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	c, err := tokenAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(c.Seal(nonce, nonce, []byte(raw), nil)), nil
}
func openToken(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	c, err := tokenAEAD()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < c.NonceSize() {
		return "", fmt.Errorf("invalid stored credential")
	}
	plain, err := c.Open(nil, raw[:c.NonceSize()], raw[c.NonceSize():], nil)
	return string(plain), err
}

func compileInput(schema json.RawMessage) (*jsonschema.Schema, error) {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil || obj["type"] != "object" {
		return nil, fmt.Errorf("输入结构的根类型须为 object")
	}
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(string) (io.ReadCloser, error) { return nil, fmt.Errorf("外部 schema 引用不可用") }
	if err := compiler.AddResource("https://campusclaw.local/input.json", bytes.NewReader(schema)); err != nil {
		return nil, err
	}
	return compiler.Compile("https://campusclaw.local/input.json")
}
func validateArguments(schema json.RawMessage, args string) error {
	compiled, err := compileInput(schema)
	if err != nil {
		return err
	}
	var value any
	if json.Unmarshal([]byte(args), &value) != nil {
		return fmt.Errorf("参数须为 JSON 对象")
	}
	return compiled.Validate(value)
}

func validateMCPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("Base URL 必须是无凭据、无查询参数的 HTTP(S) 地址")
	}
	for _, host := range strings.Split(envDefault("MCP_ALLOWED_HOSTS", "mcp-demo,localhost,127.0.0.1"), ",") {
		if strings.EqualFold(strings.TrimSpace(host), u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("服务器主机未列入服务端 MCP_ALLOWED_HOSTS")
}
func rpc(ctx context.Context, s mcpServer, method string, params any, result any) error {
	credential, err := openToken(s.Cipher)
	if err != nil {
		return fmt.Errorf("stored token unavailable")
	}
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.BaseURL, "/")+"/mcp", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid MCP endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("MCP 服务器不可达或超时")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("MCP HTTP %d；请检查地址与令牌", res.StatusCode)
	}
	var envelope struct {
		Version string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope) != nil || envelope.Version != "2.0" || envelope.ID != 1 {
		return fmt.Errorf("MCP 响应不是有效的 JSON-RPC 2.0")
	}
	if envelope.Error != nil {
		return fmt.Errorf("MCP JSON-RPC 错误 %d", envelope.Error.Code)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" || json.Unmarshal(envelope.Result, result) != nil {
		return fmt.Errorf("MCP 返回数据不完整")
	}
	return nil
}
func (a *app) loadServer(ctx context.Context, id, classID int64) (mcpServer, error) {
	s := mcpServer{Tools: []mcpTool{}}
	var tools []byte
	err := a.db.QueryRowContext(ctx, `SELECT id,name,base_url,token_cipher,connected,tools_json,last_error FROM mcp_servers WHERE id=? AND class_id=?`, id, classID).Scan(&s.ID, &s.Name, &s.BaseURL, &s.Cipher, &s.Connected, &tools, &s.LastError)
	if err == nil {
		err = json.Unmarshal(tools, &s.Tools)
	}
	s.HasToken = s.Cipher != ""
	return s, err
}
func (a *app) servers(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id FROM mcp_servers WHERE class_id=? ORDER BY id`, u.ClassID)
	if err != nil {
		serverFailure(w, err)
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		serverFailure(w, err)
		return
	}
	list := []mcpServer{}
	for _, id := range ids {
		s, err := a.loadServer(r.Context(), id, u.ClassID)
		if err != nil {
			serverFailure(w, err)
			return
		}
		list = append(list, s)
	}
	writeJSON(w, 200, list)
}
func (a *app) getServer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	s, err := a.loadServer(r.Context(), pathID(r), u.ClassID)
	if err != nil {
		resourceError(w, err)
		return
	}
	writeJSON(w, 200, s)
}

var serverNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func (a *app) saveServer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	id := pathID(r)
	old := mcpServer{}
	var err error
	if r.Method == "PUT" {
		old, err = a.loadServer(r.Context(), id, u.ClassID)
		if err != nil {
			resourceError(w, err)
			return
		}
	}
	var input struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if !readInput(w, r, &input) {
		return
	}
	if !serverNamePattern.MatchString(input.Name) || len(input.BaseURL) > 500 || len(input.Token) > 4096 {
		fail(w, 400, "服务器名须为小写字母开头，最多32字符；令牌最多4096字符")
		return
	}
	if err = validateMCPURL(input.BaseURL); err != nil {
		fail(w, 400, err.Error())
		return
	}
	encrypted := old.Cipher
	if input.Token != "" {
		encrypted, err = sealToken(input.Token)
		if err != nil {
			fail(w, 500, "credential storage failed")
			return
		}
	}
	if encrypted == "" {
		fail(w, 400, "首次登记须填写令牌")
		return
	}
	if r.Method == "PUT" {
		_, err = a.db.ExecContext(r.Context(), `UPDATE mcp_servers SET name=?,base_url=?,token_cipher=?,connected=FALSE,tools_json='[]',last_error='' WHERE id=? AND class_id=?`, input.Name, input.BaseURL, encrypted, id, u.ClassID)
	} else {
		var res sql.Result
		res, err = a.db.ExecContext(r.Context(), `INSERT INTO mcp_servers(class_id,name,base_url,token_cipher,tools_json) VALUES(?,?,?,?,'[]')`, u.ClassID, input.Name, input.BaseURL, encrypted)
		if err == nil {
			id, _ = res.LastInsertId()
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			fail(w, 400, "本班已有同名服务器")
		} else {
			serverFailure(w, err)
		}
		return
	}
	s, err := a.loadServer(r.Context(), id, u.ClassID)
	if err != nil {
		serverFailure(w, err)
		return
	}
	code := 201
	if r.Method == "PUT" {
		code = 200
	}
	writeJSON(w, code, s)
}
func (a *app) connectServer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	s, err := a.loadServer(r.Context(), pathID(r), u.ClassID)
	if err != nil {
		resourceError(w, err)
		return
	}
	var hello struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
	}
	err = rpc(r.Context(), s, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "CampusClaw", "version": "1.0"}}, &hello)
	if err == nil && (hello.ProtocolVersion != "2024-11-05" || hello.Capabilities["tools"] == nil) {
		err = fmt.Errorf("MCP 协议版本或 tools 能力不兼容")
	}
	var discovered struct {
		Tools []mcpTool `json:"tools"`
	}
	if err == nil {
		err = rpc(r.Context(), s, "tools/list", map[string]any{}, &discovered)
	}
	if err == nil {
		if len(discovered.Tools) > 32 {
			err = fmt.Errorf("服务器工具超过32个")
		}
		seen := map[string]bool{}
		for _, tool := range discovered.Tools {
			if !toolNamePattern.MatchString(tool.Name) || tool.Description == "" || len(tool.Description) > 6000 || seen[tool.Name] {
				err = fmt.Errorf("工具名称/说明无效或名称重复")
				break
			}
			seen[tool.Name] = true
			if _, e := compileInput(tool.InputSchema); e != nil {
				err = fmt.Errorf("工具 %s 输入结构无效", tool.Name)
				break
			}
		}
	}
	if err != nil {
		if _, dbErr := a.db.ExecContext(r.Context(), `UPDATE mcp_servers SET connected=FALSE,tools_json='[]',last_error=? WHERE id=? AND class_id=?`, short(err.Error(), 255), s.ID, u.ClassID); dbErr != nil {
			serverFailure(w, dbErr)
			return
		}
		fail(w, 502, err.Error())
		return
	}
	if discovered.Tools == nil {
		discovered.Tools = []mcpTool{}
	}
	tools, _ := json.Marshal(discovered.Tools)
	if _, err = a.db.ExecContext(r.Context(), `UPDATE mcp_servers SET connected=TRUE,tools_json=?,last_error='' WHERE id=? AND class_id=?`, string(tools), s.ID, u.ClassID); err != nil {
		serverFailure(w, err)
		return
	}
	s.Connected = true
	s.Tools = discovered.Tools
	s.LastError = ""
	writeJSON(w, 200, s)
}
func (a *app) disconnectServer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	s, err := a.loadServer(r.Context(), pathID(r), u.ClassID)
	if err != nil {
		resourceError(w, err)
		return
	}
	if _, err = a.db.ExecContext(r.Context(), `UPDATE mcp_servers SET connected=FALSE,tools_json='[]',last_error='' WHERE id=? AND class_id=?`, s.ID, u.ClassID); err != nil {
		serverFailure(w, err)
		return
	}
	s.Connected = false
	s.Tools = []mcpTool{}
	s.LastError = ""
	writeJSON(w, 200, s)
}
func (a *app) deleteServer(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	s, err := a.loadServer(r.Context(), pathID(r), u.ClassID)
	if err != nil {
		resourceError(w, err)
		return
	}
	if _, err = a.db.ExecContext(r.Context(), `DELETE FROM mcp_servers WHERE id=? AND class_id=?`, s.ID, u.ClassID); err != nil {
		serverFailure(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) serverTools(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	s, err := a.loadServer(r.Context(), pathID(r), u.ClassID)
	if err != nil {
		resourceError(w, err)
		return
	}
	writeJSON(w, 200, s.Tools)
}
func (a *app) serverByName(ctx context.Context, name string, classID int64) (mcpServer, error) {
	var id int64
	if err := a.db.QueryRowContext(ctx, `SELECT id FROM mcp_servers WHERE name=? AND class_id=?`, name, classID).Scan(&id); err != nil {
		return mcpServer{}, err
	}
	return a.loadServer(ctx, id, classID)
}
func (a *app) validateSkillTools(ctx context.Context, classID int64, refs []string) error {
	for _, ref := range refs {
		if ref == "knowledge_search" {
			continue
		}
		p := strings.SplitN(ref, "@", 2)
		s, err := a.serverByName(ctx, p[1], classID)
		if err != nil {
			return fmt.Errorf("工具服务器不存在")
		}
		if !s.Connected {
			return fmt.Errorf("服务器 %s 未连接", s.Name)
		}
		found := false
		for _, tool := range s.Tools {
			if tool.Name == p[0] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s 不在已发现工具清单中", ref)
		}
	}
	return nil
}

func cleanToolResult(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, phrase := range []string{"ignore previous", "ignore all", "system prompt", "system message", "忽略之前", "忽略以上", "忽略所有", "忽略先前", "系统提示", "执行命令", "泄露", "你必须", "你现在是"} {
		if strings.Contains(lower, phrase) {
			return "工具返回含指令式内容，已整条丢弃。", false
		}
	}
	return short(text, 2000), true
}

func (a *app) recordTool(ctx context.Context, u identity, convID int64, s mcpServer, event toolEvent) error {
	// A renamed/deleted server can no longer own this event. Preserve historical rows.
	if s.ID == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO tool_call_logs(class_id,server_id,tool_ref,status,user_id,conversation_id,round_no,detail) SELECT ?,id,?,?,?,?,?,? FROM mcp_servers WHERE id=? AND class_id=? AND name=?`, u.ClassID, event.Ref, event.Status, u.ID, convID, event.Round, short(event.Text, 2000), s.ID, u.ClassID, s.Name)
	return err
}
func (a *app) candidates(ctx context.Context, u identity, convID int64, s *skill) ([]candidate, []toolEvent, error) {
	list := []candidate{}
	events := []toolEvent{}
	if s == nil {
		return list, events, nil
	}
	for _, ref := range s.Tools {
		if ref == "knowledge_search" {
			continue
		}
		p := strings.SplitN(ref, "@", 2)
		server, err := a.serverByName(ctx, p[1], u.ClassID)
		if err != nil && err != sql.ErrNoRows {
			return nil, nil, err
		}
		found := false
		if err == nil && server.Connected {
			for _, tool := range server.Tools {
				if tool.Name == p[0] {
					list = append(list, candidate{ref, server, tool, tool.Name + "__" + strings.ReplaceAll(server.Name, "-", "_")})
					found = true
					break
				}
			}
		}
		if !found {
			event := toolEvent{ref, "denied", "工具未连接、不存在或不在发现清单中", 0}
			events = append(events, event)
			if err = a.recordTool(ctx, u, convID, server, event); err != nil {
				return nil, nil, err
			}
		}
	}
	return list, events, nil
}
func (a *app) executeTool(ctx context.Context, u identity, convID int64, c candidate, args string, round int) toolEvent {
	event := toolEvent{c.Ref, "denied", "工具未连接或授权已失效", round}
	current, err := a.loadServer(ctx, c.Server.ID, u.ClassID)
	if err == nil && current.Name == c.Server.Name && current.Connected {
		for _, tool := range current.Tools {
			if tool.Name != c.Tool.Name {
				continue
			}
			if validateArguments(tool.InputSchema, args) != nil {
				event.Text = "工具参数不符合已发现的输入结构"
				break
			}
			var arguments any
			_ = json.Unmarshal([]byte(args), &arguments)
			var result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			}
			err = rpc(ctx, current, "tools/call", map[string]any{"name": tool.Name, "arguments": arguments}, &result)
			event.Status = "error"
			event.Text = "工具执行失败"
			if err != nil {
				event.Text = err.Error()
			} else {
				var text strings.Builder
				for _, part := range result.Content {
					if part.Type == "text" {
						text.WriteString(part.Text)
						text.WriteByte('\n')
					}
				}
				clean, safe := cleanToolResult(strings.TrimSpace(text.String()))
				event.Text = clean
				if safe && !result.IsError && clean != "" {
					event.Status = "ok"
				}
			}
			break
		}
	}
	if err = a.recordTool(ctx, u, convID, c.Server, event); err != nil {
		event.Status = "error"
		event.Text = "调用审计写入失败"
	}
	return event
}
func (a *app) callLogs(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		n, e := strconvAtoi(value)
		if e != nil || n < 1 || n > 200 {
			fail(w, 400, "limit must be 1-200")
			return
		}
		limit = n
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT l.id,l.tool_ref,l.status,u.username,l.conversation_id,l.round_no,l.detail,l.created_at FROM tool_call_logs l JOIN users u ON u.id=l.user_id WHERE l.class_id=? ORDER BY l.id DESC LIMIT ?`, u.ClassID, limit)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, conv int64
		var round int
		var ref, status, user, detail string
		var at time.Time
		if err = rows.Scan(&id, &ref, &status, &user, &conv, &round, &detail, &at); err != nil {
			serverFailure(w, err)
			return
		}
		list = append(list, map[string]any{"id": id, "tool_ref": ref, "status": status, "username": user, "conversation_id": conv, "round": round, "detail": detail, "created_at": at})
	}
	if err = rows.Err(); err != nil {
		serverFailure(w, err)
		return
	}
	writeJSON(w, 200, list)
}
func strconvAtoi(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err == nil && fmt.Sprint(n) != s {
		err = fmt.Errorf("invalid number")
	}
	return n, err
}
