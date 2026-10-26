package acceptance

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/shyim/go-composer/repository"

	"github.com/shopware/shopware-cli/cmd"
)

// staticDataHost serves the Shopware-to-PHP version map the builtin PHP linter looks up.
const staticDataHost = "raw.githubusercontent.com"

// runCLI is what the scripts execute: main, behind a transport that keeps every CLI process offline.
// It lives outside the test files so coverage runs instrument this package.
func runCLI() {
	http.DefaultTransport = newOfflineTransport(os.Getenv("ACCEPTANCE_FAKE_URL"), os.Getenv("ACCEPTANCE_REFUSED_LOG"), http.DefaultTransport)

	os.Exit(cmd.Execute(context.Background()))
}

// offlineTransport serves the faked hosts from the harness, passes loopback requests on and refuses every other host.
type offlineTransport struct {
	fake    *url.URL
	faked   map[string]bool
	refused string
	next    http.RoundTripper
}

func newOfflineTransport(fakeURL, refusedLog string, next http.RoundTripper) offlineTransport {
	t := offlineTransport{refused: refusedLog, next: next}
	if fake, err := url.Parse(fakeURL); err == nil && fake.Host != "" {
		upstream, _ := url.Parse(repository.PackagistURL)
		t.fake = fake
		t.faked = map[string]bool{upstream.Host: true, staticDataHost: true}
	}

	return t
}

func (t offlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch {
	case t.faked[req.URL.Host]:
		req = req.Clone(req.Context())
		req.URL.Scheme, req.URL.Host, req.Host = t.fake.Scheme, t.fake.Host, t.fake.Host
	case !isLoopback(req.URL.Hostname()):
		t.record(req)

		return nil, fmt.Errorf("acceptance scripts run offline, request to %s refused", req.URL.Host)
	}

	return t.next.RoundTrip(req)
}

// record appends the refused request to the log the harness checks when the script ends.
func (t offlineTransport) record(req *http.Request) {
	if t.refused == "" {
		return
	}

	f, err := os.OpenFile(t.refused, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	_, _ = fmt.Fprintf(f, "%s %s\n", req.Method, req.URL)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
