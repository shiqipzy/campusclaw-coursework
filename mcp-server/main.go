// CampusClaw course MCP subset: JSON-RPC over authenticated POST /mcp.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
)

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var tools = []tool{
	{"formula_hint", "为方程、去括号、通分、面积和行程问题提供下一步检查提示；不求最终答案。仅用于数学解题引导。", map[string]any{"type": "object", "properties": map[string]any{"topic": map[string]any{"type": "string", "enum": []string{"方程", "去括号", "通分", "面积", "行程"}}}, "required": []string{"topic"}, "additionalProperties": false}},
	{"unit_convert", "将长度、质量或时间的数值在同维度单位之间精确换算。长度m/cm/km，质量g/kg，时间s/min/h；不能跨维度换算。", map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "number", "minimum": -1e12, "maximum": 1e12}, "from": map[string]any{"type": "string", "enum": []string{"m", "cm", "km", "g", "kg", "s", "min", "h"}}, "to": map[string]any{"type": "string", "enum": []string{"m", "cm", "km", "g", "kg", "s", "min", "h"}}}, "required": []string{"value", "from", "to"}, "additionalProperties": false}},
}

func main() {
	secret := os.Getenv("MCP_AUTH_TOKEN")
	if len(secret) < 24 {
		log.Fatal("MCP_AUTH_TOKEN must contain at least 24 characters")
	}
	log.Fatal(http.ListenAndServe(":8090", handler(secret)))
}
func handler(secret string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		send := func(id any, result any, code int, message string) {
			body := map[string]any{"jsonrpc": "2.0", "id": id}
			if code != 0 {
				body["error"] = map[string]any{"code": code, "message": message}
			} else {
				body["result"] = result
			}
			_ = json.NewEncoder(w).Encode(body)
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			w.WriteHeader(401)
			send(nil, nil, -32001, "MCP bearer token required")
			return
		}
		var req struct {
			Version string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		dec := json.NewDecoder(r.Body)
		if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
			send(nil, nil, -32700, "Parse error")
			return
		}
		var id any
		if len(req.ID) > 0 {
			if json.Unmarshal(req.ID, &id) != nil {
				send(nil, nil, -32600, "Invalid id")
				return
			}
		}
		if req.Version != "2.0" || req.Method == "" {
			send(id, nil, -32600, "Invalid JSON-RPC 2.0 request")
			return
		}
		if id != nil {
			switch id.(type) {
			case string, float64:
			default:
				send(nil, nil, -32600, "Invalid id")
				return
			}
		}
		switch req.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(req.Params, &params) != nil || params.ProtocolVersion == "" {
				send(id, nil, -32602, "protocolVersion required")
				return
			}
			send(id, map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "campus-tools", "version": "1.0.0"}}, 0, "")
		case "tools/list":
			send(id, map[string]any{"tools": tools}, 0, "")
		case "tools/call":
			var params struct {
				Name      string `json:"name"`
				Arguments any    `json:"arguments"`
			}
			if json.Unmarshal(req.Params, &params) != nil {
				send(id, nil, -32602, "name and arguments required")
				return
			}
			var selected *tool
			for i := range tools {
				if tools[i].Name == params.Name {
					selected = &tools[i]
				}
			}
			if selected == nil {
				send(id, nil, -32602, "Unknown tool")
				return
			}
			data, _ := json.Marshal(selected.InputSchema)
			schema, err := jsonschema.CompileString("https://campus-tools.local/input.json", string(data))
			if err != nil || schema.Validate(params.Arguments) != nil {
				send(id, nil, -32602, "Arguments do not match inputSchema")
				return
			}
			args := params.Arguments.(map[string]any)
			text := ""
			isError := false
			if params.Name == "formula_hint" {
				hints := map[string]string{"方程": "先找出等式两边的同类项，检查移项时符号是否改变。", "去括号": "检查括号前的系数是否乘到括号内每一项，负号是否正确分配。", "通分": "先找分母的最小公倍数，再检查分子是否同步扩大。", "面积": "先判断图形类型，核对底和高是否对应以及单位是否一致。", "行程": "先整理路程、速度、时间的已知量，确认单位相同。"}
				text = hints[args["topic"].(string)]
			} else {
				type unit struct {
					group string
					scale float64
				}
				units := map[string]unit{"m": {"length", 1}, "cm": {"length", .01}, "km": {"length", 1000}, "g": {"mass", 1}, "kg": {"mass", 1000}, "s": {"time", 1}, "min": {"time", 60}, "h": {"time", 3600}}
				from, to := args["from"].(string), args["to"].(string)
				a, b := units[from], units[to]
				if a.group != b.group {
					isError = true
					text = "换算失败：源单位和目标单位的物理维度不同。"
				} else {
					value := args["value"].(float64)
					converted := value * a.scale / b.scale
					if math.IsInf(converted, 0) || math.IsNaN(converted) {
						isError = true
						text = "换算结果超出数值范围。"
					} else {
						text = fmt.Sprintf("%g %s = %g %s", value, from, converted, to)
					}
				}
			}
			send(id, map[string]any{"content": []map[string]string{{"type": "text", "text": strings.TrimSpace(text)}}, "isError": isError}, 0, "")
		default:
			send(id, nil, -32601, "Method not found")
		}
	})
	return mux
}
