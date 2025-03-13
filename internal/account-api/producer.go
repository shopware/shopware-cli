package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/schema"
)

type ProducerEndpoint struct {
	c          *Client
	producerId int
}

func (e ProducerEndpoint) GetId() int {
	return e.producerId
}

func (c *Client) Producer(ctx context.Context) (*ProducerEndpoint, error) {
	r, err := c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/companies/%d/allocations", ApiUrl, c.GetActiveCompanyID()), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(r)
	if err != nil {
		return nil, err
	}

	var allocation companyAllocation
	if err := json.Unmarshal(body, &allocation); err != nil {
		return nil, fmt.Errorf("producer.profile: %v", err)
	}

	if !allocation.IsProducer {
		return nil, fmt.Errorf("this company is not unlocked as producer")
	}

	return &ProducerEndpoint{producerId: allocation.ProducerID, c: c}, nil
}

type companyAllocation struct {
	HasShops          bool `json:"hasShops"`
	HasCommercialShop bool `json:"hasCommercialShop"`
	IsEducationMember bool `json:"isEducationMember"`
	IsPartner         bool `json:"isPartner"`
	IsProducer        bool `json:"isProducer"`
	ProducerID        int  `json:"producerId"`
}

func (e ProducerEndpoint) Profile(ctx context.Context) (*Producer, error) {
	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/producers?companyId=%d", ApiUrl, e.c.GetActiveCompanyID()), nil)
	if err != nil {
		return nil, err
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, err
	}

	var producers []Producer
	if err := json.Unmarshal(body, &producers); err != nil {
		return nil, fmt.Errorf("my_profile: %v", err)
	}

	for _, profile := range producers {
		return &profile, nil
	}

	return nil, fmt.Errorf("cannot find a profile")
}

type Producer struct {
	Id       int    `json:"id"`
	Prefix   string `json:"prefix"`
	Contract struct {
		Id   int    `json:"id"`
		Path string `json:"path"`
	} `json:"contract"`
	Name    string `json:"name"`
	Details []struct {
		Id     int `json:"id"`
		Locale struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"locale"`
		Description string `json:"description"`
	} `json:"details"`
	Website              string `json:"website"`
	Fixed                bool   `json:"fixed"`
	HasCancelledContract bool   `json:"hasCancelledContract"`
	IconPath             string `json:"iconPath"`
	IconIsSet            bool   `json:"iconIsSet"`
	ShopwareID           string `json:"shopwareId"`
	UserId               int    `json:"userId"`
	CompanyId            int    `json:"companyId"`
	CompanyName          string `json:"companyName"`
	SaleMail             string `json:"saleMail"`
	SupportMail          string `json:"supportMail"`
	RatingMail           string `json:"ratingMail"`
	SupportedLanguages   []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"supportedLanguages"`
	IconURL           string      `json:"iconUrl"`
	CancelledContract interface{} `json:"cancelledContract"`
}

type ListExtensionCriteria struct {
	Limit         int    `schema:"limit,omitempty"`
	Offset        int    `schema:"offset,omitempty"`
	OrderBy       string `schema:"orderBy,omitempty"`
	OrderSequence string `schema:"orderSequence,omitempty"`
	Search        string `schema:"search,omitempty"`
}

func (e ProducerEndpoint) Extensions(ctx context.Context, criteria *ListExtensionCriteria) ([]Extension, error) {
	encoder := schema.NewEncoder()
	form := url.Values{}
	form.Set("producerId", strconv.FormatInt(int64(e.GetId()), 10))
	err := encoder.Encode(criteria, form)
	if err != nil {
		return nil, fmt.Errorf("list_extensions: %v", err)
	}

	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/plugins?%s", ApiUrl, form.Encode()), nil)
	if err != nil {
		return nil, err
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, err
	}

	var extensions []Extension
	if err := json.Unmarshal(body, &extensions); err != nil {
		return nil, fmt.Errorf("list_extensions: %v", err)
	}

	return extensions, nil
}

func (e ProducerEndpoint) GetExtensionByName(ctx context.Context, name string) (*Extension, error) {
	criteria := ListExtensionCriteria{
		Search: name,
	}

	extensions, err := e.Extensions(ctx, &criteria)
	if err != nil {
		return nil, err
	}

	for _, ext := range extensions {
		if strings.EqualFold(ext.Name, name) {
			return e.GetExtensionById(ctx, ext.Id)
		}
	}

	return nil, fmt.Errorf("cannot find Extension by name %s", name)
}

func (e ProducerEndpoint) GetExtensionById(ctx context.Context, id int) (*Extension, error) {
	errorFormat := "GetExtensionById: %v"

	// Create it
	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/plugins/%d", ApiUrl, id), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var extension Extension
	if err := json.Unmarshal(body, &extension); err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return &extension, nil
}

type Extension struct {
	Id       int `json:"id"`
	Producer struct {
		Id       int    `json:"id"`
		Prefix   string `json:"prefix"`
		Contract struct {
			Id   int    `json:"id"`
			Path string `json:"path"`
		} `json:"contract"`
		Name    string `json:"name"`
		Details []struct {
			Id     int `json:"id"`
			Locale struct {
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"locale"`
			Description string `json:"description"`
		} `json:"details"`
		Website              string `json:"website"`
		Fixed                bool   `json:"fixed"`
		HasCancelledContract bool   `json:"hasCancelledContract"`
		IconPath             string `json:"iconPath"`
		IconIsSet            bool   `json:"iconIsSet"`
		ShopwareID           string `json:"shopwareId"`
		UserId               int    `json:"userId"`
		CompanyId            int    `json:"companyId"`
		CompanyName          string `json:"companyName"`
		SaleMail             string `json:"saleMail"`
		SupportMail          string `json:"supportMail"`
		RatingMail           string `json:"ratingMail"`
		SupportedLanguages   []struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"supportedLanguages"`
		IconURL           string      `json:"iconUrl"`
		CancelledContract interface{} `json:"cancelledContract"`
	} `json:"producer"`
	Type struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"type"`
	Name            string `json:"name"`
	Code            string `json:"code"`
	ModuleKey       string `json:"moduleKey"`
	LifecycleStatus struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"lifecycleStatus"`
	Generation struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"generation"`
	ActivationStatus struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"activationStatus"`
	ApprovalStatus struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"approvalStatus"`
	StandardLocale Locale `json:"standardLocale"`
	License        struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"license"`
	Infos []*struct {
		Id                 int          `json:"id"`
		Locale             Locale       `json:"locale"`
		Name               string       `json:"name"`
		Description        string       `json:"description"`
		InstallationManual string       `json:"installationManual"`
		ShortDescription   string       `json:"shortDescription"`
		Highlights         string       `json:"highlights"`
		Features           string       `json:"features"`
		MetaTitle          string       `json:"metaTitle"`
		MetaDescription    string       `json:"metaDescription"`
		Tags               []StoreTag   `json:"tags"`
		Videos             []StoreVideo `json:"videos"`
		Faqs               []StoreFaq   `json:"faqs"`
	} `json:"infos"`
	PriceModels                         []interface{}      `json:"priceModels"`
	Variants                            []interface{}      `json:"variants"`
	StoreAvailabilities                 []StoreAvailablity `json:"storeAvailabilities"`
	Categories                          []StoreCategory    `json:"categories"`
	Category                            *StoreCategory     `json:"selectedFutureCategory"`
	Addons                              []interface{}      `json:"addons"`
	LastChange                          string             `json:"lastChange"`
	CreationDate                        string             `json:"creationDate"`
	Support                             bool               `json:"support"`
	SupportOnlyCommercial               bool               `json:"supportOnlyCommercial"`
	IconPath                            string             `json:"iconPath"`
	IconIsSet                           bool               `json:"iconIsSet"`
	ExamplePageUrl                      string             `json:"examplePageUrl"`
	Demos                               []interface{}      `json:"demos"`
	Localizations                       []Locale           `json:"localizations"`
	LatestBinary                        interface{}        `json:"latestBinary"`
	MigrationSupport                    bool               `json:"migrationSupport"`
	AutomaticBugfixVersionCompatibility bool               `json:"automaticBugfixVersionCompatibility"`
	HiddenInStore                       bool               `json:"hiddenInStore"`
	Certification                       interface{}        `json:"certification"`
	ProductType                         *StoreProductType  `json:"productType"`
	Status                              struct {
		Name string `json:"name"`
	} `json:"status"`
	MinimumMarketingSoftwareVersion       interface{} `json:"minimumMarketingSoftwareVersion"`
	IsSubscriptionEnabled                 bool        `json:"isSubscriptionEnabled"`
	ReleaseDate                           interface{} `json:"releaseDate"`
	PlannedReleaseDate                    interface{} `json:"plannedReleaseDate"`
	LastBusinessModelChangeDate           interface{} `json:"lastBusinessModelChangeDate"`
	IsSW5Compatible                       bool        `json:"isSW5Compatible"`
	Subprocessors                         interface{} `json:"subprocessors"`
	PluginTestingInstanceDisabled         bool        `json:"pluginTestingInstanceDisabled"`
	IconURL                               string      `json:"iconUrl"`
	Pictures                              string      `json:"pictures"`
	HasPictures                           bool        `json:"hasPictures"`
	Comments                              string      `json:"comments"`
	Reviews                               string      `json:"reviews"`
	IsPremiumPlugin                       bool        `json:"isPremiumPlugin"`
	IsAdvancedFeature                     bool        `json:"isAdvancedFeature"`
	IsEnterpriseAccelerator               bool        `json:"isEnterpriseAccelerator"`
	IsSW6EnterpriseFeature                bool        `json:"isSW6EnterpriseFeature"`
	IsSW6ProfessionalEditionFeature       bool        `json:"isSW6ProfessionalEditionFeature"`
	Binaries                              interface{} `json:"binaries"`
	Predecessor                           interface{} `json:"predecessor"`
	Successor                             interface{} `json:"successor"`
	IsCompatibleWithLatestShopwareVersion bool        `json:"isCompatibleWithLatestShopwareVersion"`
	PluginPreview                         interface{} `json:"pluginPreview"`
	IsNoLongerAvailableForDownload        bool        `json:"isNoLongerAvailableForDownload"`
}

type CreateExtensionRequest struct {
	Name       string `json:"name,omitempty"`
	Generation struct {
		Name string `json:"name"`
	} `json:"generation"`
	ProducerID int `json:"producerId"`
}

func (e ProducerEndpoint) UpdateExtension(ctx context.Context, extension *Extension) error {
	requestBody, err := json.Marshal(extension)
	if err != nil {
		return err
	}

	// Patch the name
	r, err := e.c.NewAuthenticatedRequest(ctx, "PUT", fmt.Sprintf("%s/plugins/%d", ApiUrl, extension.Id), bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}

	_, err = e.c.doRequest(r)

	return err
}

func (e ProducerEndpoint) GetSoftwareVersions(ctx context.Context, generation string) (*SoftwareVersionList, error) {
	errorFormat := "shopware_versions: %v"
	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/pluginstatics/softwareVersions?filter=[{\"property\":\"pluginGeneration\",\"value\":\"%s\"},{\"property\":\"includeNonPublic\",\"value\":\"1\"}]", ApiUrl, generation), nil)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	var versions SoftwareVersionList

	err = json.Unmarshal(body, &versions)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return &versions, nil
}

type SoftwareVersion struct {
	Id          int         `json:"id"`
	Name        string      `json:"name"`
	Parent      interface{} `json:"parent"`
	Selectable  bool        `json:"selectable"`
	Major       *string     `json:"major"`
	ReleaseDate *string     `json:"releaseDate"`
	Public      bool        `json:"public"`
}

type Locale struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type StoreAvailablity struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type StoreCategory struct {
	Id          int         `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parent      interface{} `json:"parent"`
	Position    int         `json:"position"`
	Public      bool        `json:"public"`
	Visible     bool        `json:"visible"`
	Suggested   bool        `json:"suggested"`
	Applicable  bool        `json:"applicable"`
	Details     interface{} `json:"details"`
	Active      bool        `json:"active"`
}

type StoreTag struct {
	Name string `json:"name"`
}

type StoreVideo struct {
	URL string `json:"url"`
}

type StoreProductType struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type StoreFaq struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type ExtensionGeneralInformation struct {
	Categories       []StoreCategory `json:"categories"`
	FutureCategories []StoreCategory `json:"futureCategories"`
	Addons           interface{}     `json:"addons"`
	Generations      []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"generations"`
	ActivationStatus []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"activationStatus"`
	ApprovalStatus []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"approvalStatus"`
	LifecycleStatus []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"lifecycleStatus"`
	BinaryStatus []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"binaryStatus"`
	Locales  []Locale `json:"locales"`
	Licenses []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"licenses"`
	StoreAvailabilities  []StoreAvailablity  `json:"storeAvailabilities"`
	PriceModels          []interface{}       `json:"priceModels"`
	SoftwareVersions     SoftwareVersionList `json:"softwareVersions"`
	DemoTypes            interface{}         `json:"demoTypes"`
	Localizations        []Locale            `json:"localizations"`
	ProductTypes         []StoreProductType  `json:"productTypes"`
	ReleaseRequestStatus interface{}         `json:"releaseRequestStatus"`
}

func (e ProducerEndpoint) GetExtensionGeneralInfo(ctx context.Context) (*ExtensionGeneralInformation, error) {
	r, err := e.c.NewAuthenticatedRequest(ctx, "GET", fmt.Sprintf("%s/pluginstatics/all", ApiUrl), nil)
	if err != nil {
		return nil, fmt.Errorf("GetExtensionGeneralInfo: %v", err)
	}

	body, err := e.c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf("GetExtensionGeneralInfo: %v", err)
	}

	var info *ExtensionGeneralInformation

	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("shopware_versions: %v", err)
	}

	return info, nil
}
