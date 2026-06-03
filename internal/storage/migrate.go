package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// StoreTarget describes where a datastore's files should be migrated to.
type StoreTarget struct {
	Datastore Datastore
	// Bucket is the destination bucket.
	Bucket string
	// Root is an optional key prefix inside the bucket. Object keys become
	// "<Root>/<file key>".
	Root string
	// PublicURL is the public base URL files will be served from after the
	// migration (used only for config generation).
	PublicURL string
}

// objectKey returns the full object key for a local file in this target.
func (t StoreTarget) objectKey(f LocalFile) string {
	root := strings.Trim(t.Root, "/")
	if root == "" {
		return f.Key
	}
	return root + "/" + f.Key
}

// MigrateOptions controls a migration run.
type MigrateOptions struct {
	// Concurrency is the number of parallel uploads. Defaults to 8 when <= 0.
	Concurrency int
	// SkipExisting skips files that already exist in the target with the same
	// size, making the migration resumable.
	SkipExisting bool
	// DryRun reports what would happen without uploading anything.
	DryRun bool
	// PublicACL sets a public-read ACL on objects of public datastores.
	PublicACL bool
}

// ProgressEvent reports the result of processing a single file.
type ProgressEvent struct {
	Datastore string
	Key       string
	Size      int64
	Skipped   bool
	Err       error
}

// MigrateSummary aggregates the result of a migration run.
type MigrateSummary struct {
	Uploaded     int
	Skipped      int
	Failed       int
	BytesWritten int64
}

// Migrate uploads all files of the given targets to S3. progress, when not nil,
// is called once per file (from multiple goroutines, so it must be safe for
// concurrent use). It returns a summary and the first error that aborted the
// run, if any.
func Migrate(ctx context.Context, client *S3Client, targets []StoreTarget, scans map[string]ScanResult, opts MigrateOptions, progress func(ProgressEvent)) (MigrateSummary, error) {
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}

	type job struct {
		target StoreTarget
		file   LocalFile
	}

	jobs := make(chan job)
	var summary MigrateSummary
	var uploaded, skipped, failed int64
	var bytesWritten int64

	report := func(ev ProgressEvent) {
		if progress != nil {
			progress(ev)
		}
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr error
	var errOnce sync.Once

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if ctx.Err() != nil {
				return
			}

			key := j.target.objectKey(j.file)
			ds := j.target.Datastore.Name

			if opts.SkipExisting && !opts.DryRun {
				size, exists, err := client.Stat(ctx, j.target.Bucket, key)
				if err == nil && exists && size == j.file.Size {
					atomic.AddInt64(&skipped, 1)
					report(ProgressEvent{Datastore: ds, Key: key, Size: j.file.Size, Skipped: true})
					continue
				}
			}

			if opts.DryRun {
				atomic.AddInt64(&uploaded, 1)
				report(ProgressEvent{Datastore: ds, Key: key, Size: j.file.Size})
				continue
			}

			if err := uploadFile(ctx, client, j.target, j.file, key, opts); err != nil {
				atomic.AddInt64(&failed, 1)
				report(ProgressEvent{Datastore: ds, Key: key, Size: j.file.Size, Err: err})
				errOnce.Do(func() {
					firstErr = fmt.Errorf("uploading %s: %w", key, err)
					cancel()
				})
				continue
			}

			atomic.AddInt64(&uploaded, 1)
			atomic.AddInt64(&bytesWritten, j.file.Size)
			report(ProgressEvent{Datastore: ds, Key: key, Size: j.file.Size})
		}
	}

	wg.Add(concurrency)
	for range concurrency {
		go worker()
	}

	for _, target := range targets {
		scan, ok := scans[target.Datastore.Name]
		if !ok {
			continue
		}
		for _, file := range scan.Files {
			if ctx.Err() != nil {
				break
			}
			jobs <- job{target: target, file: file}
		}
	}
	close(jobs)

	wg.Wait()

	summary.Uploaded = int(uploaded)
	summary.Skipped = int(skipped)
	summary.Failed = int(failed)
	summary.BytesWritten = bytesWritten

	return summary, firstErr
}

func uploadFile(ctx context.Context, client *S3Client, target StoreTarget, file LocalFile, key string, opts MigrateOptions) error {
	f, err := os.Open(file.AbsPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return client.Upload(ctx, target.Bucket, key, f, UploadOptions{
		PublicACL: opts.PublicACL && target.Datastore.Public,
	})
}
