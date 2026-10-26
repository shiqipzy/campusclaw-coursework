package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var learningSchema = []string{
	`CREATE TABLE IF NOT EXISTS skills (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, name VARCHAR(32) NOT NULL, trigger_text VARCHAR(120) NOT NULL, subject_text VARCHAR(32) NOT NULL, description TEXT NOT NULL, tools_json JSON NOT NULL, source_text TEXT NOT NULL, enabled BOOLEAN NOT NULL, seed_key VARCHAR(64) NULL, UNIQUE(class_id,name), UNIQUE(class_id,seed_key)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS tutor_agents (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, name VARCHAR(64) NOT NULL, prompt TEXT NOT NULL, enabled BOOLEAN NOT NULL, seed_key VARCHAR(64) NULL, UNIQUE(class_id,name), UNIQUE(class_id,seed_key)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS tutor_agent_skills (agent_id BIGINT NOT NULL, skill_id BIGINT NOT NULL, PRIMARY KEY(agent_id,skill_id), FOREIGN KEY(agent_id) REFERENCES tutor_agents(id) ON DELETE CASCADE, FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS tutor_agent_materials (agent_id BIGINT NOT NULL, material_id BIGINT NOT NULL, PRIMARY KEY(agent_id,material_id), FOREIGN KEY(agent_id) REFERENCES tutor_agents(id) ON DELETE CASCADE, FOREIGN KEY(material_id) REFERENCES materials(id) ON DELETE CASCADE) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS conversations (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, user_id BIGINT NOT NULL, agent_id BIGINT NOT NULL, title VARCHAR(100) NOT NULL, created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), INDEX(user_id,class_id), FOREIGN KEY(agent_id) REFERENCES tutor_agents(id)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS messages (id BIGINT AUTO_INCREMENT PRIMARY KEY, conversation_id BIGINT NOT NULL, role ENUM('user','assistant') NOT NULL, content MEDIUMTEXT NOT NULL, citations_json JSON NOT NULL, skill_name VARCHAR(32) NOT NULL DEFAULT '', created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS mcp_servers (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, name VARCHAR(32) NOT NULL, base_url VARCHAR(500) NOT NULL, token_cipher TEXT NOT NULL, connected BOOLEAN NOT NULL DEFAULT FALSE, tools_json JSON NOT NULL, last_error VARCHAR(255) NOT NULL DEFAULT '', UNIQUE(class_id,name)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS tool_call_logs (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, server_id BIGINT NOT NULL, tool_ref VARCHAR(100) NOT NULL, status ENUM('ok','denied','error') NOT NULL, user_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL, round_no INT NOT NULL, detail TEXT NOT NULL, created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), INDEX(class_id,created_at), INDEX(conversation_id)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS llm_exchanges (id BIGINT AUTO_INCREMENT PRIMARY KEY, class_id BIGINT NOT NULL, user_id BIGINT NOT NULL, conversation_id BIGINT NOT NULL, question_message_id BIGINT NOT NULL, call_index INT NOT NULL, request_json JSON NOT NULL, response_json JSON NOT NULL, created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), INDEX(conversation_id)) ENGINE=InnoDB`,
}

type skill struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Trigger     string   `json:"trigger_text"`
	Subject     string   `json:"subject_text"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Source      string   `json:"source_text"`
	Enabled     bool     `json:"enabled"`
}
type tutorAgent struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Prompt      string  `json:"prompt"`
	Enabled     bool    `json:"enabled"`
	SkillIDs    []int64 `json:"skill_ids"`
	MaterialIDs []int64 `json:"material_ids"`
}

var refPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*@[a-z][a-z0-9_-]*$`)
var keywordSplit = regexp.MustCompile(`[,，、;；|\n]+`)

func parseSkill(source string) (skill, error) {
	s := skill{Source: source, Tools: []string{}}
	if !utf8.ValidString(source) || utf8.RuneCountInString(source) > 8000 {
		return s, fmt.Errorf("技能文件须为 UTF-8，且不超过 8000 字")
	}
	source = strings.ReplaceAll(strings.TrimPrefix(source, "\uFEFF"), "\r\n", "\n")
	parts := strings.SplitN(strings.TrimSpace(source), "\n\n", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return s, fmt.Errorf("空行以下须填写技能说明")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		pair := strings.SplitN(strings.Replace(line, "：", ":", 1), ":", 2)
		if len(pair) != 2 {
			return s, fmt.Errorf("元数据须使用 字段：内容")
		}
		key, val := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		if _, ok := fields[key]; ok {
			return s, fmt.Errorf("重复字段：%s", key)
		}
		if key != "名称" && key != "适用条件" && key != "资料" && key != "工具" && key != "启用" {
			return s, fmt.Errorf("未知字段：%s", key)
		}
		fields[key] = val
	}
	s.Name = fields["名称"]
	s.Trigger = fields["适用条件"]
	s.Subject = fields["资料"]
	s.Description = strings.TrimSpace(parts[1])
	if s.Name == "" || s.Trigger == "" || (fields["启用"] != "是" && fields["启用"] != "否") {
		return s, fmt.Errorf("名称、适用条件、启用（是/否）必填")
	}
	s.Enabled = fields["启用"] == "是"
	if utf8.RuneCountInString(s.Name) > 32 || utf8.RuneCountInString(s.Trigger) > 120 || utf8.RuneCountInString(s.Subject) > 32 || utf8.RuneCountInString(s.Description) > 2000 {
		return s, fmt.Errorf("字段超长：名称32、适用条件120、资料32、说明2000字以内")
	}
	seen := map[string]bool{}
	for _, ref := range keywordSplit.Split(fields["工具"], -1) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if ref != "knowledge_search" && (!refPattern.MatchString(ref) || len(ref) > 64) {
			return s, fmt.Errorf("非法工具引用：%s；格式为 tool_name@server-name", ref)
		}
		if !seen[ref] {
			s.Tools = append(s.Tools, ref)
			seen[ref] = true
		}
	}
	return s, nil
}

func routeSkill(question string, skills []skill) *skill {
	best, score := -1, 0
	question = strings.ToLower(question)
	for i, s := range skills {
		if !s.Enabled {
			continue
		}
		n := 0
		for _, word := range keywordSplit.Split(strings.ToLower(s.Trigger), -1) {
			if word = strings.TrimSpace(word); word != "" && strings.Contains(question, word) {
				n++
			}
		}
		if n > score || (n > 0 && n == score && (best < 0 || s.ID < skills[best].ID)) {
			best = i
			score = n
		}
	}
	if best < 0 {
		return nil
	}
	return &skills[best]
}

func initLearning(ctx context.Context, db *sql.DB) error {
	for _, stmt := range learningSchema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if password := os.Getenv("TEACHER_B_PASSWORD"); password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `INSERT IGNORE INTO users(username,password_hash,role,class_id) VALUES('teacher_b',?,'teacher',2)`, string(hash)); err != nil {
			return err
		}
	}
	for _, classID := range []int{1, 2} {
		seeds := []struct{ key, name, trigger, body string }{
			{"guide", "数学解题引导", "方程,去括号,分数,通分,面积,应用题,未知数,行程,单位,换算", "先判断学生已做的步骤，只指出下一步需要检查什么。不给最终得数、选项或完整解法。"},
			{"review", "作业批改", "批改,检查作业,评分", "对学生提交的过程分项检查，说明评分依据与错误位置。资料不足时说明不能据此评分。"},
			{"mistake", "错题讲解", "错题,错因,为什么错", "先定位错误类型，再给一条纠正建议和一道同类练习。不要伪造学生的作答记录。"},
		}
		for _, seed := range seeds {
			source := fmt.Sprintf("名称：%s\n适用条件：%s\n资料：\n工具：knowledge_search\n启用：是\n\n%s", seed.name, seed.trigger, seed.body)
			_, err := db.ExecContext(ctx, `INSERT IGNORE INTO skills(class_id,name,trigger_text,subject_text,description,tools_json,source_text,enabled,seed_key) VALUES(?,?,?,?,?,'["knowledge_search"]',?,TRUE,?)`, classID, seed.name, seed.trigger, "", seed.body, source, seed.key)
			if err != nil {
				return err
			}
		}
		for _, seed := range []struct{ key, name, prompt string }{{"tutor", "解题助手", "你是耐心的数学学习助手，用简洁中文帮助学生独立思考。"}, {"review", "作业诊断助手", "你是作业诊断助手，帮助学生定位作业中的错误并提出改进建议。"}, {"plan", "学习计划助手", "你是学习规划助手，结合学生目标提供具体可执行的学习安排。"}} {
			result, err := db.ExecContext(ctx, `INSERT IGNORE INTO tutor_agents(class_id,name,prompt,enabled,seed_key) VALUES(?,?,?,TRUE,?)`, classID, seed.name, seed.prompt, seed.key)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n == 0 {
				continue
			}
			id, _ := result.LastInsertId()
			keys := []string{"guide"}
			if seed.key == "review" {
				keys = []string{"review", "mistake"}
			}
			if seed.key == "plan" {
				keys = nil
			}
			for _, key := range keys {
				if _, err = db.ExecContext(ctx, `INSERT IGNORE INTO tutor_agent_skills SELECT ?,id FROM skills WHERE class_id=? AND seed_key=?`, id, classID, key); err != nil {
					return err
				}
			}
			if _, err = db.ExecContext(ctx, `INSERT IGNORE INTO tutor_agent_materials SELECT ?,id FROM materials WHERE class_id=?`, id, classID); err != nil {
				return err
			}
		}
	}
	if raw := os.Getenv("MCP_AUTH_TOKEN"); raw != "" {
		cipher, err := sealToken(raw)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `INSERT IGNORE INTO mcp_servers(class_id,name,base_url,token_cipher,tools_json) VALUES(1,'campus-tools',?,?, '[]')`, envDefault("MCP_BASE_URL", "http://mcp-demo:8090"), cipher); err != nil {
			return err
		}
	}
	return nil
}

func readInput(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 400, "JSON body required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil || dec.Decode(new(any)) != io.EOF {
		fail(w, 400, "invalid JSON request")
		return false
	}
	return true
}
func (a *app) teacher(w http.ResponseWriter, r *http.Request) (identity, bool) {
	u, ok := a.requireUser(w, r)
	if ok && u.Role != "teacher" {
		fail(w, 403, "teacher required")
		ok = false
	}
	return u, ok
}
func pathID(r *http.Request) int64 { id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64); return id }
func resourceError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "not found")
	} else {
		serverFailure(w, err)
	}
}
func short(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (a *app) learningRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/agents", a.agents)
	m.HandleFunc("POST /api/agents", a.saveAgent)
	m.HandleFunc("GET /api/agents/{id}", a.getAgent)
	m.HandleFunc("PUT /api/agents/{id}", a.saveAgent)
	m.HandleFunc("GET /api/prompt-templates", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.requireUser(w, r); ok {
			writeJSON(w, 200, []map[string]string{{"name": "循序引导", "prompt": "你是耐心的数学学习助手。先询问学生已经尝试的步骤，再给一条具体提示。"}, {"name": "证据批改", "prompt": "你是作业诊断助手。按知识点检查作答，标明依据与不确定之处。"}, {"name": "学习规划", "prompt": "你是学习规划助手。先明确目标、时间和薄弱点，再制定可执行的安排。"}})
		}
	})
	m.HandleFunc("GET /api/skills", a.skills)
	m.HandleFunc("GET /api/skills/{id}", a.getSkill)
	m.HandleFunc("POST /api/skills", a.saveSkill)
	m.HandleFunc("POST /api/skills/upload", a.saveSkill)
	m.HandleFunc("PUT /api/skills/{id}", a.saveSkill)
	m.HandleFunc("DELETE /api/skills/{id}", a.deleteSkill)
	m.HandleFunc("GET /api/conversations", a.conversations)
	m.HandleFunc("POST /api/conversations", a.newConversation)
	m.HandleFunc("GET /api/conversations/{id}", a.conversation)
	m.HandleFunc("POST /api/conversations/{id}/messages", a.sendMessage)
	m.HandleFunc("GET /api/conversations/{id}/traces", a.traces)
	m.HandleFunc("GET /api/mcp/servers", a.servers)
	m.HandleFunc("POST /api/mcp/servers", a.saveServer)
	m.HandleFunc("GET /api/mcp/servers/{id}", a.getServer)
	m.HandleFunc("PUT /api/mcp/servers/{id}", a.saveServer)
	m.HandleFunc("DELETE /api/mcp/servers/{id}", a.deleteServer)
	m.HandleFunc("POST /api/mcp/servers/{id}/connect", a.connectServer)
	m.HandleFunc("POST /api/mcp/servers/{id}/disconnect", a.disconnectServer)
	m.HandleFunc("GET /api/mcp/servers/{id}/tools", a.serverTools)
	m.HandleFunc("GET /api/mcp/calls", a.callLogs)
	a.docRoutes(m)
}

func (a *app) loadSkills(ctx context.Context, classID, agentID int64) ([]skill, error) {
	query := `SELECT s.id,s.name,s.trigger_text,s.subject_text,s.description,s.tools_json,s.source_text,s.enabled FROM skills s WHERE s.class_id=?`
	args := []any{classID}
	if agentID > 0 {
		query += ` AND EXISTS(SELECT 1 FROM tutor_agent_skills b WHERE b.agent_id=? AND b.skill_id=s.id)`
		args = append(args, agentID)
	}
	query += " ORDER BY s.id"
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []skill{}
	for rows.Next() {
		var s skill
		var tools []byte
		if err = rows.Scan(&s.ID, &s.Name, &s.Trigger, &s.Subject, &s.Description, &tools, &s.Source, &s.Enabled); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(tools, &s.Tools); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
func (a *app) skills(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	list, err := a.loadSkills(r.Context(), u.ClassID, 0)
	if err != nil {
		serverFailure(w, err)
		return
	}
	writeJSON(w, 200, list)
}
func (a *app) getSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	list, err := a.loadSkills(r.Context(), u.ClassID, 0)
	if err != nil {
		serverFailure(w, err)
		return
	}
	for _, s := range list {
		if s.ID == pathID(r) {
			writeJSON(w, 200, s)
			return
		}
	}
	fail(w, 404, "not found")
}
func (a *app) saveSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	id := pathID(r)
	if r.Method == "PUT" {
		var n int
		if err := a.db.QueryRowContext(r.Context(), `SELECT 1 FROM skills WHERE id=? AND class_id=?`, id, u.ClassID).Scan(&n); err != nil {
			resourceError(w, err)
			return
		}
	}
	var source string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		if r.ParseMultipartForm(65536) != nil {
			fail(w, 400, "invalid skill upload")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "file required")
			return
		}
		defer f.Close()
		if !strings.HasSuffix(strings.ToLower(h.Filename), ".md") && !strings.HasSuffix(strings.ToLower(h.Filename), ".txt") {
			fail(w, 400, "技能仅支持 .md/.txt")
			return
		}
		body, err := io.ReadAll(io.LimitReader(f, 32001))
		if err != nil || len(body) > 32000 {
			fail(w, 400, "skill too large")
			return
		}
		source = string(body)
	} else {
		var input struct {
			Source string `json:"source_text"`
		}
		if !readInput(w, r, &input) {
			return
		}
		source = input.Source
	}
	s, err := parseSkill(source)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err = a.validateSkillTools(r.Context(), u.ClassID, s.Tools); err != nil {
		fail(w, 400, err.Error())
		return
	}
	tools, _ := json.Marshal(s.Tools)
	if r.Method == "PUT" {
		_, err = a.db.ExecContext(r.Context(), `UPDATE skills SET name=?,trigger_text=?,subject_text=?,description=?,tools_json=?,source_text=?,enabled=? WHERE id=? AND class_id=?`, s.Name, s.Trigger, s.Subject, s.Description, string(tools), s.Source, s.Enabled, id, u.ClassID)
	} else {
		var result sql.Result
		result, err = a.db.ExecContext(r.Context(), `INSERT INTO skills(class_id,name,trigger_text,subject_text,description,tools_json,source_text,enabled) VALUES(?,?,?,?,?,?,?,?)`, u.ClassID, s.Name, s.Trigger, s.Subject, s.Description, string(tools), s.Source, s.Enabled)
		if err == nil {
			id, _ = result.LastInsertId()
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			fail(w, 400, "本班已有同名技能")
		} else {
			serverFailure(w, err)
		}
		return
	}
	s.ID = id
	code := 201
	if r.Method == "PUT" {
		code = 200
	}
	writeJSON(w, code, s)
}
func (a *app) deleteSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	result, err := a.db.ExecContext(r.Context(), `DELETE FROM skills WHERE id=? AND class_id=?`, pathID(r), u.ClassID)
	if err != nil {
		serverFailure(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		fail(w, 404, "not found")
		return
	}
	w.WriteHeader(204)
}

func (a *app) loadAgent(ctx context.Context, id, classID int64) (tutorAgent, error) {
	ag := tutorAgent{SkillIDs: []int64{}, MaterialIDs: []int64{}}
	err := a.db.QueryRowContext(ctx, `SELECT id,name,prompt,enabled FROM tutor_agents WHERE id=? AND class_id=?`, id, classID).Scan(&ag.ID, &ag.Name, &ag.Prompt, &ag.Enabled)
	if err != nil {
		return ag, err
	}
	for _, binding := range []struct {
		query  string
		target *[]int64
	}{{`SELECT skill_id FROM tutor_agent_skills WHERE agent_id=? ORDER BY skill_id`, &ag.SkillIDs}, {`SELECT material_id FROM tutor_agent_materials WHERE agent_id=? ORDER BY material_id`, &ag.MaterialIDs}} {
		rows, err := a.db.QueryContext(ctx, binding.query, id)
		if err != nil {
			return ag, err
		}
		for rows.Next() {
			var n int64
			if err = rows.Scan(&n); err != nil {
				rows.Close()
				return ag, err
			}
			*binding.target = append(*binding.target, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return ag, err
		}
	}
	return ag, nil
}
func (a *app) agents(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	query := `SELECT id FROM tutor_agents WHERE class_id=?`
	if u.Role != "teacher" {
		query += ` AND enabled=TRUE`
	}
	query += " ORDER BY id"
	rows, err := a.db.QueryContext(r.Context(), query, u.ClassID)
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
	list := []tutorAgent{}
	for _, id := range ids {
		ag, err := a.loadAgent(r.Context(), id, u.ClassID)
		if err != nil {
			serverFailure(w, err)
			return
		}
		list = append(list, ag)
	}
	writeJSON(w, 200, list)
}
func (a *app) getAgent(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	ag, err := a.loadAgent(r.Context(), pathID(r), u.ClassID)
	if err == nil && u.Role != "teacher" && !ag.Enabled {
		err = sql.ErrNoRows
	}
	if err != nil {
		resourceError(w, err)
		return
	}
	writeJSON(w, 200, ag)
}
func (a *app) saveAgent(w http.ResponseWriter, r *http.Request) {
	u, ok := a.teacher(w, r)
	if !ok {
		return
	}
	id := pathID(r)
	if r.Method == "PUT" {
		if _, err := a.loadAgent(r.Context(), id, u.ClassID); err != nil {
			resourceError(w, err)
			return
		}
	}
	var ag tutorAgent
	if !readInput(w, r, &ag) {
		return
	}
	ag.Name = strings.TrimSpace(ag.Name)
	if ag.Name == "" || utf8.RuneCountInString(ag.Name) > 64 || utf8.RuneCountInString(ag.Prompt) > 2000 || len(ag.SkillIDs) > 100 || len(ag.MaterialIDs) > 100 {
		fail(w, 400, "名称1-64字，提示词最多2000字，每种绑定不超过100项")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer tx.Rollback()
	for _, b := range []struct {
		table string
		ids   []int64
	}{{"skills", ag.SkillIDs}, {"materials", ag.MaterialIDs}} {
		for _, n := range b.ids {
			var exists int
			if err = tx.QueryRowContext(r.Context(), "SELECT 1 FROM "+b.table+" WHERE id=? AND class_id=?", n, u.ClassID).Scan(&exists); err != nil {
				resourceError(w, err)
				return
			}
		}
	}
	if r.Method == "PUT" {
		_, err = tx.ExecContext(r.Context(), `UPDATE tutor_agents SET name=?,prompt=?,enabled=? WHERE id=? AND class_id=?`, ag.Name, ag.Prompt, ag.Enabled, id, u.ClassID)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(r.Context(), `INSERT INTO tutor_agents(class_id,name,prompt,enabled) VALUES(?,?,?,?)`, u.ClassID, ag.Name, ag.Prompt, ag.Enabled)
		if err == nil {
			id, _ = res.LastInsertId()
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			fail(w, 400, "本班已有同名智能体")
		} else {
			serverFailure(w, err)
		}
		return
	}
	for _, b := range []struct {
		table, column string
		ids           []int64
	}{{"tutor_agent_skills", "skill_id", ag.SkillIDs}, {"tutor_agent_materials", "material_id", ag.MaterialIDs}} {
		if _, err = tx.ExecContext(r.Context(), "DELETE FROM "+b.table+" WHERE agent_id=?", id); err != nil {
			serverFailure(w, err)
			return
		}
		for _, n := range b.ids {
			if _, err = tx.ExecContext(r.Context(), "INSERT IGNORE INTO "+b.table+"(agent_id,"+b.column+") VALUES(?,?)", id, n); err != nil {
				serverFailure(w, err)
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		serverFailure(w, err)
		return
	}
	ag.ID = id
	if ag.SkillIDs == nil {
		ag.SkillIDs = []int64{}
	}
	if ag.MaterialIDs == nil {
		ag.MaterialIDs = []int64{}
	}
	code := 201
	if r.Method == "PUT" {
		code = 200
	}
	writeJSON(w, code, ag)
}
