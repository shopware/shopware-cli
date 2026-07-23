package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterInstallVersions(t *testing.T) {
	t.Parallel()

	releases := []repository.Version{
		{Version: "6.7.12.x-dev"},
		{Version: "6.7.12.1"},
		{Version: "v6.7.11.1"},
		{Version: "dev-trunk"},
		{Version: "invalid"},
		{Version: "6.3.0.0"},
	}

	filtered := FilterInstallVersions(releases)
	got := make([]string, 0, len(filtered))
	for _, filteredVersion := range filtered {
		got = append(got, filteredVersion.String())
	}

	assert.Equal(t, []string{"6.7.12.1", "6.7.11.1"}, got)
}

func TestResolveInstallVersion(t *testing.T) {
	t.Parallel()

	versions := []*version.Version{
		version.Must(version.NewVersion("6.6.1.0-rc1")),
		version.Must(version.NewVersion("6.6.0.0")),
		version.Must(version.NewVersion("6.5.8.0")),
	}

	tests := []struct {
		name     string
		selected string
		versions []*version.Version
		want     string
		wantErr  bool
	}{
		{name: "latest stable", selected: VersionLatest, versions: versions, want: "6.6.0.0"},
		{
			name:     "latest falls back to RC",
			selected: VersionLatest,
			versions: []*version.Version{
				version.Must(version.NewVersion("6.7.0.0-rc2")),
				version.Must(version.NewVersion("6.7.0.0-rc1")),
			},
			want: "6.7.0.0-rc2",
		},
		{name: "exact version", selected: "6.5.8.0", versions: versions, want: "6.5.8.0"},
		{name: "dev branch", selected: "dev-trunk", versions: versions, want: "dev-trunk"},
		{name: "missing latest", selected: VersionLatest, wantErr: true},
		{name: "unknown version", selected: "6.4.0.0", versions: versions, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolved, err := ResolveInstallVersion(test.selected, test.versions)
			if test.wantErr {
				assert.Error(t, err)
				assert.Empty(t, resolved)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, resolved)
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	validNames := []string{
		"my-shopware-project",
		"myshop",
		"my_shop",
		"shop123",
		"123shop",
		"a",
		"path/to/my-shop",
		".",
	}
	for _, name := range validNames {
		t.Run("valid: "+name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, ValidateProjectName(name))
		})
	}

	invalidNames := []string{
		"MyShop",
		"myShop",
		"SHOP",
		"müller",
		"über-shop",
		"Müller-Shop",
		"café",
		"straße",
		"my shop",
		"my.shop",
		"shop!",
		"-shop",
		"_shop",
		"ä",
		"",
		"path/to/müller",
		"path/to/MyShop",
	}
	for _, name := range invalidNames {
		t.Run("invalid: "+name, func(t *testing.T) {
			t.Parallel()
			err := ValidateProjectName(name)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid project name")
		})
	}
}

func TestValidateProjectFolder(t *testing.T) {
	t.Parallel()

	t.Run("rejects an invalid project name", func(t *testing.T) {
		t.Parallel()
		assert.ErrorContains(t, ValidateProjectFolder(filepath.Join(t.TempDir(), "MyShop")), "invalid project name")
	})

	t.Run("accepts a missing folder", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateProjectFolder(filepath.Join(t.TempDir(), "shop")))
	})

	t.Run("accepts an empty folder", func(t *testing.T) {
		t.Parallel()
		projectFolder := filepath.Join(t.TempDir(), "shop")
		require.NoError(t, os.Mkdir(projectFolder, 0o755))
		assert.NoError(t, ValidateProjectFolder(projectFolder))
	})

	t.Run("rejects a non-empty folder", func(t *testing.T) {
		t.Parallel()
		projectFolder := filepath.Join(t.TempDir(), "shop")
		require.NoError(t, os.Mkdir(projectFolder, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(projectFolder, "existing"), nil, 0o600))
		assert.ErrorContains(t, ValidateProjectFolder(projectFolder), "not empty")
	})

	t.Run("rejects a file", func(t *testing.T) {
		t.Parallel()
		projectFolder := filepath.Join(t.TempDir(), "shop")
		require.NoError(t, os.WriteFile(projectFolder, nil, 0o600))
		assert.ErrorContains(t, ValidateProjectFolder(projectFolder), "not a directory")
	})
}

func TestValidateDeploymentMethod(t *testing.T) {
	t.Parallel()

	for _, deploymentMethod := range []string{
		DeploymentNone,
		DeploymentDeployer,
		DeploymentPlatformSH,
		DeploymentShopwarePaaS,
	} {
		t.Run(deploymentMethod, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, ValidateDeploymentMethod(deploymentMethod))
		})
	}

	assert.Error(t, ValidateDeploymentMethod("invalid"))
}

func TestValidateCISystem(t *testing.T) {
	t.Parallel()

	for _, ciSystem := range []string{CINone, CIGitHub, CIGitLab} {
		t.Run(ciSystem, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, ValidateCISystem(ciSystem))
		})
	}

	assert.Error(t, ValidateCISystem("jenkins"))
}

func TestShopwareProjectScaffoldNormalize(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults", func(t *testing.T) {
		t.Parallel()
		scaffold := ShopwareProjectScaffold{}

		scaffold.Normalize()

		assert.Equal(t, DeploymentNone, scaffold.DeploymentMethod)
		assert.Equal(t, CINone, scaffold.CISystem)
		assert.False(t, scaffold.UseElasticsearch)
	})

	t.Run("enables Elasticsearch for Shopware PaaS", func(t *testing.T) {
		t.Parallel()
		scaffold := ShopwareProjectScaffold{DeploymentMethod: DeploymentShopwarePaaS}

		scaffold.Normalize()

		assert.True(t, scaffold.UseElasticsearch)
	})
}

func TestShopwareProjectScaffoldValidate(t *testing.T) {
	t.Parallel()

	validScaffold := func(t *testing.T) ShopwareProjectScaffold {
		t.Helper()
		return ShopwareProjectScaffold{
			ProjectFolder:    filepath.Join(t.TempDir(), "shop"),
			Version:          "6.6.10.0",
			DeploymentMethod: DeploymentNone,
			CISystem:         CINone,
		}
	}

	t.Run("accepts valid options", func(t *testing.T) {
		t.Parallel()
		scaffold := validScaffold(t)
		assert.NoError(t, scaffold.Validate())
	})

	t.Run("rejects an empty version", func(t *testing.T) {
		t.Parallel()
		scaffold := validScaffold(t)
		scaffold.Version = ""
		assert.ErrorContains(t, scaffold.Validate(), "version must not be empty")
	})

	t.Run("rejects an invalid deployment", func(t *testing.T) {
		t.Parallel()
		scaffold := validScaffold(t)
		scaffold.DeploymentMethod = "invalid"
		assert.ErrorContains(t, scaffold.Validate(), "invalid deployment method")
	})

	t.Run("rejects an invalid CI system", func(t *testing.T) {
		t.Parallel()
		scaffold := validScaffold(t)
		scaffold.CISystem = "jenkins"
		assert.ErrorContains(t, scaffold.Validate(), "invalid CI system")
	})
}
