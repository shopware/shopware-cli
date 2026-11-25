package extension

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/validation"
)

const (
	TypePlatformApp    = "app"
	TypePlatformPlugin = "plugin"
	TypeShopwareBundle = "shopware-bundle"

	ComposerTypePlugin = "shopware-platform-plugin"
	ComposerTypeApp    = "shopware-app"
	ComposerTypeBundle = "shopware-bundle"
)

func GetExtensionByFolder(ctx context.Context, path string) (Extension, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path + "/plugin.xml"); err == nil {
		return nil, errors.New("shopware 5 is not supported. Please use https://github.com/FriendsOfShopware/FroshPluginUploader instead")
	}

	if _, err := os.Stat(path + "/manifest.xml"); err == nil {
		return newApp(ctx, path)
	}

	if _, err := os.Stat(path + "/composer.json"); err != nil {
		return nil, fmt.Errorf("unknown extension type: no composer.json or manifest.xml found in %s", path)
	}

	var ext Extension

	ext, err := newPlatformPlugin(ctx, path)
	if err != nil {
		if errors.Is(err, ErrPlatformInvalidType) {
			ext, err = newShopwareBundle(ctx, path)
		} else {
			return nil, err
		}
	}

	return ext, err
}

// GetExtensionByZip extracts an extension into a temporary directory. The caller
// must remove filepath.Dir(ext.GetPath()) when the returned extension is no longer needed.
// The extension must be in an immediate child folder of the extraction directory.
func GetExtensionByZip(ctx context.Context, filePath string) (Extension, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cannot open extension archive: %s is not a regular file", filePath)
	}

	archive, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()

	info, err = archive.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cannot open extension archive: %s is not a regular file", filePath)
	}

	file, err := zip.NewReader(contextReaderAt{ctx: ctx, reader: archive}, info.Size())
	if err != nil {
		return nil, err
	}
	if len(file.File) == 0 {
		return nil, errors.New("cannot extract empty extension archive")
	}

	fileName := file.File[0].Name
	if strings.Contains(fileName, "..") {
		return nil, errors.New("invalid zip file")
	}

	extName := strings.Split(fileName, "/")[0]

	dir, err := os.MkdirTemp("", "extension")
	if err != nil {
		return nil, err
	}

	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()

	extPath := filepath.Join(dir, extName)
	if filepath.Dir(extPath) != filepath.Clean(dir) {
		return nil, errors.New("cannot extract extension archive without a top-level extension folder")
	}

	err = archiver.Unzip(ctx, file, dir)
	if err != nil {
		return nil, err
	}

	ext, err := GetExtensionByFolder(ctx, extPath)
	if err != nil {
		return nil, err
	}

	keep = true
	return ext, nil
}

// The reader is confined to the lifetime of GetExtensionByZip.
type contextReaderAt struct {
	ctx    context.Context //nolint:containedctx
	reader io.ReaderAt
}

func (r contextReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.ReadAt(p, offset)
}

type ExtensionTranslated struct {
	German  string `json:"german"`
	English string `json:"english"`
}

type ExtensionChangelog struct {
	German     string `json:"german"`
	English    string `json:"english"`
	Changelogs map[string]string
}

type ExtensionMetadata struct {
	Name        string
	Label       ExtensionTranslated
	Description ExtensionTranslated
}

type Extension interface {
	GetName() (string, error)
	GetComposerName() (string, error)
	// Deprecated: use the list variation instead
	GetResourcesDir() string
	GetResourcesDirs() []string

	GetIconPath() string

	// GetRootDir Returns the root folder where the code is located plugin -> src, app ->
	GetRootDir() string
	GetSourceDirs() []string
	GetVersion() (*version.Version, error)
	GetLicense() (string, error)
	GetShopwareVersionConstraint() (*version.Constraints, error)
	GetType() string
	GetPath() string
	GetChangelog() (*ExtensionChangelog, error)
	GetMetaData() *ExtensionMetadata
	UpdateMetaData(*ExtensionMetadata) error
	GetExtensionConfig() *Config
	Validate(context.Context, validation.Check)
}

// getShopwareVersionConstraintFromComposer is a shared helper for composer-based extensions
// (PlatformPlugin and ShopwareBundle) to extract the Shopware compatibility constraint from
// the composer requirements. It intentionally ignores the build-time override
// (config.Build.ShopwareVersionConstraint) so that the reported compatibility always reflects
// what the extension actually supports (e.g. for account store uploads). Use
// GetShopwareBuildVersionConstraint / GetShopwareVersionConstraintForBuild when the build-time
// override should be honored.
func getShopwareVersionConstraintFromComposer(composerRequire map[string]string) (*version.Constraints, error) {
	shopwareConstraintString, ok := composerRequire["shopware/core"]
	if !ok {
		return nil, errors.New("require.shopware/core is required")
	}

	shopwareConstraint, err := version.NewConstraint(shopwareConstraintString)
	if err != nil {
		return nil, err
	}

	return &shopwareConstraint, nil
}

// GetShopwareBuildVersionConstraint returns the Shopware version constraint configured as a
// build-time override via config.Build.ShopwareVersionConstraint. It handles a nil config
// gracefully and returns (nil, nil) when no override is configured. This is the single source
// of truth for the build-time constraint and is never mixed into an extension's reported
// compatibility constraint.
func GetShopwareBuildVersionConstraint(config *Config) (*version.Constraints, error) {
	if config != nil && config.Build.ShopwareVersionConstraint != "" {
		constraint, err := version.NewConstraint(config.Build.ShopwareVersionConstraint)
		if err != nil {
			return nil, err
		}

		return &constraint, nil
	}

	return nil, nil //nolint:nilnil // nil constraint signals "no build override configured", not an error
}

// GetShopwareVersionConstraintForBuild resolves the constraint that should be used when building
// assets for the given extension. It honors the build-time override
// (config.Build.ShopwareVersionConstraint) when set and otherwise falls back to the extension's
// reported compatibility constraint.
func GetShopwareVersionConstraintForBuild(ext Extension) (*version.Constraints, error) {
	buildConstraint, err := GetShopwareBuildVersionConstraint(ext.GetExtensionConfig())
	if err != nil {
		return nil, err
	}

	if buildConstraint != nil {
		return buildConstraint, nil
	}

	return ext.GetShopwareVersionConstraint()
}
