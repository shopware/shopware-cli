package project

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/sqlshell"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var projectDatabaseRestoreCmd = &cobra.Command{
	Use:     "restore [file]",
	Aliases: []string{"import"},
	Short:   "Restores a SQL dump into the Shopware database",
	Long: "Imports a plain, gzip- or zstd-compressed SQL file into the project database " +
		"using the connection details of the current environment (local, docker, ...). " +
		"The compression is detected from the file content, pass - to read from stdin.",
	Example: `  shopware-cli project restore dump.sql
  shopware-cli project restore dump.sql.gz
  shopware-cli project restore dump.sql.zst
  curl -s https://example.com/dump.sql.gz | shopware-cli project restore -`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		input := cmd.InOrStdin()
		var totalSize int64

		if args[0] != "-" {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			// The file is only read, a close error carries no information.
			defer func() { _ = file.Close() }()

			if info, err := file.Stat(); err == nil {
				totalSize = info.Size()
			}

			input = file
		}

		counting := &system.CountingReader{Reader: input}

		reader, err := system.DecompressReader(counting)
		if err != nil {
			return fmt.Errorf("could not open dump: %w", err)
		}

		// Closing releases decompressor resources; read errors already
		// surface through ExecuteStream.
		if closer, ok := reader.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}

		conn, dbConn, cleanup, err := connectProjectDatabase(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		force, _ := cmd.Flags().GetBool("force")

		// No prompt when reading the dump from stdin: it is not a terminal
		// then, and the prompt would consume dump bytes.
		if !force && system.IsInteractionEnabled(cmd.Context()) && isTerminalStream(cmd.InOrStdin()) {
			confirmed := false

			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Restore into database %q at %s?", dbConn.Database, dbConn.Addr())).
				Description("Existing data will be overwritten by the dump.").
				Value(&confirmed).
				Run(); err != nil {
				return err
			}

			if !confirmed {
				return errors.New("restore cancelled")
			}
		}

		logger := logging.FromContext(cmd.Context())
		logger.Infof("Restoring into database %q at %s", dbConn.Database, dbConn.Addr())

		start := time.Now()
		lastProgress := 0

		statements, err := sqlshell.ExecuteStream(cmd.Context(), conn, reader, func(int) {
			if totalSize <= 0 {
				return
			}

			progress := int(counting.BytesRead() * 100 / totalSize)
			if progress >= lastProgress+10 {
				lastProgress = progress
				logger.Infof("Restore progress: %d%%", progress)
			}
		})
		if err != nil {
			return fmt.Errorf("restore failed (%d statements executed): %w", statements, err)
		}

		logger.Infof("Restored %d statements in %s", statements, time.Since(start).Round(time.Millisecond))

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectDatabaseRestoreCmd)
	projectDatabaseRestoreCmd.Flags().BoolP("force", "f", false, "skip the confirmation prompt")
}
