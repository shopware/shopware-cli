package account_api

import "context"

// fakeProducer is a test double implementing ProducerAPI. Each method delegates to a
// configurable function field so tests only stub what they exercise.
type fakeProducer struct {
	extensionsFn                func(ctx context.Context, criteria *ListExtensionCriteria) ([]Extension, error)
	getExtensionByNameFn        func(ctx context.Context, name string) (*Extension, error)
	updateExtensionFn           func(ctx context.Context, extension *Extension) error
	getExtensionGeneralInfoFn   func(ctx context.Context) (*ExtensionGeneralInformation, error)
	updateExtensionIconFn       func(ctx context.Context, extensionId int, iconFilePath string) error
	getExtensionImagesFn        func(ctx context.Context, extensionId int) ([]*ExtensionImage, error)
	deleteExtensionImagesFn     func(ctx context.Context, extensionId, imageId int) error
	addExtensionImageFn         func(ctx context.Context, extensionId int, file string) (*ExtensionImage, error)
	updateExtensionImageFn      func(ctx context.Context, extensionId int, image *ExtensionImage) error
	getExtensionBinariesFn      func(ctx context.Context, producerId, extensionId int) ([]*ExtensionBinary, error)
	createExtensionBinaryFn     func(ctx context.Context, producerId, extensionId int, create ExtensionCreate) (*ExtensionBinary, error)
	updateExtensionBinaryInfoFn func(ctx context.Context, producerId, extensionId int, update ExtensionUpdate) error
	updateExtensionBinaryFileFn func(ctx context.Context, producerId, extensionId, binaryId int, zipPath string) error
	getSoftwareVersionsFn       func(ctx context.Context, generation string) (*SoftwareVersionList, error)
	triggerCodeReviewFn         func(ctx context.Context, extensionId int) error
	getBinaryReviewResultsFn    func(ctx context.Context, extensionId, binaryId int) ([]BinaryReviewResult, error)
}

var _ ProducerAPI = (*fakeProducer)(nil)

func (f *fakeProducer) Extensions(ctx context.Context, criteria *ListExtensionCriteria) ([]Extension, error) {
	if f.extensionsFn != nil {
		return f.extensionsFn(ctx, criteria)
	}
	return nil, nil
}

func (f *fakeProducer) GetExtensionByName(ctx context.Context, name string) (*Extension, error) {
	if f.getExtensionByNameFn != nil {
		return f.getExtensionByNameFn(ctx, name)
	}
	return &Extension{}, nil
}

func (f *fakeProducer) UpdateExtension(ctx context.Context, extension *Extension) error {
	if f.updateExtensionFn != nil {
		return f.updateExtensionFn(ctx, extension)
	}
	return nil
}

func (f *fakeProducer) GetExtensionGeneralInfo(ctx context.Context) (*ExtensionGeneralInformation, error) {
	if f.getExtensionGeneralInfoFn != nil {
		return f.getExtensionGeneralInfoFn(ctx)
	}
	return &ExtensionGeneralInformation{}, nil
}

func (f *fakeProducer) UpdateExtensionIcon(ctx context.Context, extensionId int, iconFilePath string) error {
	if f.updateExtensionIconFn != nil {
		return f.updateExtensionIconFn(ctx, extensionId, iconFilePath)
	}
	return nil
}

func (f *fakeProducer) GetExtensionImages(ctx context.Context, extensionId int) ([]*ExtensionImage, error) {
	if f.getExtensionImagesFn != nil {
		return f.getExtensionImagesFn(ctx, extensionId)
	}
	return nil, nil
}

func (f *fakeProducer) DeleteExtensionImages(ctx context.Context, extensionId, imageId int) error {
	if f.deleteExtensionImagesFn != nil {
		return f.deleteExtensionImagesFn(ctx, extensionId, imageId)
	}
	return nil
}

func (f *fakeProducer) AddExtensionImage(ctx context.Context, extensionId int, file string) (*ExtensionImage, error) {
	if f.addExtensionImageFn != nil {
		return f.addExtensionImageFn(ctx, extensionId, file)
	}
	return &ExtensionImage{}, nil
}

func (f *fakeProducer) UpdateExtensionImage(ctx context.Context, extensionId int, image *ExtensionImage) error {
	if f.updateExtensionImageFn != nil {
		return f.updateExtensionImageFn(ctx, extensionId, image)
	}
	return nil
}

func (f *fakeProducer) GetExtensionBinaries(ctx context.Context, producerId, extensionId int) ([]*ExtensionBinary, error) {
	if f.getExtensionBinariesFn != nil {
		return f.getExtensionBinariesFn(ctx, producerId, extensionId)
	}
	return nil, nil
}

func (f *fakeProducer) CreateExtensionBinary(ctx context.Context, producerId, extensionId int, create ExtensionCreate) (*ExtensionBinary, error) {
	if f.createExtensionBinaryFn != nil {
		return f.createExtensionBinaryFn(ctx, producerId, extensionId, create)
	}
	return &ExtensionBinary{}, nil
}

func (f *fakeProducer) UpdateExtensionBinaryInfo(ctx context.Context, producerId, extensionId int, update ExtensionUpdate) error {
	if f.updateExtensionBinaryInfoFn != nil {
		return f.updateExtensionBinaryInfoFn(ctx, producerId, extensionId, update)
	}
	return nil
}

func (f *fakeProducer) UpdateExtensionBinaryFile(ctx context.Context, producerId, extensionId, binaryId int, zipPath string) error {
	if f.updateExtensionBinaryFileFn != nil {
		return f.updateExtensionBinaryFileFn(ctx, producerId, extensionId, binaryId, zipPath)
	}
	return nil
}

func (f *fakeProducer) GetSoftwareVersions(ctx context.Context, generation string) (*SoftwareVersionList, error) {
	if f.getSoftwareVersionsFn != nil {
		return f.getSoftwareVersionsFn(ctx, generation)
	}
	return &SoftwareVersionList{}, nil
}

func (f *fakeProducer) TriggerCodeReview(ctx context.Context, extensionId int) error {
	if f.triggerCodeReviewFn != nil {
		return f.triggerCodeReviewFn(ctx, extensionId)
	}
	return nil
}

func (f *fakeProducer) GetBinaryReviewResults(ctx context.Context, extensionId, binaryId int) ([]BinaryReviewResult, error) {
	if f.getBinaryReviewResultsFn != nil {
		return f.getBinaryReviewResultsFn(ctx, extensionId, binaryId)
	}
	return nil, nil
}
