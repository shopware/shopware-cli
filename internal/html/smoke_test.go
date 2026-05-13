package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeShopwareStorefront parses every .twig file in a checked-out
// shopware/storefront repository to surface real-world parsing failures.
//
// Set HTML_SMOKE_CORPUS to the storefront repo root to enable. Without it,
// the test is skipped so CI does not depend on an external clone.
//
//	HTML_SMOKE_CORPUS=/tmp/storefront go test ./internal/html/ -run TestSmokeShopwareStorefront -v
func TestSmokeShopwareStorefront(t *testing.T) {
	root := os.Getenv("HTML_SMOKE_CORPUS")
	if root == "" {
		t.Skip("set HTML_SMOKE_CORPUS to a directory of .twig files to enable")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Skipf("HTML_SMOKE_CORPUS=%q is not a directory", root)
	}

	var (
		total      int
		lexFails   int
		parseFails int
		fmtFails   int
		failures   []string
	)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".twig") {
			return nil
		}
		total++
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		src := string(data)

		// Layer 1: lexer must not error.
		if _, lexErr := newLexer(src).lex(); lexErr != nil {
			lexFails++
			failures = append(failures, "LEX  "+path+": "+lexErr.Error())
			return nil
		}

		// Layer 2: parser must not error.
		nodes, parseErr := NewParser(src)
		if parseErr != nil {
			parseFails++
			failures = append(failures, "PARSE "+path+": "+parseErr.Error())
			return nil
		}

		// Layer 3: formatter idempotency — parse, format, re-parse, re-format.
		formatted := nodes.Dump(0)
		nodes2, err2 := NewParser(formatted)
		if err2 != nil {
			fmtFails++
			failures = append(failures, "FMT-REPARSE "+path+": "+err2.Error())
			return nil
		}
		if nodes2.Dump(0) != formatted {
			fmtFails++
			failures = append(failures, "FMT-IDEMPOTENT "+path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("smoke results: total=%d  lex_fail=%d  parse_fail=%d  fmt_fail=%d",
		total, lexFails, parseFails, fmtFails)
	// Write all failures to a file so they can be analyzed without log caps.
	if dump := os.Getenv("HTML_SMOKE_FAILURES_OUT"); dump != "" {
		_ = os.WriteFile(dump, []byte(strings.Join(failures, "\n")+"\n"), 0o644)
	}
	if len(failures) > 0 {
		max := 30
		if len(failures) < max {
			max = len(failures)
		}
		for _, f := range failures[:max] {
			t.Log(f)
		}
		if len(failures) > max {
			t.Logf("... and %d more", len(failures)-max)
		}
		t.Fail()
	}
}
