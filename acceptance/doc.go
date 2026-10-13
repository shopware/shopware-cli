// Package acceptance runs the built CLI the way a user does and checks exit
// codes and output.
//
// Each testdata/*.txtar file is a script for [testscript]: commands,
// assertions on exit status, stdout, stderr and files, and the fixtures the
// script needs. Scripts assert behaviour, never message wording, and run
// without network access.
//
// Run every script, or one script with its transcript:
//
//	go test ./acceptance
//	go test ./acceptance -run TestScripts/<name> -v
//
// [testscript]: https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
package acceptance
