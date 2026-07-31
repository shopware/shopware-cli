// Package producer contains the business logic behind the
// "account producer" commands. The command layer in cmd/account only parses
// arguments and flags and delegates to this package.
//
// All account API access goes through small per-use-case interfaces, so the
// logic can be tested with fakes instead of a live account client.
package producer

import (
	"context"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
)

// ListAPI is the account API surface needed to list producer extensions.
type ListAPI interface {
	Extensions(ctx context.Context, criteria *accountApi.ListExtensionCriteria) ([]accountApi.Extension, error)
}

// StoreInfoPushAPI is the account API surface needed to push local store
// configuration to the account.
type StoreInfoPushAPI interface {
	GetExtensionByName(ctx context.Context, name string) (*accountApi.Extension, error)
	GetExtensionGeneralInfo(ctx context.Context) (*accountApi.ExtensionGeneralInformation, error)
	UpdateExtensionIcon(ctx context.Context, extensionId int, iconFile string) error
	GetExtensionImages(ctx context.Context, extensionId int) ([]*accountApi.ExtensionImage, error)
	DeleteExtensionImages(ctx context.Context, extensionId, imageId int) error
	AddExtensionImage(ctx context.Context, extensionId int, file string) (*accountApi.ExtensionImage, error)
	UpdateExtensionImage(ctx context.Context, extensionId int, image *accountApi.ExtensionImage) error
	UpdateExtension(ctx context.Context, extension *accountApi.Extension) error
}

// StoreInfoPullAPI is the account API surface needed to generate the local
// store configuration from account data.
type StoreInfoPullAPI interface {
	GetExtensionByName(ctx context.Context, name string) (*accountApi.Extension, error)
	GetExtensionImages(ctx context.Context, extensionId int) ([]*accountApi.ExtensionImage, error)
}

// UploadAPI is the account API surface needed to upload a new extension
// binary and follow its code review.
type UploadAPI interface {
	GetExtensionByName(ctx context.Context, name string) (*accountApi.Extension, error)
	GetExtensionBinaries(ctx context.Context, producerId, extensionId int) ([]*accountApi.ExtensionBinary, error)
	CreateExtensionBinary(ctx context.Context, producerId, extensionId int, create accountApi.ExtensionCreate) (*accountApi.ExtensionBinary, error)
	UpdateExtensionBinaryInfo(ctx context.Context, producerId, extensionId int, update accountApi.ExtensionUpdate) error
	UpdateExtensionBinaryFile(ctx context.Context, producerId, extensionId, binaryId int, zipPath string) error
	GetSoftwareVersions(ctx context.Context, generation string) (*accountApi.SoftwareVersionList, error)
	TriggerCodeReview(ctx context.Context, extensionId int) error
	GetBinaryReviewResults(ctx context.Context, extensionId, binaryId int) ([]accountApi.BinaryReviewResult, error)
}

var (
	_ ListAPI          = (*accountApi.ProducerEndpoint)(nil)
	_ StoreInfoPushAPI = (*accountApi.ProducerEndpoint)(nil)
	_ StoreInfoPullAPI = (*accountApi.ProducerEndpoint)(nil)
	_ UploadAPI        = (*accountApi.ProducerEndpoint)(nil)
)
