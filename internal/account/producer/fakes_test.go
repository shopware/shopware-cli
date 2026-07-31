package producer

import (
	"context"

	"github.com/shyim/go-version"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

// fakeExtension is an in-memory extension.Extension implementation for tests.
type fakeExtension struct {
	name            string
	version         string
	constraint      string
	extType         string
	path            string
	changelog       extension.ExtensionChangelog
	metadata        *extension.ExtensionMetadata
	config          *extension.Config
	updatedMetadata *extension.ExtensionMetadata
}

var _ extension.Extension = (*fakeExtension)(nil)

func (f *fakeExtension) GetName() (string, error)         { return f.name, nil }
func (f *fakeExtension) GetComposerName() (string, error) { return "acme/" + f.name, nil }
func (f *fakeExtension) GetResourcesDir() string          { return f.path }
func (f *fakeExtension) GetResourcesDirs() []string       { return nil }
func (f *fakeExtension) GetIconPath() string              { return "" }
func (f *fakeExtension) GetRootDir() string               { return f.path }
func (f *fakeExtension) GetSourceDirs() []string          { return nil }
func (f *fakeExtension) GetLicense() (string, error)      { return "MIT", nil }
func (f *fakeExtension) GetType() string                  { return f.extType }
func (f *fakeExtension) GetPath() string                  { return f.path }

func (f *fakeExtension) GetVersion() (*version.Version, error) {
	return version.NewVersion(f.version)
}

func (f *fakeExtension) GetShopwareVersionConstraint() (*version.Constraints, error) {
	constraint, err := version.NewConstraint(f.constraint)
	if err != nil {
		return nil, err
	}
	return &constraint, nil
}

func (f *fakeExtension) GetChangelog() (*extension.ExtensionChangelog, error) {
	return &f.changelog, nil
}

func (f *fakeExtension) GetMetaData() *extension.ExtensionMetadata { return f.metadata }

func (f *fakeExtension) UpdateMetaData(metadata *extension.ExtensionMetadata) error {
	f.updatedMetadata = metadata
	return nil
}

func (f *fakeExtension) GetExtensionConfig() *extension.Config { return f.config }

func (f *fakeExtension) Validate(context.Context, validation.Check) {}

// fakeUploadAPI records all calls made by Upload.
type fakeUploadAPI struct {
	extension        *accountApi.Extension
	binaries         []*accountApi.ExtensionBinary
	softwareVersions accountApi.SoftwareVersionList
	// reviewResults holds one response per GetBinaryReviewResults call; the
	// last entry is repeated once exhausted.
	reviewResults [][]accountApi.BinaryReviewResult

	uploadFileErr error

	reviewResultCalls   int
	createdBinary       *accountApi.ExtensionCreate
	updatedBinary       *accountApi.ExtensionUpdate
	uploadedZipPath     string
	codeReviewTriggered bool
}

var _ UploadAPI = (*fakeUploadAPI)(nil)

func (f *fakeUploadAPI) GetExtensionByName(ctx context.Context, name string) (*accountApi.Extension, error) {
	return f.extension, nil
}

func (f *fakeUploadAPI) GetExtensionBinaries(ctx context.Context, producerId, extensionId int) ([]*accountApi.ExtensionBinary, error) {
	return f.binaries, nil
}

func (f *fakeUploadAPI) CreateExtensionBinary(ctx context.Context, producerId, extensionId int, create accountApi.ExtensionCreate) (*accountApi.ExtensionBinary, error) {
	f.createdBinary = &create
	return &accountApi.ExtensionBinary{Id: 1000, Version: create.Version}, nil
}

func (f *fakeUploadAPI) UpdateExtensionBinaryInfo(ctx context.Context, producerId, extensionId int, update accountApi.ExtensionUpdate) error {
	f.updatedBinary = &update
	return nil
}

func (f *fakeUploadAPI) UpdateExtensionBinaryFile(ctx context.Context, producerId, extensionId, binaryId int, zipPath string) error {
	if f.uploadFileErr != nil {
		return f.uploadFileErr
	}
	f.uploadedZipPath = zipPath
	return nil
}

func (f *fakeUploadAPI) GetSoftwareVersions(ctx context.Context, generation string) (*accountApi.SoftwareVersionList, error) {
	return &f.softwareVersions, nil
}

func (f *fakeUploadAPI) TriggerCodeReview(ctx context.Context, extensionId int) error {
	f.codeReviewTriggered = true
	return nil
}

func (f *fakeUploadAPI) GetBinaryReviewResults(ctx context.Context, extensionId, binaryId int) ([]accountApi.BinaryReviewResult, error) {
	call := f.reviewResultCalls
	f.reviewResultCalls++

	if len(f.reviewResults) == 0 {
		return nil, nil
	}

	if call >= len(f.reviewResults) {
		call = len(f.reviewResults) - 1
	}

	return f.reviewResults[call], nil
}

// fakeListAPI serves a static extension list and records the used criteria.
type fakeListAPI struct {
	extensions []accountApi.Extension
	criteria   *accountApi.ListExtensionCriteria
}

var _ ListAPI = (*fakeListAPI)(nil)

func (f *fakeListAPI) Extensions(ctx context.Context, criteria *accountApi.ListExtensionCriteria) ([]accountApi.Extension, error) {
	f.criteria = criteria
	return f.extensions, nil
}

// fakeStoreInfoPushAPI records the extension update sent by PushStoreInfo.
type fakeStoreInfoPushAPI struct {
	extension   *accountApi.Extension
	generalInfo *accountApi.ExtensionGeneralInformation

	updatedExtension *accountApi.Extension
	updatedIconPath  string
}

var _ StoreInfoPushAPI = (*fakeStoreInfoPushAPI)(nil)

func (f *fakeStoreInfoPushAPI) GetExtensionByName(ctx context.Context, name string) (*accountApi.Extension, error) {
	return f.extension, nil
}

func (f *fakeStoreInfoPushAPI) GetExtensionGeneralInfo(ctx context.Context) (*accountApi.ExtensionGeneralInformation, error) {
	return f.generalInfo, nil
}

func (f *fakeStoreInfoPushAPI) UpdateExtensionIcon(ctx context.Context, extensionId int, iconFile string) error {
	f.updatedIconPath = iconFile
	return nil
}

func (f *fakeStoreInfoPushAPI) GetExtensionImages(ctx context.Context, extensionId int) ([]*accountApi.ExtensionImage, error) {
	return nil, nil
}

func (f *fakeStoreInfoPushAPI) DeleteExtensionImages(ctx context.Context, extensionId, imageId int) error {
	return nil
}

func (f *fakeStoreInfoPushAPI) AddExtensionImage(ctx context.Context, extensionId int, file string) (*accountApi.ExtensionImage, error) {
	return &accountApi.ExtensionImage{}, nil
}

func (f *fakeStoreInfoPushAPI) UpdateExtensionImage(ctx context.Context, extensionId int, image *accountApi.ExtensionImage) error {
	return nil
}

func (f *fakeStoreInfoPushAPI) UpdateExtension(ctx context.Context, extension *accountApi.Extension) error {
	f.updatedExtension = extension
	return nil
}
