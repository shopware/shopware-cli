package shop

// Rows describing files of the Shopware filesystems. When an export leaves the
// files of a filesystem out, their rows are excluded too. Foreign key delete
// rules decide what happens to rows referencing them, e.g. product_media rows
// are dropped and category.media_id is set to NULL.
const (
	mediaTable            = "media"
	importExportFileTable = "import_export_file"
	publicMediaCondition  = "`private` = 0"
	privateMediaCondition = "`private` = 1"
	allRowsCondition      = "TRUE"
)

// fileRowExclusions returns the row exclusions for files of the public and
// private filesystems that are not part of an export.
func fileRowExclusions(skipPublic, skipPrivate bool) map[string]string {
	exclusions := make(map[string]string)

	switch {
	case skipPublic && skipPrivate:
		exclusions[mediaTable] = allRowsCondition
	case skipPublic:
		exclusions[mediaTable] = publicMediaCondition
	case skipPrivate:
		exclusions[mediaTable] = privateMediaCondition
	}

	if skipPrivate {
		exclusions[importExportFileTable] = allRowsCondition
	}

	return exclusions
}
