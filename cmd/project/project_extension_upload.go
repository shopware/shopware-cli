package project

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionUploadCmd = &cobra.Command{
	Use:   "upload [path]",
	Short: "Upload local extension to external shop",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *shop.Config
		var err error

		adminCtx := adminSdk.NewApiContext(cmd.Context())

		doLifecycleEvents, _ := cmd.PersistentFlags().GetBool("activate")
		increaseVersionBeforeUpload, _ := cmd.PersistentFlags().GetBool("increase-version")

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		var ext extension.Extension

		isFolder := true

		if stat.IsDir() {
			ext, err = extension.GetExtensionByFolder(cmd.Context(), path)
		} else {
			ext, err = extension.GetExtensionByZip(cmd.Context(), path)
			isFolder = false
		}

		if err != nil {
			return err
		}

		extCfg := ext.GetExtensionConfig()
		if err != nil {
			logging.FromContext(cmd.Context()).Fatalln(fmt.Errorf("update: %v", err))
		}

		if increaseVersionBeforeUpload {
			if err := increaseExtensionVersion(cmd.Context(), ext); err != nil {
				return err
			}

			ext, err = extension.GetExtensionByFolder(cmd.Context(), ext.GetPath())
			if err != nil {
				return err
			}
		}

		if isFolder {
			// Create temp dir
			tempDir, err := os.MkdirTemp("", "extension")
			if err != nil {
				return fmt.Errorf("create temp directory: %w", err)
			}

			extName, err := ext.GetName()
			if err != nil {
				return fmt.Errorf("get extension name: %w", err)
			}

			extDir := fmt.Sprintf("%s/%s/", tempDir, extName)

			err = os.Mkdir(extDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("create temp directory: %w", err)
			}

			tempDir += "/"

			defer func(path string) {
				_ = os.RemoveAll(path)
			}(tempDir)

			err = cp.Copy(path, extDir)
			if err != nil {
				return fmt.Errorf("copy files: %w", err)
			}

			ext, err = extension.GetExtensionByFolder(cmd.Context(), extDir)
			if err != nil {
				return err
			}

			// Cleanup not wanted files
			if err := extension.CleanupExtensionFolder(ext.GetPath(), extCfg.Build.Zip.Pack.Excludes.Paths); err != nil {
				return fmt.Errorf("cleanup package: %w", err)
			}
		}

		if cfg, err = shop.ReadConfig(cmd.Context(), projectConfigPath, true); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		name, err := ext.GetName()
		if err != nil {
			return err
		}

		version, err := ext.GetVersion()
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		if err := archiver.AddZipFiles(w, ext.GetPath()+"/", name+"/"); err != nil {
			return fmt.Errorf("uploading extension: %w", err)
		}

		if err := w.Close(); err != nil {
			return err
		}

		shopInfo, _, err := client.Info.Info(adminCtx)
		if err != nil {
			return fmt.Errorf("cannot get shop info: %w", err)
		}

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminCtx)
		if err != nil {
			return err
		}

		if !shopInfo.IsCloudShop() || extensions.GetByName(name) == nil {
			if uploadResponse, err := client.ExtensionManager.UploadExtension(adminCtx, &buf); err != nil {
				return fmt.Errorf("cannot upload extension: %w", err)
			} else if uploadResponse.StatusCode != 204 {
				str, err := io.ReadAll(uploadResponse.Body)
				if err != nil {
					return fmt.Errorf("cannot upload extension update: %w", err)
				}

				return fmt.Errorf("cannot upload extension update: %s", string(str))
			}

			extensions, _, err = client.ExtensionManager.ListAvailableExtensions(adminCtx)
			if err != nil {
				return err
			}
		} else {
			if uploadResponse, err := client.ExtensionManager.UploadExtensionUpdateToCloud(adminCtx, name, &buf); err != nil {
				return fmt.Errorf("cannot upload extension update: %w", err)
			} else if uploadResponse.StatusCode != 204 {
				str, err := io.ReadAll(uploadResponse.Body)
				if err != nil {
					return fmt.Errorf("cannot upload extension update: %w", err)
				}

				return fmt.Errorf("cannot upload extension update: %s", string(str))
			}
		}

		logging.FromContext(cmd.Context()).Infof("Uploaded extension %s with version %s", name, version.String())

		if _, err := client.ExtensionManager.Refresh(adminCtx); err != nil {
			return fmt.Errorf("cannot refresh extension list: %w", err)
		}

		logging.FromContext(cmd.Context()).Infof("Refreshed extension list")

		if doLifecycleEvents {
			remoteExtension := extensions.GetByName(name)

			if remoteExtension.InstalledAt == nil {
				if _, err := client.ExtensionManager.InstallExtension(adminCtx, remoteExtension.Type, remoteExtension.Name); err != nil {
					return fmt.Errorf("cannot install extension: %w", err)
				}

				logging.FromContext(cmd.Context()).Infof("Installed %s", name)
			}

			if !remoteExtension.Active {
				if _, err := client.ExtensionManager.ActivateExtension(adminCtx, remoteExtension.Type, remoteExtension.Name); err != nil {
					return fmt.Errorf("cannot activate extension: %w", err)
				}

				logging.FromContext(cmd.Context()).Infof("Activated %s", name)
			}

			if remoteExtension.IsUpdateAble() {
				if _, err := client.ExtensionManager.UpdateExtension(adminCtx, remoteExtension.Type, remoteExtension.Name); err != nil {
					return fmt.Errorf("cannot update extension: %w", err)
				}

				logging.FromContext(cmd.Context()).Infof("Updated %s from %s to %s", name, remoteExtension.Version, remoteExtension.LatestVersion)
			}
		}

		if ext.GetType() == "plugin" {
			if _, err := client.CacheManager.Clear(adminCtx); err != nil {
				return err
			}

			logging.FromContext(cmd.Context()).Infof("Cleared cache")
		}

		return nil
	},
}

func increaseExtensionVersion(ctx context.Context, ext extension.Extension) error {
	if ext.GetType() == "app" {
		manifestPath := fmt.Sprintf("%s/manifest.xml", ext.GetPath())
		file, err := os.Open(manifestPath)
		if err != nil {
			return fmt.Errorf("cannot read manifest file: %w", err)
		}

		defer func() {
			if err := file.Close(); err != nil {
				logging.FromContext(ctx).Errorf("increaseExtensionVersion: %v", err)
			}
		}()

		var buf bytes.Buffer
		decoder := xml.NewDecoder(file)
		encoder := xml.NewEncoder(&buf)

		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				logging.FromContext(ctx).Errorf("error getting token: %v\n", err)
				break
			}

			if v, ok := token.(xml.StartElement); ok {
				if v.Name.Local == "version" {
					var versionStr string
					if err = decoder.DecodeElement(&versionStr, &v); err != nil {
						return err
					}

					ver, err := version.NewVersion(versionStr)
					if err != nil {
						return err
					}

					ver.IncreasePatch()

					if err = encoder.EncodeElement(ver.String(), v); err != nil {
						return err
					}
					continue
				}
			}

			if err := encoder.EncodeToken(token); err != nil {
				return err
			}
		}

		// must call flush, otherwise some elements will be missing
		if err := encoder.Flush(); err != nil {
			return err
		}

		newManifest := buf.String()
		newManifest = strings.ReplaceAll(newManifest, "xmlns:_xmlns=\"xmlns\" _xmlns:xsi=", "xmlns:xsi=")
		newManifest = strings.ReplaceAll(newManifest, "xmlns:_XMLSchema-instance=\"http://www.w3.org/2001/XMLSchema-instance\" _XMLSchema-instance:noNamespaceSchemaLocation=", "xsi:noNamespaceSchemaLocation=")

		if err := os.WriteFile(manifestPath, []byte(newManifest), os.ModePerm); err != nil {
			return err
		}

		return nil
	}

	composerJsonPath := fmt.Sprintf("%s/composer.json", ext.GetPath())

	composerJsonContent, err := os.ReadFile(composerJsonPath)
	if err != nil {
		return err
	}

	var composerJson map[string]interface{}

	if err := json.Unmarshal(composerJsonContent, &composerJson); err != nil {
		return err
	}

	versionStr, ok := composerJson["version"].(string)

	if !ok {
		return nil
	}

	ver, err := version.NewVersion(versionStr)
	if err != nil {
		return err
	}

	ver.IncreasePatch()

	composerJson["version"] = ver.String()

	composerJsonContent, err = json.Marshal(composerJson)
	if err != nil {
		return err
	}

	if err := os.WriteFile(composerJsonPath, composerJsonContent, os.ModePerm); err != nil {
		return err
	}

	return nil
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionUploadCmd)
	projectExtensionUploadCmd.PersistentFlags().Bool("activate", false, "Installs, Activates, Updates the extension")
	projectExtensionUploadCmd.PersistentFlags().Bool("increase-version", false, "Increases extension version before uploading")
}
