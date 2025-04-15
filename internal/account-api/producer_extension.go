package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/microcosm-cc/bluemonday"
	"github.com/shyim/go-version"
)

type SoftwareVersionList []SoftwareVersion

type ExtensionBinary struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"status"`
	CompatibleSoftwareVersions SoftwareVersionList `json:"compatibleSoftwareVersions"`
	Changelogs                 []struct {
		Id     int `json:"id"`
		Locale struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"locale"`
		Text string `json:"text"`
	} `json:"changelogs"`
	CreationDate                string `json:"creationDate"`
	LastChangeDate              string `json:"lastChangeDate"`
	IonCubeEncrypted            bool   `json:"ionCubeEncrypted"`
	LicenseCheckRequired        bool   `json:"licenseCheckRequired"`
	HasActiveCodeReviewWarnings bool   `json:"hasActiveCodeReviewWarnings"`
}

type ExtensionUpdate struct {
	Id                   int
	SoftwareVersions     []string                   `json:"softwareVersions"`
	IonCubeEncrypted     bool                       `json:"ionCubeEncrypted"`
	LicenseCheckRequired bool                       `json:"licenseCheckRequired"`
	Changelogs           []ExtensionUpdateChangelog `json:"changelogs"`
}

type ExtensionUpdateChangelog struct {
	Locale string `json:"locale"`
	Text   string `json:"text"`
}

type ExtensionCreate struct {
	SoftwareVersions []string                   `json:"softwareVersions"`
	Changelogs       []ExtensionUpdateChangelog `json:"changelogs"`
	Version          string                     `json:"version"`
}

func (e ProducerEndpoint) GetExtensionBinaries(ctx context.Context, extensionId int) ([]*ExtensionBinary, error) {
	errorFormat := "GetExtensionBinaries: %v"

	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/producers/%d/plugins/%d/binaries", ApiUrl, e.producerId, extensionId), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var binaries []*ExtensionBinary
	if err := json.Unmarshal(body, &binaries); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return binaries, nil
}

func (e ProducerEndpoint) UpdateExtensionBinaryInfo(ctx context.Context, extensionId int, update ExtensionUpdate) error {
	errorFormat := "UpdateExtensionBinaryInfo: %v"

	content, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "PUT", fmt.Sprintf("%s/producers/%d/plugins/%d/binaries/%d", ApiUrl, e.producerId, extensionId, update.Id), bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) CreateExtensionBinary(ctx context.Context, extensionId int, create ExtensionCreate) (*ExtensionBinary, error) {
	errorFormat := "CreateExtensionBinary: %v"

	createPayload, err := json.Marshal(create)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/producers/%d/plugins/%d/binaries", ApiUrl, e.producerId, extensionId), bytes.NewReader(createPayload))
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	content, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var binary *ExtensionBinary
	if err := json.Unmarshal(content, &binary); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return binary, nil
}

func (e ProducerEndpoint) UpdateExtensionBinaryFile(ctx context.Context, extensionId, binaryId int, zipPath string) error {
	errorFormat := "UpdateExtensionBinaryFile: %v"

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fileWritter, err := w.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	zipFile, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	if _, err = io.Copy(fileWritter, zipFile); err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/producers/%d/plugins/%d/binaries/%d/file", ApiUrl, e.producerId, extensionId, binaryId), &b)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r.Header.Set("content-type", w.FormDataContentType())

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) UpdateExtensionIcon(ctx context.Context, extensionId int, iconFilePath string) error {
	errorFormat := "UpdateExtensionIcon: %v"

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fileWritter, err := w.CreateFormFile("file", filepath.Base(iconFilePath))
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	iconFile, err := os.Open(iconFilePath)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	imageConfig, _, err := image.DecodeConfig(iconFile)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	if imageConfig.Width != 256 || imageConfig.Height != 256 {
		return fmt.Errorf(errorFormat, "extension icon must be 256x256")
	}

	if _, err = iconFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	if _, err = io.Copy(fileWritter, iconFile); err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/plugins/%d/icon", ApiUrl, extensionId), &b)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r.Header.Set("content-type", w.FormDataContentType())

	_, err = e.c.doRequest(r)

	return err
}

type ExtensionImage struct {
	Id         int    `json:"id"`
	RemoteLink string `json:"remoteLink"`
	Details    []struct {
		Id        int    `json:"id"`
		Preview   bool   `json:"preview"`
		Activated bool   `json:"activated"`
		Caption   string `json:"caption"`
		Locale    Locale `json:"locale"`
	} `json:"details"`
	Priority int `json:"priority"`
}

func (e ProducerEndpoint) GetExtensionImages(ctx context.Context, extensionId int) ([]*ExtensionImage, error) {
	errorFormat := "GetExtensionImages: %v"

	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/plugins/%d/pictures", ApiUrl, extensionId), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var images []*ExtensionImage
	if err := json.Unmarshal(body, &images); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return images, nil
}

func (e ProducerEndpoint) DeleteExtensionImages(ctx context.Context, extensionId, imageId int) error {
	errorFormat := "DeleteExtensionImages: %v"

	r, err := e.c.NewAuthenticatedRequest(ctx, "DELETE", fmt.Sprintf("%s/plugins/%d/pictures/%d", ApiUrl, extensionId, imageId), nil)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) UpdateExtensionImage(ctx context.Context, extensionId int, image *ExtensionImage) error {
	errorFormat := "UpdateExtensionImage: %v"

	content, err := json.Marshal(image)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "PUT", fmt.Sprintf("%s/plugins/%d/pictures/%d", ApiUrl, extensionId, image.Id), bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) AddExtensionImage(ctx context.Context, extensionId int, file string) (*ExtensionImage, error) {
	errorFormat := "AddExtensionImage: %v"

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fileWritter, err := w.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	zipFile, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	if _, err = io.Copy(fileWritter, zipFile); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	if err = w.Close(); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/plugins/%d/pictures", ApiUrl, extensionId), &b)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	r.Header.Set("content-type", w.FormDataContentType())

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var list []*ExtensionImage

	if err = json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("AddExtensionImage: %v", err)
	}

	return list[0], nil
}

func (e ProducerEndpoint) TriggerCodeReview(ctx context.Context, extensionId int) error {
	errorFormat := "TriggerCodeReview: %v"

	r, err := e.c.NewAuthenticatedRequest(ctx, "POST", fmt.Sprintf("%s/plugins/%d/reviews", ApiUrl, extensionId), nil)
	if err != nil {
		return fmt.Errorf(errorFormat, err)
	}

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) GetBinaryReviewResults(ctx context.Context, extensionId, binaryId int) ([]BinaryReviewResult, error) {
	errorFormat := "GetBinaryReviewResults: %v"

	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/plugins/%d/binaries/%d/checkresults", ApiUrl, extensionId, binaryId), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var results []BinaryReviewResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return results, nil
}

type BinaryReviewResult struct {
	Id       int `json:"id"`
	BinaryId int `json:"binaryId"`
	Type     struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"type"`
	Message         string `json:"message"`
	CreationDate    string `json:"creationDate"`
	SubCheckResults []struct {
		SubCheck    string `json:"subCheck"`
		Status      string `json:"status"`
		Passed      bool   `json:"passed"`
		Message     string `json:"message"`
		HasWarnings bool   `json:"hasWarnings"`
	} `json:"subCheckResults"`
}

func (review BinaryReviewResult) HasPassed() bool {
	return review.Type.Id == 3 || review.Type.Name == "automaticcodereviewsucceeded"
}

func (review BinaryReviewResult) HasWarnings() bool {
	for _, result := range review.SubCheckResults {
		if result.HasWarnings {
			return true
		}
	}

	return false
}

func (review BinaryReviewResult) IsPending() bool {
	return review.Type.Id == 4
}

func (review BinaryReviewResult) GetSummary() string {
	message := ""

	p := bluemonday.NewPolicy()

	for _, result := range review.SubCheckResults {
		if result.Passed && !result.HasWarnings {
			continue
		}

		message += fmt.Sprintf("=== %s ===\n", result.SubCheck)
		message += fmt.Sprintf("%s\n\n", p.Sanitize(result.Message))
	}

	return message
}

func (list SoftwareVersionList) FilterOnVersion(constriant *version.Constraints) SoftwareVersionList {
	newList := make(SoftwareVersionList, 0)

	for _, swVersion := range list {
		if !swVersion.Selectable {
			continue
		}

		v, err := version.NewVersion(swVersion.Name)
		if err != nil {
			continue
		}

		if constriant.Check(v) {
			newList = append(newList, swVersion)
		}
	}

	return newList
}

func (list SoftwareVersionList) FilterOnVersionStringList(constriant *version.Constraints) []string {
	newList := make([]string, 0)

	for _, swVersion := range list {
		if !swVersion.Selectable {
			continue
		}

		v, err := version.NewVersion(swVersion.Name)
		if err != nil {
			continue
		}

		if constriant.Check(v) {
			newList = append(newList, swVersion.Name)
		}
	}

	return newList
}
