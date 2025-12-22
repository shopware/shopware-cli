package extension

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path"
)

type BuildModifierConfig struct {
	AppBackendUrl    string
	AppBackendSecret string
	Version          string
}

func BuildModifier(ext Extension, extensionRoot string, config BuildModifierConfig) error {
	if (config.AppBackendUrl != "" || config.AppBackendSecret != "" || config.Version != "") && ext.GetType() == TypePlatformApp {
		manifestBytes, _ := os.ReadFile(path.Join(extensionRoot, "manifest.xml"))

		var manifest Manifest

		if err := xml.Unmarshal(manifestBytes, &manifest); err != nil {
			return fmt.Errorf("could not parse manifest.xml: %w", err)
		}

		if config.Version != "" {
			manifest.Meta.Version = config.Version
		}

		if config.AppBackendSecret != "" && manifest.Setup != nil {
			manifest.Setup.Secret = config.AppBackendSecret
		}

		if config.AppBackendUrl != "" {
			if err := replaceUrlsInManifest(config, manifest); err != nil {
				return err
			}
		}

		newXml, err := xml.MarshalIndent(manifest, "", "  ")

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
			return fmt.Errorf("could not write manifest.xml: %w", err)
		}
	}

	return nil
}

func replaceUrlsInManifest(config BuildModifierConfig, manifest Manifest) error {
	newBackendUrl, err := url.Parse(config.AppBackendUrl)

	if err != nil {
		return fmt.Errorf("could not parse app backend url: %w", err)
	}

	if manifest.Setup != nil {
		if err := replaceUrl(&manifest.Setup.RegistrationUrl, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace app backend url: %w", err)
		}
	}

	if manifest.Admin != nil {
		if err := replaceUrl(&manifest.Admin.BaseAppUrl, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace app backend url: %w", err)
		}

		for index, button := range manifest.Admin.ActionButton {
			if err := replaceUrl(&button.URL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace action button url on index %d: %w", index, err)
			}
		}
	}

	if manifest.Gateways != nil {
		if err := replaceUrl(&manifest.Gateways.Checkout, newBackendUrl); err != nil {
			return fmt.Errorf("could not replace checkout gateway url: %w", err)
		}
	}

	if manifest.Payments != nil {
		for _, payment := range manifest.Payments.PaymentMethod {
			if err := replaceUrl(&payment.RefundURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace refund url: %w", err)
			}

			if err := replaceUrl(&payment.CaptureURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace capture url: %w", err)
			}

			if err := replaceUrl(&payment.FinalizeURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace finanlize url: %w", err)
			}

			if err := replaceUrl(&payment.PayURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace pay url: %w", err)
			}

			if err := replaceUrl(&payment.RecurringURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace recurring url: %w", err)
			}

			if err := replaceUrl(&payment.ValidateURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace validate url: %w", err)
			}
		}
	}

	if manifest.Tax != nil {
		for _, tax := range manifest.Tax.TaxProvider {
			if err := replaceUrl(&tax.ProcessURL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace tax provider url: %w", err)
			}
		}
	}

	if manifest.Webhooks != nil {
		for _, webhook := range manifest.Webhooks.Webhook {
			if err := replaceUrl(&webhook.URL, newBackendUrl); err != nil {
				return fmt.Errorf("could not replace webhook url: %w", err)
			}
		}
	}
	return nil
}

func replaceUrl(registrationUrl *string, backendUrl *url.URL) error {
	if registrationUrl == nil || *registrationUrl == "" {
		return nil
	}

	currentUrl, err := url.Parse(*registrationUrl)

	if err != nil {
		return fmt.Errorf("could not parse url: %w", err)
	}

	currentUrl.Scheme = backendUrl.Scheme
	currentUrl.Host = backendUrl.Host

	*registrationUrl = currentUrl.String()

	return nil
}
