package admin_sdk

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/shyim/go-version"
)

type ExtensionManagerService ClientService

func (e ExtensionManagerService) Refresh(ctx ApiContext) (*http.Response, error) {
	r, err := e.Client.NewRequest(ctx, http.MethodPost, "/api/_action/extension/refresh", nil)

	if err != nil {
		return nil, fmt.Errorf("cannot refresh extension manager %w", err)
	}

	return e.Client.BareDo(ctx.Context, r)
}

func (e ExtensionManagerService) ListAvailableExtensions(ctx ApiContext) (ExtensionList, *http.Response, error) {
	r, err := e.Client.NewRequest(ctx, http.MethodGet, "/api/_action/extension/installed", nil)

	if err != nil {
		return nil, nil, fmt.Errorf("cannot list installed extensions %w", err)
	}

	var extensions ExtensionList
	resp, err := e.Client.Do(ctx.Context, r, &extensions)

	if err != nil {
		return nil, nil, err
	}

	return extensions, resp, err
}

func (e ExtensionManagerService) lifecycleUpdate(typeName string, ctx ApiContext, httpUrl, httpMethod string) (*http.Response, error) {
	r, err := e.Client.NewRequest(ctx, httpMethod, httpUrl, nil)

	if err != nil {
		return nil, fmt.Errorf("cannot %s %w", typeName, err)
	}

	return e.Client.BareDo(ctx.Context, r)
}

func (e ExtensionManagerService) InstallExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	return e.lifecycleUpdate("InstallExtension", ctx, fmt.Sprintf("/api/_action/extension/install/%s/%s", extType, name), http.MethodPost)
}

func (e ExtensionManagerService) UninstallExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	return e.lifecycleUpdate("UninstallExtension", ctx, fmt.Sprintf("/api/_action/extension/uninstall/%s/%s", extType, name), http.MethodPost)
}

func (e ExtensionManagerService) UpdateExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	return e.lifecycleUpdate("UpdateExtension", ctx, fmt.Sprintf("/api/_action/extension/update/%s/%s", extType, name), http.MethodPost)
}

func (e ExtensionManagerService) DownloadExtension(ctx ApiContext, name string) (*http.Response, error) {
	return e.lifecycleUpdate("DownloadExtension", ctx, fmt.Sprintf("/api/_action/extension/download/%s", name), http.MethodPost)
}

func (e ExtensionManagerService) ActivateExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	return e.lifecycleUpdate("ActivateExtension", ctx, fmt.Sprintf("/api/_action/extension/activate/%s/%s", extType, name), http.MethodPut)
}

func (e ExtensionManagerService) DeactivateExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	return e.lifecycleUpdate("DeactivateExtension", ctx, fmt.Sprintf("/api/_action/extension/deactivate/%s/%s", extType, name), http.MethodPut)
}

func (e ExtensionManagerService) RemoveExtension(ctx ApiContext, extType, name string) (*http.Response, error) {
	// Since 6.6.10.2 is it POST instead of DELETE
	if version.MustConstraints(version.NewConstraint(">=6.6.10.2")).Check(e.Client.ShopwareVersion) {
		return e.lifecycleUpdate("RemoveExtension", ctx, fmt.Sprintf("/api/_action/extension/remove/%s/%s", extType, name), http.MethodPost)
	}

	return e.lifecycleUpdate("RemoveExtension", ctx, fmt.Sprintf("/api/_action/extension/remove/%s/%s", extType, name), http.MethodDelete)
}

func (e ExtensionManagerService) UploadExtension(ctx ApiContext, extensionZip io.Reader) (*http.Response, error) {
	var buf bytes.Buffer
	parts := multipart.NewWriter(&buf)
	mimeHeader := textproto.MIMEHeader{}
	mimeHeader.Set("Content-Disposition", `form-data; name="file"; filename="extension.zip"`)
	mimeHeader.Set("Content-Type", "application/zip")

	part, err := parts.CreatePart(mimeHeader)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, extensionZip); err != nil {
		return nil, err
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}

	var body io.Reader = &buf

	r, err := e.Client.NewRawRequest(ctx, http.MethodPost, "/api/_action/extension/upload", body)

	if err != nil {
		return nil, fmt.Errorf("cannot upload extension %w", err)
	}

	r.Header.Set("Content-Type", parts.FormDataContentType())

	return e.Client.BareDo(ctx.Context, r)
}

func (e ExtensionManagerService) UploadExtensionUpdateToCloud(ctx ApiContext, extensionName string, extensionZip io.Reader) (*http.Response, error) {
	var buf bytes.Buffer
	parts := multipart.NewWriter(&buf)

	if writer, err := parts.CreateFormField("media"); err != nil {
		return nil, err
	} else {
		_, err := writer.Write([]byte(extensionName))
		if err != nil {
			return nil, err
		}
	}

	mimeHeader := textproto.MIMEHeader{}
	mimeHeader.Set("Content-Disposition", `form-data; name="file"; filename="extension.zip"`)
	mimeHeader.Set("Content-Type", "application/zip")

	part, err := parts.CreatePart(mimeHeader)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, extensionZip); err != nil {
		return nil, err
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}

	var body io.Reader = &buf

	r, err := e.Client.NewRawRequest(ctx, http.MethodPost, "/api/_action/extension/update-private", body)

	if err != nil {
		return nil, fmt.Errorf("cannot upload extension update to cloud %w", err)
	}

	r.Header.Set("Content-Type", parts.FormDataContentType())

	return e.Client.BareDo(ctx.Context, r)
}

type ExtensionList []*ExtensionDetail

func (l ExtensionList) GetByName(name string) *ExtensionDetail {
	for _, detail := range l {
		if detail.Name == name {
			return detail
		}
	}

	return nil
}

func (l ExtensionList) FilterByUpdateable() ExtensionList {
	newList := make(ExtensionList, 0)

	for _, detail := range l {
		if detail.IsUpdateAble() {
			newList = append(newList, detail)
		}
	}

	return newList
}

type ExtensionDetail struct {
	Extensions             []interface{} `json:"extensions"`
	Id                     interface{}   `json:"id"`
	LocalId                string        `json:"localId"`
	Name                   string        `json:"name"`
	Label                  string        `json:"label"`
	Description            string        `json:"description"`
	ShortDescription       interface{}   `json:"shortDescription"`
	ProducerName           string        `json:"producerName"`
	License                string        `json:"license"`
	Version                string        `json:"version"`
	LatestVersion          string        `json:"latestVersion"`
	Languages              []interface{} `json:"languages"`
	Rating                 interface{}   `json:"rating"`
	NumberOfRatings        int           `json:"numberOfRatings"`
	Variants               []interface{} `json:"variants"`
	Faq                    []interface{} `json:"faq"`
	Binaries               []interface{} `json:"binaries"`
	Images                 []interface{} `json:"images"`
	Icon                   interface{}   `json:"icon"`
	IconRaw                *string       `json:"iconRaw"`
	Categories             []interface{} `json:"categories"`
	Permissions            interface{}   `json:"permissions"`
	Active                 bool          `json:"active"`
	Type                   string        `json:"type"`
	IsTheme                bool          `json:"isTheme"`
	Configurable           bool          `json:"configurable"`
	PrivacyPolicyExtension interface{}   `json:"privacyPolicyExtension"`
	StoreLicense           interface{}   `json:"storeLicense"`
	StoreExtension         interface{}   `json:"storeExtension"`
	InstalledAt            *struct {
		Date         string `json:"date"`
		TimezoneType int    `json:"timezone_type"`
		Timezone     string `json:"timezone"`
	} `json:"installedAt"`
	UpdatedAt    interface{}   `json:"updatedAt"`
	Notices      []interface{} `json:"notices"`
	Source       string        `json:"source"`
	UpdateSource string        `json:"updateSource"`
}

type Extension = ExtensionDetail

func (e ExtensionDetail) Status() string {
	var text string

	switch {
	case e.Source == "store":
		text = "can be downloaded from store"
	case e.Active:
		text = "installed, activated"
	case e.InstalledAt != nil:
		text = "installed, not activated"
	default:
		text = "not installed, not activated"
	}

	if e.IsUpdateAble() {
		text = fmt.Sprintf("%s, update available to %s", text, e.LatestVersion)
	}

	return text
}

func (e ExtensionDetail) IsPlugin() bool {
	return e.Type == "plugin"
}

func (e ExtensionDetail) IsUpdateAble() bool {
	return len(e.LatestVersion) > 0 && e.LatestVersion != e.Version
}
