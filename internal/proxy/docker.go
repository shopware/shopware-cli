package proxy

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/oci"
)

// runDocker runs `<oci-runtime> <args...>` and returns its combined output.
func runDocker(ctx context.Context, args ...string) (string, error) {
	runtime := oci.FromContext(ctx)

	out, err := runtime.Command(ctx, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w\n%s", runtime.Binary(), args, err, out)
	}

	return string(out), nil
}
