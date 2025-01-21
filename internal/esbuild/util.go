package esbuild

import (
	"regexp"
	"strings"
)

var matchLetter = regexp.MustCompile(`[A-Z]`)

// @see https://github.com/symfony/symfony/blob/6.3/src/Symfony/Component/Serializer/NameConverter/CamelCaseToSnakeCaseNameConverter.php#L31
func ToKebabCase(str string) string {
	converted := matchLetter.ReplaceAllStringFunc(str, func(match string) string {
		return "-" + strings.ToLower(match)
	})

	// See https://github.com/shopware/shopware/blob/240386d/src/Core/Framework/Plugin/BundleConfigGenerator.php#L73
	converted = strings.ReplaceAll(converted, "_", "-")

	return strings.TrimPrefix(converted, "-")
}

// @see https://github.com/symfony/symfony/blob/7.2/src/Symfony/Bundle/FrameworkBundle/Command/AssetsInstallCommand.php#L128
func toBundleFolderName(name string) string {
	assetDir := strings.ToLower(name)
	assetDir = strings.TrimSuffix(assetDir, "bundle")
	return assetDir
}
