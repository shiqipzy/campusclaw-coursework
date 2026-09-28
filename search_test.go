package main

import (
	"strings"
	"testing"
)

func TestChunkOffsetsAndOverlap(t *testing.T) {
	body := strings.Repeat("汉字材料。", 210)
	chunks := chunkText(body)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	runes := []rune(body)
	for i, c := range chunks {
		if c.Index != i+1 || c.Start < 0 || c.End > len(runes) || c.Start >= c.End {
			t.Fatalf("invalid chunk coordinates: %+v", c)
		}
		if c.Text != string(runes[c.Start:c.End]) {
			t.Fatalf("chunk %d is not the original text at its offsets", c.Index)
		}
		if i > 0 && c.Start != chunks[i-1].End-chunkOverlap {
			t.Fatalf("chunk %d has no expected overlap", c.Index)
		}
	}
	if chunks[len(chunks)-1].End != len(runes) {
		t.Fatal("last chunk does not reach the end")
	}
}
