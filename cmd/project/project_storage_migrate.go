package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/storage"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var (
	storageSectionStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	storageLabelStyle   = tui.BoldStyle
)

// providerPreset describes sensible defaults for a storage provider so users do
// not have to know the path-style / endpoint quirks upfront.
type providerPreset struct {
	id        string
	label     string
	pathStyle bool
	// endpointHint is shown as a placeholder/help when asking for the endpoint.
	endpointHint string
	// note is printed after selection to explain provider specifics.
	note string
}

var providerPresets = []providerPreset{
	{id: "aws", label: "Amazon S3", pathStyle: false, endpointHint: "(leave empty for AWS)", note: "Region is required. Endpoint can stay empty."},
	{id: "minio", label: "MinIO", pathStyle: true, endpointHint: "http://localhost:9000", note: "Uses path-style addressing."},
	{id: "r2", label: "Cloudflare R2", pathStyle: true, endpointHint: "https://<account-id>.r2.cloudflarestorage.com", note: "Use region 'auto'. R2 ignores object ACLs; make the bucket public via R2 settings."},
	{id: "digitalocean", label: "DigitalOcean Spaces", pathStyle: false, endpointHint: "https://<region>.digitaloceanspaces.com", note: "Region is the Spaces region, e.g. fra1."},
	{id: "hetzner", label: "Hetzner Object Storage", pathStyle: true, endpointHint: "https://<region>.your-objectstorage.com", note: "Uses path-style addressing."},
	{id: "custom", label: "Other S3-compatible", pathStyle: true, endpointHint: "https://...", note: ""},
}

func presetByID(id string) (providerPreset, bool) {
	for _, p := range providerPresets {
		if p.id == id {
			return p, true
		}
	}
	return providerPreset{}, false
}

type storageMigrateFlags struct {
	stores       []string
	provider     string
	endpoint     string
	region       string
	bucket       string
	accessKey    string
	secretKey    string
	pathStyle    bool
	root         string
	publicURL    string
	publicACL    bool
	concurrency  int
	skipExisting bool
	dryRun       bool
	configOut    string
	printConfig  bool
	assumeYes    bool
}

var storageMigrateCmd = &cobra.Command{
	Use:   "migrate [project-dir]",
	Short: "Guided migration of local file storages to S3",
	Long: `Walks you through moving Shopware's file datastores from the local disk to an
S3 compatible storage. It helps you decide which datastores to move, validates
the target bucket, copies the files and generates a reviewable Shopware
filesystem configuration.

Local files are never deleted; the migration only copies them.

For non-interactive use (CI), provide --stores, --bucket and the connection
flags. Credentials can also be passed via the STORAGE_S3_ACCESS_KEY and
STORAGE_S3_SECRET_KEY environment variables.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := readStorageMigrateFlags(cmd)
		ctx := cmd.Context()
		interactive := system.IsInteractionEnabled(ctx)

		projectRoot, err := resolveStorageProjectRoot(args)
		if err != nil {
			return err
		}

		// Phase 1 — Detect & explain.
		stores, err := selectDatastores(projectRoot, flags, interactive)
		if err != nil {
			return err
		}
		if len(stores) == 0 {
			return fmt.Errorf("no datastores selected to migrate")
		}

		// Phase 5 (scan) up front so the plan can be shown early.
		scans := make(map[string]storage.ScanResult, len(stores))
		var totalFiles int
		var totalSize int64
		for _, ds := range stores {
			scan, err := storage.ScanLocal(projectRoot, ds)
			if err != nil {
				return fmt.Errorf("scanning %s: %w", ds.Name, err)
			}
			scans[ds.Name] = scan
			totalFiles += len(scan.Files)
			totalSize += scan.TotalSize
		}

		printPlan(stores, scans)

		if totalFiles == 0 {
			fmt.Println()
			fmt.Println(tui.SecondaryText.Render("No local files found for the selected datastores. Only the configuration will be generated."))
		}

		// Phase 3 — Collect connection + per-store targets.
		conn, err := collectConnection(flags, interactive)
		if err != nil {
			return err
		}

		targets, err := collectTargets(conn, stores, flags, interactive)
		if err != nil {
			return err
		}

		// Phase 4 — Validate target(s) (skipped on dry-run).
		var client *storage.S3Client
		if !flags.dryRun {
			client, err = storage.NewS3Client(conn)
			if err != nil {
				return err
			}
			if err := validateTargets(ctx, client, targets); err != nil {
				return err
			}
		}

		// Phase 6 — Migrate.
		if flags.dryRun {
			fmt.Println()
			fmt.Println(tui.YellowText.Render("Dry-run: no files were uploaded."))
		} else if totalFiles > 0 {
			if !flags.assumeYes && interactive {
				proceed := false
				if err := huh.NewConfirm().
					Title(fmt.Sprintf("Upload %d files (%s) to S3 now?", totalFiles, humanize.Bytes(uint64(totalSize)))).
					Description("Local files are kept; this only copies them to the bucket.").
					Value(&proceed).
					WithTheme(tui.ShopwareTheme()).
					Run(); err != nil {
					return err
				}
				if !proceed {
					return fmt.Errorf("migration cancelled")
				}
			}

			summary, err := runMigration(ctx, client, targets, scans, flags, totalFiles)
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Printf("%s Uploaded %d files (%s)", tui.Checkmark, summary.Uploaded, humanize.Bytes(uint64(summary.BytesWritten)))
			if summary.Skipped > 0 {
				fmt.Printf(", skipped %d already present", summary.Skipped)
			}
			fmt.Println()
		}

		// Phase 7 — Generate config.
		if err := writeStorageConfig(ctx, projectRoot, conn, targets, flags, interactive); err != nil {
			return err
		}

		// Phase 8 — Post steps.
		printPostSteps(projectRoot, stores)

		return nil
	},
}

func readStorageMigrateFlags(cmd *cobra.Command) storageMigrateFlags {
	f := cmd.Flags()
	flags := storageMigrateFlags{}
	flags.stores, _ = f.GetStringSlice("stores")
	flags.provider, _ = f.GetString("provider")
	flags.endpoint, _ = f.GetString("endpoint")
	flags.region, _ = f.GetString("region")
	flags.bucket, _ = f.GetString("bucket")
	flags.accessKey, _ = f.GetString("access-key")
	flags.secretKey, _ = f.GetString("secret-key")
	flags.pathStyle, _ = f.GetBool("path-style")
	flags.root, _ = f.GetString("root")
	flags.publicURL, _ = f.GetString("public-url")
	flags.publicACL, _ = f.GetBool("public-acl")
	flags.concurrency, _ = f.GetInt("concurrency")
	flags.skipExisting, _ = f.GetBool("skip-existing")
	flags.dryRun, _ = f.GetBool("dry-run")
	flags.configOut, _ = f.GetString("config-out")
	flags.printConfig, _ = f.GetBool("print-config")
	flags.assumeYes, _ = f.GetBool("yes")

	if flags.accessKey == "" {
		flags.accessKey = os.Getenv(storage.DefaultAccessKeyEnv)
	}
	if flags.secretKey == "" {
		flags.secretKey = os.Getenv(storage.DefaultSecretKeyEnv)
	}

	return flags
}

func resolveStorageProjectRoot(args []string) (string, error) {
	if len(args) > 0 {
		return filepath.Abs(args[0])
	}
	return findClosestShopwareProject()
}

// selectDatastores resolves which datastores to migrate, either from the
// --stores flag or via an interactive multi-select that explains each option.
func selectDatastores(projectRoot string, flags storageMigrateFlags, interactive bool) ([]storage.Datastore, error) {
	if len(flags.stores) > 0 {
		var out []storage.Datastore
		for _, name := range flags.stores {
			ds, ok := storage.DatastoreByName(strings.TrimSpace(name))
			if !ok {
				return nil, fmt.Errorf("unknown datastore %q (known: %s)", name, knownDatastoreNames())
			}
			out = append(out, ds)
		}
		return out, nil
	}

	if !interactive {
		return nil, fmt.Errorf("no datastores given; pass --stores (e.g. --stores public,private)")
	}

	fmt.Println(storageSectionStyle.Render("Datastores"))
	fmt.Println()

	options := make([]huh.Option[string], 0, len(storage.KnownDatastores))
	var defaults []string
	for _, ds := range storage.KnownDatastores {
		scan, err := storage.ScanLocal(projectRoot, ds)
		if err != nil {
			return nil, err
		}

		label := fmt.Sprintf("%s — %s", ds.Title, ds.Description)
		if len(scan.Files) > 0 {
			label += fmt.Sprintf(" (%d files, %s)", len(scan.Files), humanize.Bytes(uint64(scan.TotalSize)))
		} else {
			label += " (empty)"
		}
		if ds.RebuildCommand != "" {
			label += tui.SecondaryText.Render(fmt.Sprintf(" — tip: regenerate with bin/console %s instead", ds.RebuildCommand))
		}

		options = append(options, huh.NewOption(label, ds.Name))
		if ds.Recommended && len(scan.Files) > 0 {
			defaults = append(defaults, ds.Name)
		}
	}

	selected := defaults
	if err := huh.NewMultiSelect[string]().
		Title("Which datastores do you want to migrate to S3?").
		Description("Pre-selected: datastores that hold user data and have files on disk.").
		Options(options...).
		Value(&selected).
		WithTheme(tui.ShopwareTheme()).
		Run(); err != nil {
		return nil, err
	}

	var out []storage.Datastore
	for _, ds := range storage.KnownDatastores {
		if slices.Contains(selected, ds.Name) {
			out = append(out, ds)
		}
	}
	return out, nil
}

func knownDatastoreNames() string {
	names := make([]string, 0, len(storage.KnownDatastores))
	for _, ds := range storage.KnownDatastores {
		names = append(names, ds.Name)
	}
	return strings.Join(names, ", ")
}

func printPlan(stores []storage.Datastore, scans map[string]storage.ScanResult) {
	fmt.Println()
	fmt.Println(storageSectionStyle.Render("Migration plan"))
	fmt.Println()
	for _, ds := range stores {
		scan := scans[ds.Name]
		fmt.Printf("  %s %-10s %d files, %s\n",
			tui.Checkmark,
			ds.Name,
			len(scan.Files),
			humanize.Bytes(uint64(scan.TotalSize)),
		)
	}
}

// collectConnection builds the S3 connection from flags, prompting for missing
// values when interactive.
func collectConnection(flags storageMigrateFlags, interactive bool) (storage.S3Connection, error) {
	conn := storage.S3Connection{
		Endpoint:     flags.endpoint,
		Region:       flags.region,
		AccessKey:    flags.accessKey,
		SecretKey:    flags.secretKey,
		UsePathStyle: flags.pathStyle,
	}

	if interactive {
		fmt.Println()
		fmt.Println(storageSectionStyle.Render("S3 connection"))
		fmt.Println()

		preset, _ := presetByID(flags.provider)
		if flags.provider == "" {
			presetID := "aws"
			presetOptions := make([]huh.Option[string], 0, len(providerPresets))
			for _, p := range providerPresets {
				presetOptions = append(presetOptions, huh.NewOption(p.label, p.id))
			}
			if err := huh.NewSelect[string]().
				Title("Which storage provider are you using?").
				Options(presetOptions...).
				Value(&presetID).
				WithTheme(tui.ShopwareTheme()).
				Run(); err != nil {
				return conn, err
			}
			preset, _ = presetByID(presetID)
		}

		if preset.note != "" {
			fmt.Println(tui.SecondaryText.Render("  " + preset.note))
			fmt.Println()
		}

		// Apply preset defaults unless the user already enabled path-style via flag.
		if !conn.UsePathStyle {
			conn.UsePathStyle = preset.pathStyle
		}

		fields := []huh.Field{}
		if conn.Endpoint == "" && preset.id != "aws" {
			fields = append(fields, huh.NewInput().
				Title("Endpoint").
				Description(preset.endpointHint).
				Value(&conn.Endpoint))
		}
		if conn.Region == "" {
			fields = append(fields, huh.NewInput().
				Title("Region").
				Placeholder("eu-central-1").
				Value(&conn.Region))
		}
		if conn.AccessKey == "" {
			fields = append(fields, huh.NewInput().
				Title("Access key").
				Validate(emptyValidator).
				Value(&conn.AccessKey))
		}
		if conn.SecretKey == "" {
			fields = append(fields, huh.NewInput().
				Title("Secret key").
				EchoMode(huh.EchoModePassword).
				Validate(emptyValidator).
				Value(&conn.SecretKey))
		}

		if len(fields) > 0 {
			if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(tui.ShopwareTheme()).Run(); err != nil {
				return conn, err
			}
		}
	} else {
		if preset, ok := presetByID(flags.provider); ok && !flags.pathStyle {
			conn.UsePathStyle = preset.pathStyle
		}
	}

	if conn.AccessKey == "" || conn.SecretKey == "" {
		return conn, fmt.Errorf("missing S3 credentials; provide --access-key/--secret-key or the %s/%s env vars", storage.DefaultAccessKeyEnv, storage.DefaultSecretKeyEnv)
	}

	return conn, nil
}

// collectTargets determines the bucket / public URL for each datastore.
func collectTargets(conn storage.S3Connection, stores []storage.Datastore, flags storageMigrateFlags, interactive bool) ([]storage.StoreTarget, error) {
	targets := make([]storage.StoreTarget, 0, len(stores))

	lastBucket := flags.bucket
	for _, ds := range stores {
		target := storage.StoreTarget{
			Datastore: ds,
			Bucket:    flags.bucket,
			Root:      flags.root,
			PublicURL: flags.publicURL,
		}

		if interactive {
			bucket := lastBucket
			if err := huh.NewInput().
				Title(fmt.Sprintf("Bucket for the %q datastore", ds.Name)).
				Description(bucketHint(ds)).
				Validate(emptyValidator).
				Value(&bucket).
				WithTheme(tui.ShopwareTheme()).
				Run(); err != nil {
				return nil, err
			}
			target.Bucket = bucket
			lastBucket = bucket

			if ds.Public {
				url := target.PublicURL
				if url == "" {
					url = guessPublicURL(conn, bucket)
				}
				if err := huh.NewInput().
					Title(fmt.Sprintf("Public base URL for %q", ds.Name)).
					Description("Where these files are reachable from the browser (CDN or bucket URL).").
					Value(&url).
					WithTheme(tui.ShopwareTheme()).
					Run(); err != nil {
					return nil, err
				}
				target.PublicURL = url
			}
		} else {
			if target.Bucket == "" {
				return nil, fmt.Errorf("missing --bucket for datastore %q", ds.Name)
			}
			if ds.Public && target.PublicURL == "" {
				target.PublicURL = guessPublicURL(conn, target.Bucket)
			}
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func bucketHint(ds storage.Datastore) string {
	if ds.Public {
		return "Public datastore — the bucket should allow public read access."
	}
	return "Private datastore — keep this bucket private (no public access)."
}

func guessPublicURL(conn storage.S3Connection, bucket string) string {
	if conn.Endpoint != "" {
		base := strings.TrimRight(conn.Endpoint, "/")
		if conn.UsePathStyle {
			return base + "/" + bucket
		}
		// virtual-hosted style: bucket.endpoint-host
		if scheme, host, ok := splitScheme(base); ok {
			return scheme + bucket + "." + host
		}
	}
	region := conn.Region
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, region)
}

func splitScheme(u string) (scheme, host string, ok bool) {
	for _, s := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, s) {
			return s, strings.TrimPrefix(u, s), true
		}
	}
	return "", "", false
}

func validateTargets(ctx context.Context, client *storage.S3Client, targets []storage.StoreTarget) error {
	fmt.Println()
	fmt.Println(storageSectionStyle.Render("Validating target"))
	fmt.Println()

	checked := map[string]bool{}
	for _, t := range targets {
		if checked[t.Bucket] {
			continue
		}
		checked[t.Bucket] = true

		if err := client.ValidateBucket(ctx, t.Bucket); err != nil {
			fmt.Printf("  %s bucket %s\n", tui.RedText.Render("✗"), t.Bucket)
			return fmt.Errorf("bucket validation failed: %w", err)
		}
		fmt.Printf("  %s bucket %s reachable, read/write OK\n", tui.Checkmark, t.Bucket)
	}

	return nil
}

func runMigration(ctx context.Context, client *storage.S3Client, targets []storage.StoreTarget, scans map[string]storage.ScanResult, flags storageMigrateFlags, totalFiles int) (storage.MigrateSummary, error) {
	fmt.Println()
	fmt.Println(storageSectionStyle.Render("Migrating files"))
	fmt.Println()

	var done int64
	var bytesDone int64
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				d := atomic.LoadInt64(&done)
				b := atomic.LoadInt64(&bytesDone)
				fmt.Printf("\r  %d/%d files (%s)   ", d, totalFiles, humanize.Bytes(uint64(b)))
			}
		}
	}()

	summary, err := storage.Migrate(ctx, client, targets, scans, storage.MigrateOptions{
		Concurrency:  flags.concurrency,
		SkipExisting: flags.skipExisting,
		DryRun:       flags.dryRun,
		PublicACL:    flags.publicACL,
	}, func(ev storage.ProgressEvent) {
		atomic.AddInt64(&done, 1)
		if ev.Err == nil && !ev.Skipped {
			atomic.AddInt64(&bytesDone, ev.Size)
		}
	})

	close(stop)
	fmt.Printf("\r  %d/%d files (%s)   \n", atomic.LoadInt64(&done), totalFiles, humanize.Bytes(uint64(atomic.LoadInt64(&bytesDone))))

	return summary, err
}

func writeStorageConfig(ctx context.Context, projectRoot string, conn storage.S3Connection, targets []storage.StoreTarget, flags storageMigrateFlags, interactive bool) error {
	opts := storage.ConfigOptions{
		Connection: conn,
		Targets:    targets,
		PublicACL:  flags.publicACL,
	}
	yaml := storage.RenderFilesystemConfig(opts)

	fmt.Println()
	fmt.Println(storageSectionStyle.Render("Shopware configuration"))
	fmt.Println()
	fmt.Println(yaml)

	if flags.printConfig || flags.dryRun {
		fmt.Println(tui.SecondaryText.Render("Add the YAML above to your config/packages and set the credentials in .env.local."))
		return nil
	}

	if interactive && !flags.assumeYes {
		write := true
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Write this config to %s and store credentials in .env.local?", flags.configOut)).
			Value(&write).
			WithTheme(tui.ShopwareTheme()).
			Run(); err != nil {
			return err
		}
		if !write {
			fmt.Println(tui.SecondaryText.Render("Skipped writing config. Copy the YAML above manually."))
			return nil
		}
	}

	configPath := filepath.Join(projectRoot, filepath.FromSlash(flags.configOut))
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		return err
	}
	logging.FromContext(ctx).Infof("Wrote %s", flags.configOut)

	if err := upsertEnvLocal(filepath.Join(projectRoot, ".env.local"), opts.EnvVars()); err != nil {
		return err
	}
	logging.FromContext(ctx).Infof("Stored credentials in .env.local")

	return nil
}

// upsertEnvLocal sets the given keys in the .env.local file, replacing existing
// definitions and appending the rest.
func upsertEnvLocal(path string, vars map[string]string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	remaining := map[string]string{}
	for k, v := range vars {
		remaining[k] = v
	}

	for i, line := range lines {
		for k, v := range remaining {
			if strings.HasPrefix(strings.TrimSpace(line), k+"=") {
				lines[i] = fmt.Sprintf("%s=%s", k, v)
				delete(remaining, k)
			}
		}
	}

	keys := make([]string, 0, len(remaining))
	for k := range remaining {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, remaining[k]))
	}

	out := strings.Join(lines, "\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func printPostSteps(projectRoot string, stores []storage.Datastore) {
	fmt.Println()
	fmt.Println(storageSectionStyle.Render("Next steps"))
	fmt.Println()

	step := 1
	if !hasS3Adapter(projectRoot) {
		fmt.Printf("  %d. Install the S3 flysystem adapter:\n", step)
		fmt.Printf("       %s\n", storageLabelStyle.Render("composer require league/flysystem-aws-s3-v3"))
		step++
	}

	for _, ds := range stores {
		if ds.RebuildCommand != "" {
			fmt.Printf("  %d. Regenerate the %q datastore on S3:\n", step, ds.Name)
			fmt.Printf("       %s\n", storageLabelStyle.Render("bin/console "+ds.RebuildCommand))
			step++
		}
	}

	fmt.Printf("  %d. Clear the cache so the new config is picked up:\n", step)
	fmt.Printf("       %s\n", storageLabelStyle.Render("bin/console cache:clear"))
	step++

	fmt.Printf("  %d. Verify a media URL in the Storefront/Admin and, once happy, remove the\n", step)
	fmt.Println("     local files to reclaim disk space.")
	fmt.Println()
	fmt.Println(tui.SecondaryText.Render("Docs: https://developer.shopware.com/docs/guides/hosting/infrastructure/filesystem.html"))
}

func hasS3Adapter(projectRoot string) bool {
	data, err := os.ReadFile(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "league/flysystem-aws-s3-v3")
}

func init() {
	f := storageMigrateCmd.Flags()
	f.StringSlice("stores", nil, "Datastores to migrate (e.g. public,private). Prompts when omitted.")
	f.String("provider", "", "Provider preset: aws, minio, r2, digitalocean, hetzner, custom")
	f.String("endpoint", "", "S3 endpoint URL (leave empty for AWS)")
	f.String("region", "", "S3 region")
	f.String("bucket", "", "Target bucket (applies to all selected datastores)")
	f.String("access-key", "", "S3 access key (or set "+storage.DefaultAccessKeyEnv+")")
	f.String("secret-key", "", "S3 secret key (or set "+storage.DefaultSecretKeyEnv+")")
	f.Bool("path-style", false, "Use path-style addressing (required by most non-AWS providers)")
	f.String("root", "", "Optional key prefix inside the bucket")
	f.String("public-url", "", "Public base URL for public datastores")
	f.Bool("public-acl", false, "Set a public-read ACL on objects of public datastores")
	f.Int("concurrency", 8, "Number of parallel uploads")
	f.Bool("skip-existing", true, "Skip files already present in the bucket with the same size")
	f.Bool("dry-run", false, "Show what would be migrated without uploading")
	f.String("config-out", "config/packages/zz-shopware-cli-storage.yaml", "Where to write the generated config")
	f.Bool("print-config", false, "Only print the generated config instead of writing it")
	f.Bool("yes", false, "Skip confirmation prompts")

	projectStorageRootCmd.AddCommand(storageMigrateCmd)
}
