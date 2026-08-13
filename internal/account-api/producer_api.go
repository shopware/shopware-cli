package account_api

import (
	"context"
)

// ProducerAPI describes the producer operations of the Shopware Account API. It is implemented
// by *ProducerEndpoint and exists so that the store synchronization logic can be tested without
// hitting the real API.
type ProducerAPI interface {
	Extensions(ctx context.Context, criteria *ListExtensionCriteria) ([]Extension, error)
	GetExtensionByName(ctx context.Context, name string) (*Extension, error)
	UpdateExtension(ctx context.Context, extension *Extension) error
	GetExtensionGeneralInfo(ctx context.Context) (*ExtensionGeneralInformation, error)
	UpdateExtensionIcon(ctx context.Context, extensionId int, iconFilePath string) error
	GetExtensionImages(ctx context.Context, extensionId int) ([]*ExtensionImage, error)
	DeleteExtensionImages(ctx context.Context, extensionId, imageId int) error
	AddExtensionImage(ctx context.Context, extensionId int, file string) (*ExtensionImage, error)
	UpdateExtensionImage(ctx context.Context, extensionId int, image *ExtensionImage) error
	GetExtensionBinaries(ctx context.Context, producerId, extensionId int) ([]*ExtensionBinary, error)
	CreateExtensionBinary(ctx context.Context, producerId, extensionId int, create ExtensionCreate) (*ExtensionBinary, error)
	UpdateExtensionBinaryInfo(ctx context.Context, producerId, extensionId int, update ExtensionUpdate) error
	UpdateExtensionBinaryFile(ctx context.Context, producerId, extensionId, binaryId int, zipPath string) error
	GetSoftwareVersions(ctx context.Context, generation string) (*SoftwareVersionList, error)
	TriggerCodeReview(ctx context.Context, extensionId int) error
	GetBinaryReviewResults(ctx context.Context, extensionId, binaryId int) ([]BinaryReviewResult, error)
}

var _ ProducerAPI = (*ProducerEndpoint)(nil)
