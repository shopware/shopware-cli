package producer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/markdown"
	"github.com/shopware/shopware-cli/logging"
)

// PushStoreInfo updates the store page of the given extension in the account
// from the local extension configuration.
func PushStoreInfo(ctx context.Context, api StoreInfoPushAPI, zipExt extension.Extension) error {
	zipName, err := zipExt.GetName()
	if err != nil {
		return fmt.Errorf("cannot get name: %w", err)
	}

	storeExt, err := api.GetExtensionByName(ctx, zipName)
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

	info, err := api.GetExtensionGeneralInfo(ctx)
	if err != nil {
		return fmt.Errorf("cannot get general info: %w", err)
	}

	extCfg := zipExt.GetExtensionConfig()

	if extCfg != nil {
		if extCfg.Store.Icon != nil {
			err := api.UpdateExtensionIcon(ctx, storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), *extCfg.Store.Icon))
			if err != nil {
				return fmt.Errorf("cannot update extension icon due error: %w", err)
			}
		}

		if extCfg.Store.Images != nil || extCfg.Store.ImageDirectory != nil {
			images, err := api.GetExtensionImages(ctx, storeExt.Id)
			if err != nil {
				return fmt.Errorf("cannot get images from remote server: %w", err)
			}

			for _, image := range images {
				err := api.DeleteExtensionImages(ctx, storeExt.Id, image.Id)
				if err != nil {
					return fmt.Errorf("cannot extension image: %w", err)
				}
			}

			if extCfg.Store.ImageDirectory != nil {
				if err := uploadImagesByDirectory(ctx, storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 0, api); err != nil {
					return err
				}

				if err := uploadImagesByDirectory(ctx, storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 1, api); err != nil {
					return err
				}
			} else {
				// manually specified images
				for _, configImage := range *extCfg.Store.Images {
					apiImage, err := api.AddExtensionImage(ctx, storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), configImage.File))
					if err != nil {
						return fmt.Errorf("cannot upload image %s to extension: %w", configImage.File, err)
					}

					apiImage.Priority = configImage.Priority
					apiImage.Details[0].Activated = configImage.Activate.German
					apiImage.Details[0].Preview = configImage.Preview.German

					apiImage.Details[1].Activated = configImage.Activate.English
					apiImage.Details[1].Preview = configImage.Preview.English

					err = api.UpdateExtensionImage(ctx, storeExt.Id, apiImage)
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

	err = api.UpdateExtension(ctx, storeExt)
	if err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Store information has been updated")

	return nil
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

	if cfg.Store.DemoShops != nil {
		newDemos, err := buildExtensionDemos(*cfg.Store.DemoShops, info)
		if err != nil {
			return err
		}

		ext.Demos = newDemos
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

// buildExtensionDemos maps the configured demo shops to the API representation. The type and the
// localization are only known by name in the configuration, so both are resolved against the
// general information of the account API.
func buildExtensionDemos(configDemos []extension.ConfigStoreDemoShop, info *accountApi.ExtensionGeneralInformation) ([]accountApi.ExtensionDemo, error) {
	newDemos := make([]accountApi.ExtensionDemo, 0)

	for _, configDemo := range configDemos {
		demoType, err := findDemoType(configDemo.Type, info.DemoTypes)
		if err != nil {
			return nil, err
		}

		localization, err := findLocale(configDemo.Localization, info.Locales)
		if err != nil {
			return nil, err
		}

		newDemos = append(newDemos, accountApi.ExtensionDemo{
			Type:          *demoType,
			Link:          configDemo.Link,
			Localization:  *localization,
			LoginName:     configDemo.LoginName,
			LoginPassword: configDemo.LoginPassword,
		})
	}

	return newDemos, nil
}

func findDemoType(name string, demoTypes []accountApi.StoreDemoType) (*accountApi.StoreDemoType, error) {
	for i, demoType := range demoTypes {
		if demoType.Name == name {
			return &demoTypes[i], nil
		}
	}

	possible := make([]string, 0, len(demoTypes))
	for _, demoType := range demoTypes {
		possible = append(possible, demoType.Name)
	}

	return nil, fmt.Errorf("unknown demo shop type %q, possible values are: %s", name, strings.Join(possible, ", "))
}

func findLocale(name string, locales []accountApi.Locale) (*accountApi.Locale, error) {
	for i, locale := range locales {
		if locale.Name == name {
			return &locales[i], nil
		}
	}

	possible := make([]string, 0, len(locales))
	for _, locale := range locales {
		possible = append(possible, locale.Name)
	}

	return nil, fmt.Errorf("unknown demo shop localization %q, possible values are: %s", name, strings.Join(possible, ", "))
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

	html, err := markdown.ToHTML(content)
	if err != nil {
		return "", fmt.Errorf("cannot convert file at path %s from markdown to html with error: %v", filePath, err)
	}

	return html, nil
}

func uploadImagesByDirectory(ctx context.Context, extensionId int, directory string, index int, api StoreInfoPushAPI) error {
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

		apiImage, err := api.AddExtensionImage(ctx, extensionId, path.Join(directory, image.Name()))
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

		if err := api.UpdateExtensionImage(ctx, extensionId, apiImage); err != nil {
			return fmt.Errorf("cannot update image information of extension: %w", err)
		}
	}

	return nil
}
