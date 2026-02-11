package account

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

var accountCompanyProducerExtensionInfoPushCmd = &cobra.Command{
	Use:   "push [zip or path]",
	Short: "Update store information of extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absolutePath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}

		stat, err := os.Stat(absolutePath)
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}

		var zipExt extension.Extension

		if stat.IsDir() {
			zipExt, err = extension.GetExtensionByFolder(cmd.Context(), absolutePath)
		} else {
			zipExt, err = extension.GetExtensionByZip(cmd.Context(), absolutePath)
		}

		if err != nil {
			return fmt.Errorf("cannot open extension: %w", err)
		}

		zipName, err := zipExt.GetName()
		if err != nil {
			return fmt.Errorf("cannot get name: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		storeExt, err := p.GetExtensionByName(cmd.Context(), zipName)
		if err != nil {
			return fmt.Errorf("cannot get store extension: %w", err)
		}

		metadata := zipExt.GetMetaData()

		for _, info := range storeExt.Infos {
			language := info.Locale.Name[0:2]

			if language == "de" {
				info.Name = metadata.Label.German
				info.ShortDescription = metadata.Description.German
			} else {
				info.Name = metadata.Label.English
				info.ShortDescription = metadata.Description.English
			}
		}

		info, err := p.GetExtensionGeneralInfo(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get general info: %w", err)
		}

		extCfg := zipExt.GetExtensionConfig()

		if extCfg != nil {
			if extCfg.Store.Icon != nil {
				err := p.UpdateExtensionIcon(cmd.Context(), storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), *extCfg.Store.Icon))
				if err != nil {
					return fmt.Errorf("cannot update extension icon due error: %w", err)
				}
			}

			if extCfg.Store.Images != nil || extCfg.Store.ImageDirectory != nil {
				images, err := p.GetExtensionImages(cmd.Context(), storeExt.Id)
				if err != nil {
					return fmt.Errorf("cannot get images from remote server: %w", err)
				}

				for _, image := range images {
					err := p.DeleteExtensionImages(cmd.Context(), storeExt.Id, image.Id)
					if err != nil {
						return fmt.Errorf("cannot extension image: %w", err)
					}
				}

				if extCfg.Store.ImageDirectory != nil {
					if err := uploadImagesByDirectory(cmd.Context(), storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 0, p); err != nil {
						return err
					}

					if err := uploadImagesByDirectory(cmd.Context(), storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 1, p); err != nil {
						return err
					}
				} else {
					// manually specified images
					for _, configImage := range *extCfg.Store.Images {
						apiImage, err := p.AddExtensionImage(cmd.Context(), storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), configImage.File))
						if err != nil {
							return fmt.Errorf("cannot upload image %s to extension: %w", configImage.File, err)
						}

						apiImage.Priority = configImage.Priority
						apiImage.Details[0].Activated = configImage.Activate.German
						apiImage.Details[0].Preview = configImage.Preview.German

						apiImage.Details[1].Activated = configImage.Activate.English
						apiImage.Details[1].Preview = configImage.Preview.English

						err = p.UpdateExtensionImage(cmd.Context(), storeExt.Id, apiImage)
						if err != nil {
							return fmt.Errorf("cannot update image information of extension: %w", err)
						}
					}
				}
			}

			if err := updateStoreInfo(storeExt, zipExt, extCfg, info); err != nil {
				return fmt.Errorf("cannot update store information: %w", err)
			}
		}

		err = p.UpdateExtension(cmd.Context(), storeExt)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Store information has been updated")

		return nil
	},
}

func updateStoreInfo(ext *accountApi.Extension, zipExt extension.Extension, cfg *extension.Config, info *accountApi.ExtensionGeneralInformation) error { //nolint:gocyclo
	if cfg.Store.DefaultLocale != nil {
		for _, locale := range info.Locales {
			if locale.Name == *cfg.Store.DefaultLocale {
				ext.StandardLocale = locale
			}
		}
	}

	if cfg.Store.Localizations != nil {
		newLocales := make([]accountApi.Locale, 0)

		for _, configLocale := range *cfg.Store.Localizations {
			newLocales = append(newLocales, accountApi.Locale{Name: configLocale})
		}

		ext.Localizations = newLocales
	}

	if cfg.Store.Availabilities != nil {
		newAvailabilities := make([]accountApi.StoreAvailablity, 0)

		for _, availability := range info.StoreAvailabilities {
			for _, configLocale := range *cfg.Store.Availabilities {
				if availability.Name == configLocale {
					newAvailabilities = append(newAvailabilities, availability)
				}
			}
		}

		ext.StoreAvailabilities = newAvailabilities
	}

	if cfg.Store.Categories != nil {
		for _, category := range info.FutureCategories {
			for _, configCategory := range *cfg.Store.Categories {
				if category.Name == configCategory {
					selectCategory := category
					ext.Category = &selectCategory
					break
				}
			}
		}
	}

	if cfg.Store.Type != nil {
		for i, storeProductType := range info.ProductTypes {
			if storeProductType.Name == *cfg.Store.Type {
				ext.ProductType = &info.ProductTypes[i]
			}
		}
	}

	if cfg.Store.AutomaticBugfixVersionCompatibility != nil {
		ext.AutomaticBugfixVersionCompatibility = *cfg.Store.AutomaticBugfixVersionCompatibility
	}

	for _, info := range ext.Infos {
		language := info.Locale.Name[0:2]

		storeTags := getTranslation(language, cfg.Store.Tags)
		if storeTags != nil {
			var newTags []accountApi.StoreTag
			for _, tag := range *storeTags {
				newTags = append(newTags, accountApi.StoreTag{Name: tag})
			}

			info.Tags = newTags
		}

		storeVideos := getTranslation(language, cfg.Store.Videos)
		if storeVideos != nil {
			var newVideos []accountApi.StoreVideo
			for _, video := range *storeVideos {
				newVideos = append(newVideos, accountApi.StoreVideo{URL: video})
			}

			info.Videos = newVideos
		}

		storeHighlights := getTranslation(language, cfg.Store.Highlights)
		if storeHighlights != nil {
			info.Highlights = strings.Join(*storeHighlights, "\n")
		}

		storeFeatures := getTranslation(language, cfg.Store.Features)
		if storeFeatures != nil {
			info.Features = strings.Join(*storeFeatures, "\n")
		}

		storeFaqs := getTranslation(language, cfg.Store.Faq)
		if storeFaqs != nil {
			var newFaq []accountApi.StoreFaq
			for _, faq := range *storeFaqs {
				newFaq = append(newFaq, accountApi.StoreFaq{Question: faq.Question, Answer: faq.Answer, Position: faq.Position})
			}

			info.Faqs = newFaq
		}

		var err error

		storeDescription := getTranslation(language, cfg.Store.Description)
		if storeDescription != nil {
			info.Description, err = parseInlineablePath(*storeDescription, zipExt.GetPath())
			if err != nil {
				return err
			}
		}

		storeManual := getTranslation(language, cfg.Store.InstallationManual)
		if storeManual != nil {
			info.InstallationManual, err = parseInlineablePath(*storeManual, zipExt.GetPath())
			if err != nil {
				return err
			}
		}

		storeMetaTitle := getTranslation(language, cfg.Store.MetaTitle)
		if storeMetaTitle != nil {
			info.MetaTitle = *storeMetaTitle
		}

		storeMetaDescription := getTranslation(language, cfg.Store.MetaDescription)
		if storeMetaDescription != nil {
			info.MetaDescription = *storeMetaDescription
		}
	}

	return nil
}

func getTranslation[T extension.Translatable](language string, config extension.ConfigTranslated[T]) *T {
	switch language {
	case "de":
		return config.German
	case "en":
		return config.English
	}

	return nil
}

func init() {
	accountCompanyProducerExtensionInfoCmd.AddCommand(accountCompanyProducerExtensionInfoPushCmd)
}

func parseInlineablePath(path, extensionDir string) (string, error) {
	if !strings.HasPrefix(path, "file:") {
		return path, nil
	}

	filePath := fmt.Sprintf("%s/%s", extensionDir, strings.TrimPrefix(path, "file:"))

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file at path %s with error: %v", filePath, err)
	}

	if filepath.Ext(filePath) != ".md" {
		return string(content), nil
	}

	md := extension.GetConfiguredGoldMark()

	var buf bytes.Buffer
	err = md.Convert(content, &buf)
	if err != nil {
		return "", fmt.Errorf("cannot convert file at path %s from markdown to html with error: %v", filePath, err)
	}

	return buf.String(), nil
}

func uploadImagesByDirectory(ctx context.Context, extensionId int, directory string, index int, p *accountApi.ProducerEndpoint) error {
	// index 0 is for german, 1 for english defined by account api
	if index == 0 {
		directory = path.Join(directory, "de")
	} else {
		directory = path.Join(directory, "en")
	}

	images, err := os.ReadDir(directory)
	// When folder does not exists, skip
	if err != nil {
		return nil //nolint:nilerr
	}

	imagesLen := len(images) - 1
	re := regexp.MustCompile(`^(\d+)([_-][a-zA-Z0-9-_]+)?$`)

	for i, image := range images {
		if image.IsDir() {
			continue
		}

		fileName := image.Name()
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName))

		apiImage, err := p.AddExtensionImage(ctx, extensionId, path.Join(directory, image.Name()))
		if err != nil {
			return fmt.Errorf("cannot upload image %s to extension: %w", image.Name(), err)
		}

		matches := re.FindStringSubmatch(fileName)

		if matches == nil {
			logging.FromContext(ctx).Warnf("Invalid image name %s, skipping", image.Name())
			continue
		}

		priority, err := strconv.Atoi(matches[1])
		if err != nil {
			logging.FromContext(ctx).Warnf("Unexpected error: \"%s\", skipping", err)
			continue
		}

		apiImage.Priority = priority
		apiImage.Details[0].Activated = false
		apiImage.Details[0].Preview = false
		apiImage.Details[1].Activated = false
		apiImage.Details[1].Preview = false

		if index == 0 {
			apiImage.Details[0].Activated = true
			apiImage.Details[0].Preview = imagesLen-i == 0
		} else {
			apiImage.Details[1].Activated = true
			apiImage.Details[1].Preview = imagesLen-i == 0
		}

		if err := p.UpdateExtensionImage(ctx, extensionId, apiImage); err != nil {
			return fmt.Errorf("cannot update image information of extension: %w", err)
		}
	}

	return nil
}
