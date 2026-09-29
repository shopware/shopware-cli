package system

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkCopyFiles(b *testing.B) {
	for _, workload := range []struct {
		name        string
		files, size int
	}{
		{name: "small", files: 3000, size: 1024},
		{name: "medium", files: 3000, size: 16 * 1024},
		{name: "large", files: 1, size: 128 * 1024 * 1024},
	} {
		b.Run(workload.name, func(b *testing.B) {
			base := b.TempDir()
			src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
			data := bytes.Repeat([]byte("x"), workload.size)
			for i := range workload.files {
				dir := filepath.Join(src, strconv.Itoa(i/100))
				require.NoError(b, os.MkdirAll(dir, 0o755))
				require.NoError(b, os.WriteFile(filepath.Join(dir, strconv.Itoa(i)), data, 0o644))
			}
			b.ReportAllocs()
			for b.Loop() {
				err := CopyFiles(b.Context(), src, dst)
				b.StopTimer()
				require.NoError(b, err)
				require.NoError(b, os.RemoveAll(dst))
				b.StartTimer()
			}
		})
	}
}
