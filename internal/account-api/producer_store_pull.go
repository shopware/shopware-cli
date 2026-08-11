package account_api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

type PullOptions struct {
	// HTTPClient is used to download the icon and images. Defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// PullExtensionStoreInfo generates the local store configuration from the account data and
// downloads all store assets into the extension folder.
func PullExtensionStoreInfo(ctx context.Context, producer ProducerAPI, zipExt extension.Extension, opts PullOptions) error {
	zipName, err := zipExt.GetName()
	if err != nil {
		return fmt.Errorf("cannot get extension name: %w", err)
	}

	storeExt, err := producer.GetExtensionByName(ctx, zipName)
	if err != nil {
		return fmt.Errorf("cannot get store extension: %w", err)
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resourcesFolder := path.Join(zipExt.GetPath(), "src/Resources/store/")
	availabilities := make([]string, 0)
	localizations := make([]string, 0)

	if _, err := os.Stat(resourcesFolder); os.IsNotExist(err) {
		err = os.MkdirAll(resourcesFolder, 0o755)
		if err != nil {
			return fmt.Errorf("cannot create file: %w", err)
		}
	}

	var iconConfigPath *string

	if len(storeExt.IconURL) > 0 {
		icon := "src/Resources/store/icon.png"
		iconConfigPath = &icon
		err := downloadFileTo(ctx, httpClient, storeExt.IconURL, path.Join(resourcesFolder, "icon.png"))
		if err != nil {
			return fmt.Errorf("cannot download file: %w", err)
		}
	}

	for _, localization := range storeExt.Localizations {
		localizations = append(localizations, localization.Name)
	}

	for _, a := range storeExt.StoreAvailabilities {
		availabilities = append(availabilities, a.Name)
	}

	demoShops := make([]extension.ConfigStoreDemoShop, 0)

	for _, demo := range storeExt.Demos {
		demoShops = append(demoShops, extension.ConfigStoreDemoShop{
			Type:          demo.Type.Name,
			Link:          demo.Link,
			Localization:  demo.Localization.Name,
			LoginName:     demo.LoginName,
			LoginPassword: demo.LoginPassword,
		})
	}

	storeImages, err := producer.GetExtensionImages(ctx, storeExt.Id)
	if err != nil {
		return fmt.Errorf("cannot get extension images: %w", err)
	}

	if len(storeImages) > 0 {
		imagesDir := path.Join(zipExt.GetPath(), "src/Resources/store/images/")

		if err := writeImages(ctx, httpClient, imagesDir, 0, storeImages); err != nil {
			return fmt.Errorf("cannot write images: %w", err)
		}

		if err := writeImages(ctx, httpClient, imagesDir, 1, storeImages); err != nil {
			return fmt.Errorf("cannot write images: %w", err)
		}
	}

	de := newStoreLanguageInfo("de")
	en := newStoreLanguageInfo("en")

	for _, info := range storeExt.Infos {
		lang := en
		if strings.HasPrefix(info.Locale.Name, "de") {
			lang = de
		}

		if err := collectStoreLanguageInfo(zipExt.GetPath(), lang, info); err != nil {
			return err
		}
	}

	if de.hasContent() || en.hasContent() {
		err = zipExt.UpdateMetaData(&extension.ExtensionMetadata{
			Label: extension.ExtensionTranslated{
				German:  de.label,
				English: en.label,
			},
			Description: extension.ExtensionTranslated{
				German:  de.shortDescription,
				English: en.shortDescription,
			},
		})
		if err != nil {
			return fmt.Errorf("cannot update extension metadata: %w", err)
		}
	}

	extType := "extension"

	if storeExt.ProductType != nil {
		extType = storeExt.ProductType.Name
	}

	newCfg := zipExt.GetExtensionConfig()

	newCfg.Store.Icon = iconConfigPath
	newCfg.Store.DefaultLocale = &storeExt.StandardLocale.Name
	newCfg.Store.Type = &extType
	newCfg.Store.AutomaticBugfixVersionCompatibility = &storeExt.AutomaticBugfixVersionCompatibility
	newCfg.Store.Availabilities = &availabilities
	newCfg.Store.Localizations = &localizations
	newCfg.Store.Description = translatedFrom(&de.description, &en.description)
	newCfg.Store.InstallationManual = translatedFrom(&de.installationManual, &en.installationManual)
	newCfg.Store.Tags = translatedFrom(&de.tags, &en.tags)
	newCfg.Store.Videos = translatedFrom(&de.videos, &en.videos)
	newCfg.Store.Highlights = translatedFrom(&de.highlights, &en.highlights)
	newCfg.Store.Features = translatedFrom(&de.features, &en.features)
	newCfg.Store.Faq = translatedFrom(&de.faqs, &en.faqs)
	newCfg.Store.MetaTitle = translatedFrom(&de.metaTitle, &en.metaTitle)
	newCfg.Store.MetaDescription = translatedFrom(&de.metaDescription, &en.metaDescription)
	newCfg.Store.DemoShops = &demoShops
	newCfg.Store.Images = nil

	if len(storeImages) > 0 {
		imageDir := "src/Resources/store/images"
		newCfg.Store.ImageDirectory = &imageDir
	}

	content, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("cannot encode yaml: %w", err)
	}

	extCfgFile := fmt.Sprintf("%s/%s", zipExt.GetPath(), newCfg.FileName)
	err = os.WriteFile(extCfgFile, content, 0o644)
	if err != nil {
		return fmt.Errorf("cannot save file: %w", err)
	}

	logging.FromContext(ctx).Infof("Files has been written to the given extension folder")

	return nil
}

// storeLanguageInfo accumulates the translated store information of one language while pulling.
type storeLanguageInfo struct {
	suffix             string
	label              string
	shortDescription   string
	description        string
	installationManual string
	metaTitle          string
	metaDescription    string
	tags               []string
	videos             []string
	highlights         []string
	features           []string
	faqs               []extension.ConfigStoreFaq
}

func newStoreLanguageInfo(suffix string) *storeLanguageInfo {
	return &storeLanguageInfo{
		suffix:     suffix,
		tags:       []string{},
		videos:     []string{},
		highlights: []string{},
		features:   []string{},
		faqs:       []extension.ConfigStoreFaq{},
	}
}

func (lang *storeLanguageInfo) hasContent() bool {
	return lang.label != "" || lang.shortDescription != ""
}

// collectStoreLanguageInfo copies one account info entry into the language accumulator and writes
// the description and installation manual into the extension folder.
func collectStoreLanguageInfo(extensionPath string, lang *storeLanguageInfo, info *ExtensionInfo) error {
	lang.label = info.Name
	lang.shortDescription = info.ShortDescription
	lang.description = fmt.Sprintf("file:src/Resources/store/description.%s.html", lang.suffix)
	lang.installationManual = fmt.Sprintf("file:src/Resources/store/installation_manual.%s.html", lang.suffix)
	lang.metaTitle = info.MetaTitle
	lang.metaDescription = info.MetaDescription

	if err := os.WriteFile(path.Join(extensionPath, lang.description[5:]), []byte(info.Description), 0o644); err != nil {
		return fmt.Errorf("cannot write file: %w", err)
	}

	if err := os.WriteFile(path.Join(extensionPath, lang.installationManual[5:]), []byte(info.InstallationManual), 0o644); err != nil {
		return fmt.Errorf("cannot write file: %w", err)
	}

	for _, element := range info.Tags {
		lang.tags = append(lang.tags, element.Name)
	}

	for _, element := range info.Videos {
		lang.videos = append(lang.videos, element.URL)
	}

	if info.Highlights != "" {
		lang.highlights = append(lang.highlights, strings.Split(info.Highlights, "\n")...)
	}

	if info.Features != "" {
		lang.features = append(lang.features, strings.Split(info.Features, "\n")...)
	}

	for _, element := range info.Faqs {
		lang.faqs = append(lang.faqs, extension.ConfigStoreFaq{Question: element.Question, Answer: element.Answer, Position: element.Position})
	}

	return nil
}

func translatedFrom[T any](german, english *T) extension.ConfigTranslated[T] {
	return extension.ConfigTranslated[T]{German: german, English: english}
}

func downloadFileTo(ctx context.Context, client *http.Client, url string, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("downloadFileTo: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download file: unexpected status %d for %s", resp.StatusCode, url)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read file body: %w", err)
	}

	err = os.WriteFile(target, content, 0o644)
	if err != nil {
		return fmt.Errorf("write to file: %w", err)
	}

	return nil
}

func writeImages(ctx context.Context, client *http.Client, imagePath string, index int, storeImages []*ExtensionImage) error {
	imageMap := make(map[int]string)

	for _, image := range storeImages {
		if index >= len(image.Details) {
			continue
		}

		if image.Details[index].Activated {
			priority := image.Priority

			if _, ok := imageMap[priority]; !ok {
				imageMap[priority] = image.RemoteLink
			} else {
				for {
					priority++
					if _, ok := imageMap[priority]; !ok {
						imageMap[priority] = image.RemoteLink
						break
					}
				}
			}
		}
	}

	if index == 0 {
		imagePath = path.Join(imagePath, "de")
	} else {
		imagePath = path.Join(imagePath, "en")
	}

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		if err := os.MkdirAll(imagePath, 0o755); err != nil {
			return err
		}
	}

	for priority, link := range imageMap {
		if err := downloadFileTo(ctx, client, link, path.Join(imagePath, fmt.Sprintf("%d.png", priority))); err != nil {
			return err
		}
	}

	return nil
}
