package acceptance

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"github.com/shyim/go-composer/repository"

	"github.com/shopware/shopware-cli/cmd"
)

// runCLI is what the scripts execute: main, with Packagist requests sent to the harness's fake.
// It lives outside the test files so coverage runs instrument this package.
func runCLI() {
	if fake, err := url.Parse(os.Getenv("ACCEPTANCE_PACKAGIST_URL")); err == nil && fake.Host != "" {
		upstream, _ := url.Parse(repository.PackagistURL)
		http.DefaultTransport = fakePackagist{host: upstream.Host, fake: fake, next: http.DefaultTransport}
	}

	os.Exit(cmd.Execute(context.Background()))
}

// fakePackagist sends requests for the Packagist host to the fake repository and passes the rest on.
type fakePackagist struct {
	host string
	fake *url.URL
	next http.RoundTripper
}

func (f fakePackagist) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == f.host {
		req = req.Clone(req.Context())
		req.URL.Scheme, req.URL.Host, req.Host = f.fake.Scheme, f.fake.Host, f.fake.Host
	}

	return f.next.RoundTrip(req)
}
