package cmd

import "testing"

func TestRootCommandShortcuts(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"p"}, want: "project"},
		{args: []string{"e"}, want: "extension"},
		{args: []string{"a"}, want: "account"},
		{args: []string{"p", "e", "l"}, want: "list"},
		{args: []string{"p", "co"}, want: "config"},
		{args: []string{"p", "cc"}, want: "clear-cache"},
		{args: []string{"p", "aw"}, want: "admin-watch"},
		{args: []string{"e", "gv"}, want: "get-version"},
		{args: []string{"a", "p", "e", "i", "p"}, want: "pull"},
	}

	for _, tt := range tests {
		command, _, err := rootCmd.Find(tt.args)
		if err != nil {
			t.Fatalf("Find(%v) returned error: %v", tt.args, err)
		}
		if command == nil {
			t.Fatalf("Find(%v) returned no command", tt.args)
		}
		if command.Name() != tt.want {
			t.Fatalf("Find(%v) resolved to %q, want %q", tt.args, command.Name(), tt.want)
		}
	}
}
