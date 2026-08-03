package account_api

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/markdown"
	"github.com/shopware/shopware-cli/logging"
)

// PushExtensionStoreInfo synchronizes the local extension metadata and store
// configuration with the Shopware Account.
func PushExtensionStoreInfo(ctx context.Context, producer ProducerAPI, zipExt extension.Extension) error {
	zipName, err := zipExt.GetName()
	if err != nil {
		return fmt.Errorf("cannot get name: %w", err)
	}

	storeExt, err := producer.GetExtensionByName(ctx, zipName)
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

	info, err := producer.GetExtensionGeneralInfo(ctx)
	if err != nil {
		return fmt.Errorf("cannot get general info: %w", err)
	}

	extCfg := zipExt.GetExtensionConfig()

	if extCfg != nil {
		if extCfg.Store.Icon != nil {
			err := producer.UpdateExtensionIcon(ctx, storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), *extCfg.Store.Icon))
			if err != nil {
				return fmt.Errorf("cannot update extension icon due error: %w", err)
			}
		}

		if extCfg.Store.Images != nil || extCfg.Store.ImageDirectory != nil {
			images, err := producer.GetExtensionImages(ctx, storeExt.Id)
			if err != nil {
				return fmt.Errorf("cannot get images from remote server: %w", err)
			}

			for _, image := range images {
				err := producer.DeleteExtensionImages(ctx, storeExt.Id, image.Id)
				if err != nil {
					return fmt.Errorf("cannot extension image: %w", err)
				}
			}

			if extCfg.Store.ImageDirectory != nil {
				if err := uploadImagesByDirectory(ctx, producer, storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 0); err != nil {
					return err
				}

				if err := uploadImagesByDirectory(ctx, producer, storeExt.Id, path.Join(zipExt.GetPath(), *extCfg.Store.ImageDirectory), 1); err != nil {
					return err
				}
			} else {
				// manually specified images
				for _, configImage := range *extCfg.Store.Images {
					apiImage, err := producer.AddExtensionImage(ctx, storeExt.Id, fmt.Sprintf("%s/%s", zipExt.GetPath(), configImage.File))
					if err != nil {
						return fmt.Errorf("cannot upload image %s to extension: %w", configImage.File, err)
					}

					apiImage.Priority = configImage.GetPosition()
					apiImage.Details[0].Activated = configImage.Activate.German
					apiImage.Details[0].Preview = configImage.Preview.German

					apiImage.Details[1].Activated = configImage.Activate.English
					apiImage.Details[1].Preview = configImage.Preview.English

					err = producer.UpdateExtensionImage(ctx, storeExt.Id, apiImage)
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

	if err := producer.UpdateExtension(ctx, storeExt); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Store information has been updated")

	return nil
}

func updateStoreInfo(ext *Extension, zipExt extension.Extension, cfg *extension.Config, info *ExtensionGeneralInformation) error { //nolint:gocyclo
	if cfg.Store.DefaultLocale != nil {
		for _, locale := range info.Locales {
			if locale.Name == *cfg.Store.DefaultLocale {
				ext.StandardLocale = locale
			}
		}
	}

	if cfg.Store.Localizations != nil {
		ext.Localizations = mapSlice(*cfg.Store.Localizations, func(locale string) Locale {
			return Locale{Name: locale}
		})
	}

	if cfg.Store.Availabilities != nil {
		newAvailabilities := make([]StoreAvailablity, 0)

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

		applyTranslated(language, cfg.Store.Tags, func(tags []string) {
			info.Tags = mapSlice(tags, func(tag string) StoreTag { return StoreTag{Name: tag} })
		})

		applyTranslated(language, cfg.Store.Videos, func(videos []string) {
			info.Videos = mapSlice(videos, func(video string) StoreVideo { return StoreVideo{URL: video} })
		})

		applyTranslated(language, cfg.Store.Highlights, func(highlights []string) {
			info.Highlights = strings.Join(highlights, "\n")
		})

		applyTranslated(language, cfg.Store.Features, func(features []string) {
			info.Features = strings.Join(features, "\n")
		})

		applyTranslated(language, cfg.Store.Faq, func(faqs []extension.ConfigStoreFaq) {
			info.Faqs = mapSlice(faqs, func(faq extension.ConfigStoreFaq) StoreFaq {
				return StoreFaq{Question: faq.Question, Answer: faq.Answer, Position: faq.Position}
			})
		})

		var err error

		if storeDescription := getTranslation(language, cfg.Store.Description); storeDescription != nil {
			info.Description, err = parseInlineablePath(*storeDescription, zipExt.GetPath())
			if err != nil {
				return err
			}
		}

		if storeManual := getTranslation(language, cfg.Store.InstallationManual); storeManual != nil {
			info.InstallationManual, err = parseInlineablePath(*storeManual, zipExt.GetPath())
			if err != nil {
				return err
			}
		}

		applyTranslated(language, cfg.Store.MetaTitle, func(metaTitle string) {
			info.MetaTitle = metaTitle
		})

		applyTranslated(language, cfg.Store.MetaDescription, func(metaDescription string) {
			info.MetaDescription = metaDescription
		})
	}

	return nil
}

// buildExtensionDemos maps the configured demo shops to the API representation. The type and the
// localization are only known by name in the configuration, so both are resolved against the
// general information of the account API.
func buildExtensionDemos(configDemos []extension.ConfigStoreDemoShop, info *ExtensionGeneralInformation) ([]ExtensionDemo, error) {
	newDemos := make([]ExtensionDemo, 0)

	for _, configDemo := range configDemos {
		demoType, err := findDemoType(configDemo.Type, info.DemoTypes)
		if err != nil {
			return nil, err
		}

		localization, err := findLocale(configDemo.Localization, info.Locales)
		if err != nil {
			return nil, err
		}

		newDemos = append(newDemos, ExtensionDemo{
			Type:          *demoType,
			Link:          configDemo.Link,
			Localization:  *localization,
			LoginName:     configDemo.LoginName,
			LoginPassword: configDemo.LoginPassword,
		})
	}

	return newDemos, nil
}

func findDemoType(name string, demoTypes []StoreDemoType) (*StoreDemoType, error) {
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

func findLocale(name string, locales []Locale) (*Locale, error) {
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

func getTranslation[T any](language string, config extension.ConfigTranslated[T]) *T {
	switch language {
	case "de":
		return config.German
	case "en":
		return config.English
	}

	return nil
}

// applyTranslated calls set with the configured value of the given language if one exists.
func applyTranslated[T any](language string, config extension.ConfigTranslated[T], set func(T)) {
	if value := getTranslation(language, config); value != nil {
		set(*value)
	}
}

// mapSlice converts each element of values using the given function.
func mapSlice[A, B any](values []A, convert func(A) B) []B {
	mapped := make([]B, 0, len(values))
	for _, value := range values {
		mapped = append(mapped, convert(value))
	}
	return mapped
}

// parseInlineablePath returns the given value as is, unless it starts with "file:". In that case
// the referenced file is read relative to the extension directory. Markdown files are converted
// to HTML.
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

// uploadImagesByDirectory uploads all images of the given language directory (index 0 is german,
// 1 is english, defined by the account api) and derives the priority from the file name.
func uploadImagesByDirectory(ctx context.Context, producer ProducerAPI, extensionId int, directory string, index int) error {
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

		apiImage, err := producer.AddExtensionImage(ctx, extensionId, path.Join(directory, image.Name()))
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

		if err := producer.UpdateExtensionImage(ctx, extensionId, apiImage); err != nil {
			return fmt.Errorf("cannot update image information of extension: %w", err)
		}
	}

	return nil
}
