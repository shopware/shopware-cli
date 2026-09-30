package system

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
)

// More workers increase filesystem contention, especially when copying many
// small files into the same directory. Keep both I/O and queued work bounded.
const copyFileWorkers = 4

// Fallback copies check for cancellation between chunks of this size.
const copyChunkSize = 8 << 20

type copyJob struct {
	src, dst string
	entry    fs.DirEntry
}

func CopyFiles(ctx context.Context, currentPath string, targetPath string) error {
	currentPath, err := prepareCopySource(currentPath)
	if err != nil {
		return err
	}
	if currentPath == "" {
		return nil
	}
	targetPath, err = resolveCopyTarget(targetPath)
	if err != nil {
		return err
	}
	if isCopyTargetInsideSource(currentPath, targetPath) {
		return fmt.Errorf("target directory %q must not be inside source directory %q", targetPath, currentPath)
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	return copyDirectoryTree(ctx, currentPath, targetPath)
}

func prepareCopySource(currentPath string) (string, error) {
	// When the currentPath folder does not exist, return
	sourceInfo, err := os.Stat(currentPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to access source directory: %w", err)
	}
	if !sourceInfo.IsDir() {
		return "", fmt.Errorf("source %q is not a directory", currentPath)
	}

	currentPath, err = filepath.Abs(currentPath)
	if err != nil {
		return "", err
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return "", err
	}
	return currentPath, nil
}

func copyDirectoryTree(ctx context.Context, currentPath string, targetPath string) error {
	jobs := make(chan copyJob, copyFileWorkers)
	group, ctx := errgroup.WithContext(ctx)
	for range copyFileWorkers {
		group.Go(func() error {
			return runCopyJobs(ctx, jobs)
		})
	}

	// WalkDir avoids statting every entry on the walking goroutine. Workers
	// fetch file metadata once and reuse it for symlinks and permissions.
	walkErr := filepath.WalkDir(currentPath, walkCopySource(currentPath, targetPath, jobs, ctx))
	close(jobs)

	// Always join the workers, including when walking fails. Return the actual
	// copy error in preference to cancellation caused by that error.
	if err := group.Wait(); err != nil {
		return err
	}
	return walkErr
}

func runCopyJobs(ctx context.Context, jobs <-chan copyJob) error {
	for job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := job.entry.Info()
		if err != nil {
			return fmt.Errorf("failed to access path %q: %w", job.src, err)
		}
		if err := copyFile(ctx, job.src, job.dst, info); err != nil {
			return fmt.Errorf("failed to copy %q to %q: %w", job.src, job.dst, err)
		}
	}
	return nil
}

func walkCopySource(currentPath string, targetPath string, jobs chan<- copyJob, ctx context.Context) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to access path %q: %w", path, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		// Get the relative path
		relPath, err := filepath.Rel(currentPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		// Skip VCS and dev environment folders at any depth, but never the source root
		if relPath != "." && entry.IsDir() && isSkippedCopyDir(entry.Name()) {
			return filepath.SkipDir
		}

		// Construct target path
		targetFilePath := filepath.Join(targetPath, relPath)

		// If it's a directory, create it in target
		if entry.IsDir() {
			if relPath == "." {
				return nil
			}
			return ensureCopyTargetDir(targetFilePath)
		}

		select {
		case jobs <- copyJob{src: path, dst: targetFilePath, entry: entry}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func isSkippedCopyDir(name string) bool {
	return name == ".devenv" || name == ".direnv" || name == ".git"
}

func ensureCopyTargetDir(targetFilePath string) error {
	// The walk has already created the parent. Avoid MkdirAll's
	// extra stat calls for the usual case of a fresh destination.
	if err := os.Mkdir(targetFilePath, 0o755); err != nil {
		if !os.IsExist(err) {
			return err
		}
		destinationInfo, lstatErr := os.Lstat(targetFilePath)
		if lstatErr != nil {
			return lstatErr
		}
		if destinationInfo.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(targetFilePath); err != nil {
				return err
			}
			return os.Mkdir(targetFilePath, 0o755)
		}
		if !destinationInfo.IsDir() {
			return err
		}
	}
	return nil
}

func isCopyTargetInsideSource(src, dst string) bool {
	rel, err := filepath.Rel(src, dst)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Resolve existing ancestors before creating the target, so a directory alias
// cannot cause us to create a destination inside the source being walked.
func resolveCopyTarget(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		parent := filepath.Dir(path)
		if !os.IsNotExist(err) || parent == path {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(path), suffix)
		path = parent
	}
}

func copyFile(ctx context.Context, src, dst string, info fs.FileInfo) error {
	// If it's a symlink, create a new symlink
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		err = os.Symlink(linkTarget, dst)
		if os.IsExist(err) {
			if err := removeCopyDestination(dst, info); err != nil {
				return err
			}
			err = os.Symlink(linkTarget, dst)
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type: %s", describeFileType(info.Mode()))
	}

	// Try a copy-on-write clone first (macOS/APFS), falling back to io.Copy
	// for unsupported filesystems or cross-device copies.
	err := cloneFile(src, dst)
	if os.IsExist(err) {
		if err := removeCopyDestination(dst, info); err != nil {
			return err
		}
		err = cloneFile(src, dst)
	}
	if err == nil {
		// clonefile preserves ordinary permissions, but clears these bits.
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			return os.Chmod(dst, info.Mode())
		}
		return nil
	}

	return copyFileFallback(ctx, src, dst, info)
}

// describeFileType names special files for errors instead of printing raw mode bits.
func describeFileType(mode fs.FileMode) string {
	switch {
	case mode&os.ModeNamedPipe != 0:
		return "named pipe"
	case mode&os.ModeSocket != 0:
		return "socket"
	case mode&os.ModeCharDevice != 0:
		return "character device"
	case mode&os.ModeDevice != 0:
		return "block device"
	default:
		return mode.Type().String()
	}
}

// Only remove an existing file after creation reports EEXIST. Never follow a
// destination symlink, remove a directory, or unlink the source itself.
func removeCopyDestination(dst string, sourceInfo fs.FileInfo) error {
	destinationInfo, err := os.Lstat(dst)
	if err != nil {
		return err
	}
	if destinationInfo.IsDir() {
		return fmt.Errorf("cannot overwrite directory %q with a file", dst)
	}
	if os.SameFile(sourceInfo, destinationInfo) {
		return fmt.Errorf("source and destination %q are the same file", dst)
	}
	return os.Remove(dst)
}

// copyFileFallback retains io.Copy's platform-specific optimizations when
// cloning is unavailable (e.g. cross-device or non-APFS filesystems).
func copyFileFallback(ctx context.Context, src, dst string, info fs.FileInfo) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}

	// Create target file
	targetFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if os.IsExist(err) {
		err = removeCopyDestination(dst, info)
		if err == nil {
			targetFile, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		}
	}
	if err != nil {
		_ = sourceFile.Close()
		return fmt.Errorf("failed to create target file: %w", err)
	}

	// Copy the contents
	copyErr := copyContents(ctx, targetFile, sourceFile)
	// Creating with the source mode is still subject to umask; apply the
	// complete mode after writing, which can otherwise clear setuid/setgid.
	var chmodErr error
	if copyErr == nil {
		chmodErr = targetFile.Chmod(info.Mode())
	}
	closeTargetErr := targetFile.Close()
	closeSourceErr := sourceFile.Close()

	if copyErr != nil {
		return fmt.Errorf("failed to copy file contents: %w", copyErr)
	}
	if chmodErr != nil {
		return fmt.Errorf("failed to set target file permissions: %w", chmodErr)
	}
	if closeTargetErr != nil {
		return fmt.Errorf("failed to close target file: %w", closeTargetErr)
	}
	if closeSourceErr != nil {
		return fmt.Errorf("failed to close source file: %w", closeSourceErr)
	}

	return nil
}

// copyContents stops a large copy between chunks when ctx is canceled.
// io.CopyN passes an io.LimitedReader, which *os.File.ReadFrom still
// accelerates with copy_file_range or sendfile.
func copyContents(ctx context.Context, dst io.Writer, src io.Reader) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.CopyN(dst, src, copyChunkSize); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close()
	}()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}
