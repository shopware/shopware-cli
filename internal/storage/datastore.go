package storage

// Datastore describes a Shopware filesystem datastore that can be migrated from
// the local disk to an S3 compatible storage.
//
// Shopware configures each datastore under shopware.filesystem.<Name>. The
// local adapter of a datastore is rooted at LocalBase (relative to the project
// root) and the actual files live in the given Prefixes below that root. When a
// datastore is switched to S3 the prefixes stay the same, so an object key is
// simply the file path relative to LocalBase.
type Datastore struct {
	// Name is the Shopware config key under shopware.filesystem.<Name>.
	Name string
	// Title is a short human readable label.
	Title string
	// Description explains what the datastore holds.
	Description string
	// LocalBase is the project-relative directory the local adapter is rooted at.
	LocalBase string
	// Prefixes are the sub-directories (relative to LocalBase) that hold this
	// datastore's files. An empty slice means the whole LocalBase directory.
	Prefixes []string
	// Public indicates whether the datastore serves publicly reachable files.
	Public bool
	// Recommended marks datastores that typically should be migrated.
	Recommended bool
	// RebuildCommand, when set, names a bin/console command that regenerates the
	// datastore's contents. Such datastores usually do not need to be copied;
	// running the command after switching to S3 is the cleaner option.
	RebuildCommand string
}

// KnownDatastores lists the Shopware filesystem datastores in the order they
// should be presented to the user.
var KnownDatastores = []Datastore{
	{
		Name:        "public",
		Title:       "Public — media & thumbnails",
		Description: "Customer-uploaded media files and generated thumbnails.",
		LocalBase:   "public",
		Prefixes:    []string{"media", "thumbnail"},
		Public:      true,
		Recommended: true,
	},
	{
		Name:        "private",
		Title:       "Private — documents, downloads, im/export",
		Description: "Invoices, delivery notes, digital products and import/export files.",
		LocalBase:   "files",
		Prefixes:    nil,
		Public:      false,
		Recommended: true,
	},
	{
		Name:        "sitemap",
		Title:       "Sitemap",
		Description: "Generated sitemap.xml files. Small and regenerated regularly.",
		LocalBase:   "public",
		Prefixes:    []string{"sitemap"},
		Public:      true,
		Recommended: false,
	},
	{
		Name:           "theme",
		Title:          "Theme — compiled CSS/JS",
		Description:    "Compiled theme assets. Better regenerated than copied.",
		LocalBase:      "public",
		Prefixes:       []string{"theme"},
		Public:         true,
		Recommended:    false,
		RebuildCommand: "theme:compile",
	},
	{
		Name:           "asset",
		Title:          "Asset — plugin & app bundles",
		Description:    "Bundled plugin/app assets. Better regenerated than copied.",
		LocalBase:      "public",
		Prefixes:       []string{"bundles"},
		Public:         true,
		Recommended:    false,
		RebuildCommand: "asset:install",
	},
}

// DatastoreByName returns the known datastore with the given name.
func DatastoreByName(name string) (Datastore, bool) {
	for _, ds := range KnownDatastores {
		if ds.Name == name {
			return ds, true
		}
	}
	return Datastore{}, false
}
