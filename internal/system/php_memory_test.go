package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParsePHPMemoryLimit(t *testing.T) {
	for _, tc := range []struct {
		input   string
		want    int64
		invalid bool
	}{
		{"128M", 134217728, false}, {"512M", 536870912, false},
		{"1G", 1073741824, false}, {"512m", 536870912, false},
		{"524288K", 536870912, false}, {"536870912", 536870912, false},
		{"-1", -1, false}, {"0", 0, false}, {" 512M ", 536870912, false},
		{"0x20000000", 536870912, false},
		{"", 0, true}, {"garbage", 0, true}, {"-2", 0, true},
		{"9223372036854775807G", 0, true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parsePHPMemoryLimit(tc.input)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got (%d, %v), want (%d, invalid=%v)", got, err, tc.want, tc.invalid)
			}
		})
	}
}

func TestValidatePHPCreateMemory(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shell unavailable")
	}
	for _, tc := range []struct {
		name, limit, override string
		fail                  bool
	}{
		{"low", "128M", "", true}, {"boundary", "512M", "", false},
		{"high", "1G", "", false}, {"unlimited", "-1", "", false},
		{"override", "128M", "512M", false},
		{"lower override", "1G", "128M", true},
		{"unlimited override", "128M", "-1", false},
		{"empty override", "128M", "", true},
		{"zero override follows Flex", "512M", "0", false},
		{"invalid", "bad", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COMPOSER_MEMORY_LIMIT", tc.override)
			// A named executable outside PATH ensures the selected PHP is used.
			binary := filepath.Join(t.TempDir(), "selected-php")
			script := "#!" + sh + "\nlimit='" + tc.limit + "'\nif [ \"$1\" = '-d' ]; then\n limit=${2#memory_limit=}\n shift 2\nfi\n[ \"$1\" = '-r' ] || exit 2\nprintf '{\"limit\":\"%s\",\"ini\":\"/test/php.ini\"}' \"$limit\"\n"
			if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			err := ValidatePHPCreateMemory(t.Context(), binary)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v; want failure=%v", err, tc.fail)
			}
		})
	}
}

func TestValidatePHPCreateMemoryProbeFailures(t *testing.T) {
	t.Setenv("COMPOSER_MEMORY_LIMIT", "")
	if err := ValidatePHPCreateMemory(t.Context(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing PHP must fail")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("shell unavailable")
	}
	for _, body := range []string{"exit 1", "echo invalid-json", "echo '{}'"} {
		binary := filepath.Join(t.TempDir(), "php")
		if err := os.WriteFile(binary, []byte("#!"+sh+"\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ValidatePHPCreateMemory(t.Context(), binary); err == nil {
			t.Fatalf("probe %q must fail", body)
		}
	}
}
