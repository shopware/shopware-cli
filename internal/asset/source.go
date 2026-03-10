package asset

type Source struct {
	Name                        string
	Path                        string
	AdminEsbuildCompatible      bool
	StorefrontEsbuildCompatible bool
	DisableSass                 bool
	NpmStrict                   bool
	AdditionalCaches            []AdditionalCache
}

// AdditionalCache defines a custom path to be included in asset caching.
type AdditionalCache struct {
	// The output path to cache, relative to extension root
	Path string
	// Source paths to hash for the cache key, relative to extension root
	SourcePaths []string
}
