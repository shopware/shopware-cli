package shop

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/logging"
)

// DumpDatabaseOptions configures DumpDatabase. CLI concerns such as resolving
// the database credentials and prompting for passwords intentionally live
// outside this type.
type DumpDatabaseOptions struct {
	// Output is the destination file path, or "-" for stdout
	Output string
	// Compression compresses the dump output, parsed via mysqldump.ParseCompression
	Compression mysqldump.Compression
	// Clean excludes volatile tables (cart, messenger_messages, ...) from the data dump
	Clean bool
	// Anonymize rewrites customer related columns with faker data
	Anonymize bool
	// SkipLockTables disables locking the tables during the dump
	SkipLockTables bool
	// Quick enables the mysqldump quick mode
	Quick bool
	// Parallel controls how many tables are dumped concurrently (0 = disabled)
	Parallel int
	// InsertIntoLimit caps the rows per INSERT statement (0 = auto, takes priority over Quick when set)
	InsertIntoLimit int
	// LimitOverrides contains "table=rows" entries limiting the dumped rows per table
	LimitOverrides []string
}

// DumpDatabase dumps the database described by sqlCfg to opts.Output. cfg may
// be nil; Clean, Anonymize and LimitOverrides are applied to cfg in place.
func DumpDatabase(ctx context.Context, sqlCfg *mysql.Config, cfg *ConfigDump, opts DumpDatabaseOptions) error {
	db, err := sql.Open("mysql", sqlCfg.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	return dumpDatabase(ctx, db, cfg, opts)
}

func dumpDatabase(ctx context.Context, db *sql.DB, cfg *ConfigDump, opts DumpDatabaseOptions) error {
	if cfg == nil {
		cfg = &ConfigDump{}
	}

	if opts.Clean {
		cfg.EnableClean()
	}

	if opts.Anonymize {
		cfg.EnableAnonymization()
	}

	if err := applyLimitOverrides(cfg, opts.LimitOverrides); err != nil {
		return err
	}

	cfg.NormalizeFakerExpressions()

	dumper := mysqldump.NewMySQLDumper(db)
	dumper.LockTables = !opts.SkipLockTables
	dumper.Quick = opts.Quick
	dumper.Parallel = opts.Parallel
	dumper.InsertIntoLimit = opts.InsertIntoLimit
	dumper.SelectMap = cfg.Rewrite
	dumper.WhereMap = cfg.Where
	dumper.NoData = cfg.NoData
	dumper.Ignore = cfg.Ignore

	dumper.LimitMap = cfg.Limit

	w, output, err := mysqldump.OpenOutput(opts.Output, opts.Compression)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()

	if err := dumper.Dump(ctx, w); err != nil {
		if strings.Contains(err.Error(), "the RELOAD or FLUSH_TABLES privilege") {
			return fmt.Errorf("%s, you maybe want to disable locking with --skip-lock-tables", err.Error())
		}

		return err
	}

	// Closing flushes the compressor into the file; report flush errors
	// instead of silently producing a truncated dump.
	if err := w.Close(); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Successfully created the dump %s", output)

	return nil
}

// applyLimitOverrides merges "table=rows" entries into the dump config,
// keeping a configured OrderBy of the table.
func applyLimitOverrides(cfg *ConfigDump, overrides []string) error {
	for _, override := range overrides {
		table, rows, ok := strings.Cut(override, "=")
		if !ok {
			return fmt.Errorf("invalid --limit %q, expected format table=rows (e.g. order=100)", override)
		}

		rowCount, err := strconv.Atoi(rows)
		if err != nil || rowCount < 1 {
			return fmt.Errorf("invalid --limit %q, rows must be a positive number", override)
		}

		if cfg.Limit == nil {
			cfg.Limit = make(map[string]mysqldump.TableLimit)
		}

		limit := cfg.Limit[table]
		limit.Rows = rowCount
		cfg.Limit[table] = limit
	}

	return nil
}
