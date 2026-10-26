// Package acceptance runs the built CLI the way a user does and checks exit
// codes and output.
//
// Each testdata/*.txtar file is a script for [testscript]: commands,
// assertions on exit status, stdout, stderr and files, and the fixtures the
// script needs. Scripts match values the command echoes back and structural
// markers such as the Usage: heading, never sentences.
//
// Scripts run offline by construction: inside each CLI process, Packagist and
// the Shopware static data are served by fakes on loopback, and a request to
// any other host is refused and fails the script when it ends. PATH is the
// host's, so a script that needs a tool such as php gates its commands with
// the [exec:php] condition.
//
// Run every script, or one script with its transcript:
//
//	go test ./acceptance
//	go test ./acceptance -run TestScripts/<name> -v
//
// [testscript]: https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
package acceptance
