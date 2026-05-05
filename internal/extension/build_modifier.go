package extension

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/xmlpath"
)

type BuildModifierConfig struct {
	AppBackendUrl    string
	AppBackendSecret string
	Version          string
}

func BuildModifier(ext Extension, extensionRoot string, config BuildModifierConfig) error {
	if (config.AppBackendUrl != "" || config.AppBackendSecret != "" || config.Version != "") && ext.GetType() == TypePlatformApp {
		manifestBytes, _ := os.ReadFile(path.Join(extensionRoot, "manifest.xml"))

		manifest, err := xmlpath.Parse(manifestBytes)
		if err != nil {
			return fmt.Errorf("could not parse manifest.xml: %w", err)
		}
		manifestRoot := manifest.Root()

		if config.Version != "" {
			manifestRoot.EnsurePath("meta/version").SetText(config.Version)
		}

		if config.AppBackendSecret != "" {
			if setup := manifestRoot.Find("setup"); setup != nil {
				setup.EnsurePath("secret").SetText(config.AppBackendSecret)
			}
		}

		if config.AppBackendUrl != "" {
			if err := replaceUrlsInManifest(config, manifestRoot); err != nil {
				return err
			}
		}

		newXml, err := manifest.MarshalIndent("", "  ")

		if err != nil {
			return fmt.Errorf("could not marshal manifest.xml: %w", err)
		}

		if err := os.WriteFile(path.Join(extensionRoot, "manifest.xml"), newXml, os.ModePerm); err != nil {
			return fmt.Errorf("could not write manifest.xml: %w", err)
		}
	}

	if config.Version != "" && ext.GetType() == TypePlatformPlugin {
		composerJson, err := os.ReadFile(path.Join(extensionRoot, "composer.json"))

		if err != nil {
			return fmt.Errorf("could not read composer.json: %w", err)
		}

		var composerJsonStruct map[string]interface{}

		if err := json.Unmarshal(composerJson, &composerJsonStruct); err != nil {
			return fmt.Errorf("could not unmarshal composer.json: %w", err)
		}

		composerJsonStruct["version"] = config.Version

		newComposerJson, err := json.MarshalIndent(composerJsonStruct, "", "  ")

		if err != nil {
			return fmt.Errorf("could not marshal composer.json: %w", err)
		}

		if err := os.WriteFile(path.Join(extensionRoot, "composer.json"), newComposerJson, os.ModePerm); err != nil {
			return fmt.Errorf("could not write composer.json: %w", err)
		}
	}

	return nil
}

func replaceUrlsInManifest(config BuildModifierConfig, manifest *xmlpath.Element) error {
	newBackendUrl, err := url.Parse(config.AppBackendUrl)

	if err != nil {
		return fmt.Errorf("could not parse app backend url: %w", err)
	}

	if registrationUrl := manifest.Find("setup/registrationUrl"); registrationUrl != nil {
		if err := replaceElementUrl(registrationUrl, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace app backend url: %w", err)
		}
	}

	if baseAppUrl := manifest.Find("admin/base-app-url"); baseAppUrl != nil {
		if err := replaceElementUrl(baseAppUrl, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace app backend url: %w", err)
		}
	}

	for index, button := range manifest.FindAll("admin/action-button") {
		if err := replaceElementAttrUrl(button, "url", newBackendUrl); err != nil {
			return fmt.Errorf("could not replace action button url on index %d: %w", index, err)
		}
	}

	if checkout := manifest.Find("gateways/checkout"); checkout != nil {
		if err := replaceElementUrl(checkout, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace checkout gateway url: %w", err)
		}
	}

	for _, payment := range manifest.FindAll("payments/payment-method") {
		for _, child := range []struct {
			name        string
			description string
		}{
			{"refund-url", "refund url"},
			{"capture-url", "capture url"},
			{"finalize-url", "finalize url"},
			{"pay-url", "pay url"},
			{"recurring-url", "recurring url"},
			{"validate-url", "validate url"},
		} {
			urlElement := payment.Find(child.name)
			if urlElement == nil {
				continue
			}
			if err := replaceElementUrl(urlElement, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace %s: %w", child.description, err)
			}
		}
	}

	for _, tax := range manifest.FindAll("tax/tax-provider") {
		processURL := tax.Find("process-url")
		if processURL == nil {
			continue
		}
		if err := replaceElementUrl(processURL, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace tax provider url: %w", err)
		}
	}

	for _, webhook := range manifest.FindAll("webhooks/webhook") {
		if err := replaceElementAttrUrl(webhook, "url", newBackendUrl); err != nil {
			return fmt.Errorf("could not replace webhook url: %w", err)
		}
	}
	return nil
}

func replaceElementUrl(element *xmlpath.Element, backendUrl *url.URL) error {
	replaced, err := replaceUrl(element.Text(), backendUrl)
	if err != nil {
		return err
	}
	element.SetText(replaced)
	return nil
}

func replaceElementAttrUrl(element *xmlpath.Element, attr string, backendUrl *url.URL) error {
	currentURL, ok := element.Attr(attr)
	if !ok {
		return nil
	}

	replaced, err := replaceUrl(currentURL, backendUrl)
	if err != nil {
		return err
	}
	element.SetAttr(attr, replaced)
	return nil
}

func replaceUrl(registrationUrl string, backendUrl *url.URL) (string, error) {
	if registrationUrl == "" {
		return registrationUrl, nil
	}

	currentUrl, err := url.Parse(registrationUrl)

	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	currentUrl.Scheme = backendUrl.Scheme
	currentUrl.Host = backendUrl.Host

	return currentUrl.String(), nil
}
