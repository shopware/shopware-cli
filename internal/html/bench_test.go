package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BenchmarkStorefront is opt-in via HTML_SMOKE_CORPUS. It parses every
// .twig file in the corpus once and reports total time + bytes/sec.
func BenchmarkStorefront(b *testing.B) {
	root := os.Getenv("HTML_SMOKE_CORPUS")
	if root == "" {
		b.Skip("set HTML_SMOKE_CORPUS=/path/to/storefront")
	}
	var files [][]byte
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, _ error) error {
		if d == nil || d.IsDir() || !strings.HasSuffix(path, ".twig") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files = append(files, data)
		return nil
	})
	totalBytes := 0
	for _, f := range files {
		totalBytes += len(f)
	}
	b.Logf("loaded %d files, %d bytes total", len(files), totalBytes)
	b.ResetTimer()
	start := time.Now()
	for n := 0; n < b.N; n++ {
		for _, f := range files {
			_, err := NewParser(string(f))
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	elapsed := time.Since(start)
	b.ReportMetric(float64(totalBytes*b.N)/elapsed.Seconds()/1024/1024, "MB/s")
}
