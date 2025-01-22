package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

type MailTemplateSync struct{}

func (MailTemplateSync) Push(ctx adminSdk.ApiContext, client *adminSdk.Client, config *shop.Config, operation *ConfigSyncOperation) error {
	mailTemplates, err := fetchAllMailTemplates(ctx, client)
	if err != nil {
		return err
	}

	mailUpdates := make([]map[string]interface{}, 0)

	for _, external := range mailTemplates.Data {
		for _, configEntry := range config.Sync.MailTemplate {
			if external.Id == configEntry.Id {
				mailUpdate := make(map[string]interface{})
				mailUpdate["id"] = configEntry.Id
				translationUpdates := make(map[string]map[string]interface{})

				for _, translation := range external.Translations {
					if translation.Language == nil {
						continue
					}

					for _, configTranslation := range configEntry.Translations {
						if translation.Language.Name == configTranslation.Language {
							translationUpdate := make(map[string]interface{})

							if translation.SenderName != configTranslation.SenderName {
								translationUpdate["senderName"] = configTranslation.SenderName
							}

							if translation.Subject != configTranslation.Subject {
								translationUpdate["subject"] = configTranslation.Subject
							}

							if configTranslation.HTML != "" {
								if content, err := os.ReadFile(configTranslation.HTML); err == nil {
									if translation.ContentHtml != string(content) {
										translationUpdate["contentHtml"] = string(content)
									}
								} else {
									logging.FromContext(ctx.Context).Errorf("Cannot read file %s, with error: %s", configTranslation.HTML, err)
								}
							}

							if configTranslation.Plain != "" {
								if content, err := os.ReadFile(configTranslation.Plain); err == nil {
									if translation.ContentPlain != string(content) {
										translationUpdate["contentPlain"] = string(content)
									}
								} else {
									logging.FromContext(ctx.Context).Errorf("Cannot read file %s, with error: %s", configTranslation.Plain, err)
								}
							}

							localCustomFields, _ := json.Marshal(configTranslation.CustomFields)
							remoteCustomFields, _ := json.Marshal(translation.CustomFields)

							if !bytes.Equal(localCustomFields, remoteCustomFields) {
								translationUpdate["customFields"] = configTranslation.CustomFields
							}

							if len(translationUpdate) > 0 {
								translationUpdates[translation.LanguageId] = translationUpdate
							}
						}
					}

					if len(translationUpdates) > 0 {
						mailUpdate["translations"] = translationUpdates
					}
				}

				if len(mailUpdate) > 1 {
					mailUpdates = append(mailUpdates, mailUpdate)
				}
			}
		}
	}

	if len(mailUpdates) > 0 {
		operation.Operations["update-mail-template"] = adminSdk.SyncOperation{
			Action:  "upsert",
			Entity:  "mail_template",
			Payload: mailUpdates,
		}
	}

	return nil
}

func (MailTemplateSync) Pull(ctx adminSdk.ApiContext, client *adminSdk.Client, config *shop.Config) error {
	mailTemplates, err := fetchAllMailTemplates(ctx, client)
	if err != nil {
		return err
	}

	config.Sync.MailTemplate = make([]shop.MailTemplate, 0)

	duplicateTypes := getDuplicateMailTemplateTypes(mailTemplates.Data)

	for _, row := range mailTemplates.Data {
		if row.MailTemplateType == nil {
			logging.FromContext(ctx.Context).Infof("mail_template entity with id %s does not have a type. Skipping", row.Id)
			continue
		}

		cfg := shop.MailTemplate{
			Id:           row.Id,
			Translations: []shop.MailTemplateTranslation{},
		}

		for _, translation := range row.Translations {
			if translation.Language == nil {
				continue
			}

			configKey := translation.Language.Name

			htmLFilePath := fmt.Sprintf(".shopware-cli/mail-template/%s/%s-html.twig", row.MailTemplateType.TechnicalName, configKey)
			plainFilePath := fmt.Sprintf(".shopware-cli/mail-template/%s/%s-plain.twig", row.MailTemplateType.TechnicalName, configKey)

			if ok := duplicateTypes[row.MailTemplateType.TechnicalName]; ok {
				htmLFilePath = fmt.Sprintf(".shopware-cli/mail-template/%s/%s-%s-html.twig", row.MailTemplateType.TechnicalName, configKey, row.Id)
				plainFilePath = fmt.Sprintf(".shopware-cli/mail-template/%s/%s-%s-plain.twig", row.MailTemplateType.TechnicalName, configKey, row.Id)
			}

			dir := filepath.Dir(htmLFilePath)

			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if createErr := os.MkdirAll(dir, os.ModePerm); createErr != nil {
					return createErr
				}
			}

			cfgLang := shop.MailTemplateTranslation{
				Language:     configKey,
				SenderName:   translation.SenderName,
				Subject:      translation.Subject,
				HTML:         htmLFilePath,
				Plain:        plainFilePath,
				CustomFields: translation.CustomFields,
			}

			if err := os.WriteFile(htmLFilePath, []byte(translation.ContentHtml), os.ModePerm); err != nil {
				return err
			}

			if err := os.WriteFile(plainFilePath, []byte(translation.ContentPlain), os.ModePerm); err != nil {
				return err
			}

			cfg.Translations = append(cfg.Translations, cfgLang)
		}

		config.Sync.MailTemplate = append(config.Sync.MailTemplate, cfg)
	}

	return nil
}

func getDuplicateMailTemplateTypes(data []adminSdk.MailTemplate) map[string]bool {
	check := make(map[string]bool)
	duplicates := make(map[string]bool)

	for _, row := range data {
		if row.MailTemplateType == nil {
			continue
		}

		if row.MailTemplateType.TechnicalName == "" {
			continue
		}

		if _, ok := check[row.MailTemplateType.TechnicalName]; ok {
			duplicates[row.MailTemplateType.TechnicalName] = true
		} else {
			check[row.MailTemplateType.TechnicalName] = true
		}
	}

	return duplicates
}

func fetchAllMailTemplates(ctx adminSdk.ApiContext, client *adminSdk.Client) (*adminSdk.MailTemplateCollection, error) {
	criteria := adminSdk.Criteria{}
	criteria.Includes = map[string][]string{
		"mail_template":             {"id", "mailTemplateType", "translations"},
		"mail_template_type":        {"technicalName"},
		"mail_template_translation": {"senderName", "subject", "contentHtml", "contentPlain", "language", "languageId"},
		"language":                  {"name"},
	}
	criteria.Associations = map[string]adminSdk.Criteria{"mailTemplateType": {}, "translations": {Associations: map[string]adminSdk.Criteria{"language": {}}}}

	collection, resp, err := client.Repository.MailTemplate.SearchAll(ctx, criteria)

	if err == nil {
		if err := resp.Body.Close(); err != nil {
			return nil, err
		}
	}

	return collection, err
}
