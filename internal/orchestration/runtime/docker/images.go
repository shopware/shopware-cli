package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var _ runtime.ContainerImages = (*Runtime)(nil)

// InspectImage returns metadata from the local image store.
func (d *Runtime) InspectImage(ctx context.Context, reference string) (runtime.ContainerImageState, error) {
	if !validName(reference) {
		return runtime.ContainerImageState{}, errors.New("invalid image reference")
	}
	out, diagnostic, err := d.command(ctx, "image", "inspect", "--", reference)
	if err != nil {
		if absent(diagnostic, "image", "inspect", reference) && ctx.Err() == nil {
			return runtime.ContainerImageState{}, errors.Join(fmt.Errorf("%w: %s", runtime.ErrNotFound, reference), err)
		}
		return runtime.ContainerImageState{}, fmt.Errorf("cannot inspect image: %w", err)
	}
	var images []struct {
		ID          string   `json:"Id"`
		RepoTags    []string `json:"RepoTags"`
		RepoDigests []string `json:"RepoDigests"`
	}
	if err := json.Unmarshal([]byte(out), &images); err != nil {
		return runtime.ContainerImageState{}, fmt.Errorf("cannot decode image inspection: %w", err)
	}
	if len(images) != 1 || images[0].ID == "" {
		return runtime.ContainerImageState{}, errors.New("invalid image inspection result")
	}
	state := runtime.ContainerImageState{ID: images[0].ID}
	for _, ref := range append(images[0].RepoTags, images[0].RepoDigests...) {
		if ref != "" && !strings.HasPrefix(ref, "<none>") {
			state.References = append(state.References, ref)
		}
	}
	slices.Sort(state.References)
	state.References = slices.Compact(state.References)
	return state, nil
}

// PullImage pulls an image into the selected engine.
func (d *Runtime) PullImage(ctx context.Context, reference string, options runtime.PullImageOptions) error {
	if !validName(reference) {
		return errors.New("invalid image reference")
	}
	if err := d.run(ctx, options.Stdout, options.Stderr, "image", "pull", "--", reference); err != nil {
		return fmt.Errorf("cannot pull image: %w", err)
	}
	return nil
}

// BuildImage returns this build's ID while streaming normal output.
// The Docker client must run on the CLI host to access the context and ID file.
// The Docker daemon may be remote.
func (d *Runtime) BuildImage(ctx context.Context, options runtime.BuildImageOptions) (id string, resultErr error) {
	if !filepath.IsAbs(options.ContextDir) || hasControl(options.ContextDir) {
		return "", errors.New("build context must be an absolute path")
	}
	for _, tag := range options.Tags {
		if !validName(tag) {
			return "", errors.New("invalid image tag")
		}
	}
	dockerfile := options.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if !filepath.IsLocal(dockerfile) || hasControl(dockerfile) {
		return "", errors.New("dockerfile must be relative to the build context")
	}
	tempDir, err := os.MkdirTemp("", "shopware-cli-image-build-")
	if err != nil {
		return "", fmt.Errorf("cannot create image ID directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("cannot remove image ID directory: %w", err))
		}
	}()
	idFile := filepath.Join(tempDir, "iid")
	args := []string{"image", "build", "--iidfile", idFile}
	for _, tag := range options.Tags {
		args = append(args, "--tag", tag)
	}
	args = append(args, "--file", filepath.Join(options.ContextDir, dockerfile), "--", options.ContextDir)
	if err := d.run(ctx, options.Stdout, options.Stderr, args...); err != nil {
		return "", fmt.Errorf("cannot build image: %w", err)
	}
	data, err := os.ReadFile(idFile)
	if err != nil {
		return "", fmt.Errorf("cannot read built image ID: %w", err)
	}
	id = strings.TrimSpace(string(data))
	if id == "" || len(strings.Fields(id)) != 1 || hasControl(id) {
		return "", errors.New("invalid built image ID")
	}
	return id, nil
}
