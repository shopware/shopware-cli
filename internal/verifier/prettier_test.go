package verifier

import (
	"errors"
	"reflect"
	"testing"
)

// TestPrettierExpectedExit checks successful, formatting, and critical exit statuses.
func TestPrettierExpectedExit(t *testing.T) {
	p := Prettier{}

	tests := []struct {
		name string
		code int
		want bool
	}{
		{"success", 0, true},
		{"formatting violations", 1, true},
		{"prettier error", 2, false},
		{"unexpected status", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.code != 0 {
				err = testExitError(t, tt.code)
			}
			if got := p.isExpectedPrettierExit(err, true); got != tt.want {
				t.Errorf("isExpectedPrettierExit(exit %d) = %t, want %t", tt.code, got, tt.want)
			}
		})
	}

	t.Run("non-exit error", func(t *testing.T) {
		if p.isExpectedPrettierExit(errors.New("failed to start"), true) {
			t.Fatal("unexpected success for non-exit error")
		}
	})

	t.Run("wrapped exit error", func(t *testing.T) {
		if !p.isExpectedPrettierExit(errors.Join(errors.New("context"), testExitError(t, 1)), true) {
			t.Fatal("expected wrapped exit status 1 to be accepted")
		}
	})

	t.Run("write mode rejects exit one", func(t *testing.T) {
		if p.isExpectedPrettierExit(testExitError(t, 1), false) {
			t.Fatal("unexpected success for write mode exit status 1")
		}
	})
}

func TestPrettierParse(t *testing.T) {
	p := Prettier{}
	tests := []struct {
		name    string
		output  string
		want    []PrettierOutputFile
		wantErr bool
	}{
		{
			name:   "empty output",
			output: "\n \t\n",
		},
		{
			name:   "list-different output",
			output: "src/example.js\nsrc/my extension/plugin.js\n",
			want: []PrettierOutputFile{
				{Path: "src/example.js"},
				{Path: "src/my extension/plugin.js"},
			},
		},
		{
			name:   "write output",
			output: "src/example.js 42ms\nsrc/my extension/plugin.js 18ms (unchanged)\n",
			want: []PrettierOutputFile{
				{Path: "src/example.js", Time: 42},
				{Path: "src/my extension/plugin.js", Time: 18, Unchanged: true},
			},
		},
		{
			name:   "blank lines and surrounding whitespace",
			output: "\n\n src/example.js 0ms \n\n",
			want:   []PrettierOutputFile{{Path: "src/example.js", Time: 0}},
		},
		{
			name:    "invalid processing time",
			output:  "src/example.js invalidms (unchanged)",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Parse(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %t", tt.output, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got.Files, tt.want) {
				t.Errorf("Parse(%q) files = %#v, want %#v", tt.output, got.Files, tt.want)
			}
		})
	}
}
