package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	chunkSize    = 800
	chunkOverlap = 80
	searchLimit  = 10
	noResults    = "资料中未找到相关内容"
)

type knowledgeChunk struct {
	Index int
	Start int // Unicode character offset, inclusive.
	End   int // Unicode character offset, exclusive.
	Text  string
}

type searchHit struct {
	ChunkID     int64   `json:"chunk_id"`
	MaterialID  int64   `json:"material_id"`
	Title       string  `json:"title"`
	ChunkIndex  int     `json:"chunk_index"`
	CharStart   int     `json:"char_start"`
	CharEnd     int     `json:"char_end"`
	Excerpt     string  `json:"excerpt"`
	MaterialURL string  `json:"material_url"`
	Score       float64 `json:"score"`
}

type searchResponse struct {
	Hits    []searchHit `json:"hits"`
	Message string      `json:"message,omitempty"`
}

// chunkText keeps offsets in the original text so each result can be traced
// back to the exact character range in knowledge_entries.content.
func chunkText(body string) []knowledgeChunk {
	runes := []rune(body)
	if len(runes) == 0 {
		return nil
	}
	chunks := make([]knowledgeChunk, 0, (len(runes)+chunkSize-1)/chunkSize)
	for start := 0; start < len(runes); {
		end := start + chunkSize
		if end >= len(runes) {
			end = len(runes)
		} else {
			// Prefer a nearby sentence or paragraph boundary over cutting mid-sentence.
			for i := end; i >= start+chunkSize*3/4; i-- {
				if strings.ContainsRune("\n。！？.!?；;", runes[i-1]) {
					end = i
					break
				}
			}
		}
		chunks = append(chunks, knowledgeChunk{Index: len(chunks) + 1, Start: start, End: end, Text: string(runes[start:end])})
		if end == len(runes) {
			break
		}
		start = end - chunkOverlap
	}
	return chunks
}

func insertKnowledgeChunks(ctx context.Context, tx *sql.Tx, materialID, classID int64, body string) error {
	for _, chunk := range chunkText(body) {
		_, err := tx.ExecContext(ctx, `INSERT INTO knowledge_chunks(material_id,chunk_index,class_id,char_start,char_end,content)
			VALUES(?,?,?,?,?,?)`, materialID, chunk.Index, classID, chunk.Start, chunk.End, chunk.Text)
		if err != nil {
			return err
		}
	}
	return nil
}

// Backfill existing lesson-three materials without changing their original text.
func backfillKnowledgeChunks(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT k.material_id,k.class_id,k.content FROM knowledge_entries k
		WHERE NOT EXISTS (SELECT 1 FROM knowledge_chunks c WHERE c.material_id=k.material_id)`)
	if err != nil {
		return err
	}
	type entry struct {
		materialID, classID int64
		body                string
	}
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.materialID, &e.classID, &e.body); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err = insertKnowledgeChunks(ctx, tx, e.materialID, e.classID, e.body); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func excerpt(body string) string {
	runes := []rune(strings.TrimSpace(body))
	if len(runes) <= 240 {
		return string(runes)
	}
	return string(runes[:240]) + "…"
}

func (a *app) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(query); n < 2 || n > 200 {
		fail(w, http.StatusBadRequest, "q must contain 2 to 200 characters")
		return
	}
	// The class is taken exclusively from the authenticated session. Any
	// class_id supplied by the caller is ignored.
	rows, err := a.db.QueryContext(r.Context(), `SELECT c.id,m.id,m.title,c.chunk_index,c.char_start,c.char_end,c.content,
		MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
		FROM knowledge_chunks c JOIN materials m ON m.id=c.material_id AND m.class_id=?
		WHERE c.class_id=? AND MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE)>0
		AND LOCATE(?,c.content)>0
		ORDER BY score DESC,c.id ASC LIMIT ?`, query, u.ClassID, u.ClassID, query, query, searchLimit)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer rows.Close()
	response := searchResponse{Hits: []searchHit{}}
	for rows.Next() {
		var h searchHit
		var body string
		if err := rows.Scan(&h.ChunkID, &h.MaterialID, &h.Title, &h.ChunkIndex, &h.CharStart, &h.CharEnd, &body, &h.Score); err != nil {
			serverFailure(w, err)
			return
		}
		h.Excerpt = excerpt(body)
		h.MaterialURL = fmt.Sprintf("/api/materials/%d", h.MaterialID)
		response.Hits = append(response.Hits, h)
	}
	if err := rows.Err(); err != nil {
		serverFailure(w, err)
		return
	}
	if len(response.Hits) == 0 {
		response.Message = noResults
	}
	writeJSON(w, http.StatusOK, response)
}
