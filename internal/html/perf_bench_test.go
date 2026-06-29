package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadCorpus builds an in-memory corpus from the first section of every
// testdata .txt file (the section before the first "-----" separator is valid
// input twig/html). It is used by the allocation/throughput benchmarks below.
func loadCorpus(tb testing.TB) []string {
	tb.Helper()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		tb.Fatal(err)
	}
	var corpus []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			continue
		}
		input := string(data)
		if i := strings.Index(input, "\n-----"); i != -1 {
			input = input[:i]
		}
		if strings.TrimSpace(input) == "" {
			continue
		}
		corpus = append(corpus, input)
	}
	if len(corpus) == 0 {
		tb.Fatal("empty corpus")
	}
	return corpus
}

// BenchmarkParseCorpus parses the whole corpus once per iteration and reports
// allocations + throughput. Run with:
//
//	go test ./internal/html -run=^$ -bench=BenchmarkParseCorpus -benchmem
func BenchmarkParseCorpus(b *testing.B) {
	corpus := loadCorpus(b)
	totalBytes := 0
	for _, s := range corpus {
		totalBytes += len(s)
	}
	b.SetBytes(int64(totalBytes))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, src := range corpus {
			if _, err := NewParser(src); err != nil {
				b.Fatalf("parse: %v", err)
			}
		}
	}
}

// BenchmarkLexCorpus isolates the lexer (no parser/AST build) so we can see how
// much of the cost is tokenization vs tree building.
func BenchmarkLexCorpus(b *testing.B) {
	corpus := loadCorpus(b)
	totalBytes := 0
	for _, s := range corpus {
		totalBytes += len(s)
	}
	b.SetBytes(int64(totalBytes))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, src := range corpus {
			if _, err := newLexer(src).lex(); err != nil {
				b.Fatalf("lex: %v", err)
			}
		}
	}
}

// BenchmarkParseLarge parses one big concatenated document, reflecting the
// per-byte cost without the fixed per-call overhead that dominates tiny files.
func BenchmarkParseLarge(b *testing.B) {
	corpus := loadCorpus(b)
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		for _, s := range corpus {
			sb.WriteString(s)
			sb.WriteByte('\n')
		}
	}
	big := sb.String()
	b.SetBytes(int64(len(big)))
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := NewParser(big); err != nil {
			b.Fatalf("parse: %v", err)
		}
	}
}
