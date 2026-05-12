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

type AdditionalCache struct {
	Path        string
	SourcePaths []string
}
