package esbuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKebabCase(t *testing.T) {
	assert.Equal(t, "foo-bar", ToKebabCase("FooBar"))
	assert.Equal(t, "f-o-o-bar-baz", ToKebabCase("FOOBarBaz"))
	assert.Equal(t, "frosh-tools", ToKebabCase("FroshTools"))
	assert.Equal(t, "my-module-name-s-w6", ToKebabCase("MyModuleNameSW6"))
	assert.Equal(t, "a-i-search", ToKebabCase("AISearch"))
	assert.Equal(t, "mediameets-fb-pixel", ToKebabCase("mediameetsFbPixel"))
	assert.Equal(t, "wwbla-bar-foo", ToKebabCase("wwblaBarFoo"))
	assert.Equal(t, "with-underscore", ToKebabCase("with_underscore"))
}

func TestBundleFolderName(t *testing.T) {
	assert.Equal(t, "myplugin", toBundleFolderName("MyPluginBundle"))
	assert.Equal(t, "anotherplugin", toBundleFolderName("AnotherPluginBundle"))
	assert.Equal(t, "simpleplugin", toBundleFolderName("SimplePlugin"))
	assert.Equal(t, "plugin", toBundleFolderName("PluginBundle"))
	assert.Equal(t, "plugin", toBundleFolderName("Plugin"))
}
