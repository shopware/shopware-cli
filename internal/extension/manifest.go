package extension

import (
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type Manifest struct {
	XMLName              xml.Name      `xml:"manifest"`
	Meta                 Meta          `xml:"meta"`
	Setup                *Setup        `xml:"setup,omitempty"`
	Admin                *Admin        `xml:"admin,omitempty"`
	Permissions          *Permissions  `xml:"permissions,omitempty"`
	Webhooks             *Webhooks     `xml:"webhooks,omitempty"`
	Payments             *Payments     `xml:"payments,omitempty"`
	Tax                  *Tax          `xml:"tax,omitempty"`
	Gateways             *Gateways     `xml:"gateways,omitempty"`
	Requirements         *Requirements `xml:"requirements,omitempty"`
	ValidatesPermissions *bool         `xml:"validates-permissions,attr,omitempty"`
	RemainingXML         []xml.Token   `xml:"-"`
}

func (m *Manifest) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	m.XMLName = start.Name
	for _, attr := range start.Attr {
		if attr.Name.Local == "validates-permissions" {
			v := attr.Value == "true"
			m.ValidatesPermissions = &v
		}
	}

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if err := m.decodeChildElement(d, &t); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		default:
			if !isWhitespaceToken(tok) {
				m.RemainingXML = append(m.RemainingXML, cloneToken(t))
			}
		}
	}

	return nil
}

func (m *Manifest) decodeChildElement(d *xml.Decoder, t *xml.StartElement) error {
	switch t.Name.Local {
	case "meta":
		return d.DecodeElement(&m.Meta, t)
	case "setup":
		m.Setup = &Setup{}
		return d.DecodeElement(m.Setup, t)
	case "admin":
		m.Admin = &Admin{}
		return d.DecodeElement(m.Admin, t)
	case "permissions":
		m.Permissions = &Permissions{}
		return d.DecodeElement(m.Permissions, t)
	case "webhooks":
		m.Webhooks = &Webhooks{}
		return d.DecodeElement(m.Webhooks, t)
	case "payments":
		m.Payments = &Payments{}
		return d.DecodeElement(m.Payments, t)
	case "tax":
		m.Tax = &Tax{}
		return d.DecodeElement(m.Tax, t)
	case "gateways":
		m.Gateways = &Gateways{}
		return d.DecodeElement(m.Gateways, t)
	case "requirements":
		m.Requirements = &Requirements{}
		return d.DecodeElement(m.Requirements, t)
	default:
		m.RemainingXML = append(m.RemainingXML, *t)
		inner, err := readElementTokens(d)
		if err != nil {
			return err
		}
		m.RemainingXML = append(m.RemainingXML, inner...)
		return nil
	}
}

func (m *Manifest) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "manifest"}
	if m.ValidatesPermissions != nil {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "validates-permissions"},
			Value: fmt.Sprintf("%t", *m.ValidatesPermissions),
		})
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	writeField := func(name string, v any) error {
		if isZero(v) {
			return nil
		}
		innerStart := xml.StartElement{Name: xml.Name{Local: name}}
		return e.EncodeElement(v, innerStart)
	}

	if err := writeField("meta", m.Meta); err != nil {
		return err
	}
	if err := writeField("setup", m.Setup); err != nil {
		return err
	}
	if err := writeField("admin", m.Admin); err != nil {
		return err
	}
	if err := writeField("permissions", m.Permissions); err != nil {
		return err
	}
	if err := writeField("webhooks", m.Webhooks); err != nil {
		return err
	}
	if err := writeField("payments", m.Payments); err != nil {
		return err
	}
	if err := writeField("tax", m.Tax); err != nil {
		return err
	}
	if err := writeField("gateways", m.Gateways); err != nil {
		return err
	}
	if err := writeField("requirements", m.Requirements); err != nil {
		return err
	}

	for _, tok := range m.RemainingXML {
		if err := e.EncodeToken(tok); err != nil {
			return err
		}
	}

	return e.EncodeToken(xml.EndElement{Name: xml.Name{Local: "manifest"}})
}

func readElementTokens(d *xml.Decoder) ([]xml.Token, error) {
	depth := 1
	var tokens []xml.Token
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, cloneToken(tok))
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return tokens, nil
}

func cloneToken(tok xml.Token) xml.Token {
	switch t := tok.(type) {
	case xml.CharData:
		cp := make([]byte, len(t))
		copy(cp, t)
		return xml.CharData(cp)
	case xml.Comment:
		cp := make([]byte, len(t))
		copy(cp, t)
		return xml.Comment(cp)
	case xml.ProcInst:
		return xml.ProcInst{Target: t.Target, Inst: append([]byte(nil), t.Inst...)}
	default:
		return tok
	}
}

func isWhitespaceToken(tok xml.Token) bool {
	if cd, ok := tok.(xml.CharData); ok {
		return len(strings.TrimSpace(string(cd))) == 0
	}
	return false
}

func isZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return rv.IsNil()
	}
	return reflect.DeepEqual(v, reflect.Zero(rv.Type()).Interface())
}

type Meta struct {
	Name                    string             `xml:"name"`
	Label                   TranslatableString `xml:"label"`
	Description             TranslatableString `xml:"description,omitempty"`
	Author                  string             `xml:"author,omitempty"`
	Copyright               string             `xml:"copyright,omitempty"`
	Version                 string             `xml:"version"`
	Icon                    string             `xml:"icon,omitempty"`
	License                 string             `xml:"license"`
	Compatibility           string             `xml:"compatibility,omitempty"`
	Privacy                 string             `xml:"privacy,omitempty"`
	PrivacyPolicyExtensions TranslatableString `xml:"privacyPolicyExtensions,omitempty"`
}

type Setup struct {
	RegistrationUrl string `xml:"registrationUrl"`
	Secret          string `xml:"secret,omitempty"`
}

type Admin struct {
	ActionButton []ActionButton `xml:"action-button,omitempty"`
	Module       []Module       `xml:"module,omitempty"`
	MainModule   *MainModule    `xml:"main-module,omitempty"`
	BaseAppUrl   string         `xml:"base-app-url,omitempty"`
}

type Storefront struct {
	TemplateLoadPriority int `xml:"template-load-priority,omitempty"`
}

type Permissions struct {
	Read       []string `xml:"read,omitempty"`
	Create     []string `xml:"create,omitempty"`
	Update     []string `xml:"update,omitempty"`
	Delete     []string `xml:"delete,omitempty"`
	Crud       []string `xml:"crud,omitempty"`
	Permission []string `xml:"permission,omitempty"`
}

type AllowedHosts struct {
	Host []string `xml:"host"`
}

type CustomFields struct {
	CustomFieldSet []CustomFieldSet `xml:"custom-field-set,omitempty"`
}

type Webhooks struct {
	Webhook []Webhook `xml:"webhook,omitempty"`
}

type Cookies struct {
	Cookie []Cookie      `xml:"cookie,omitempty"`
	Group  []CookieGroup `xml:"group,omitempty"`
}

type Payments struct {
	PaymentMethod []PaymentMethod `xml:"payment-method,omitempty"`
}

type ShippingMethods struct {
	ShippingMethod []ShippingMethod `xml:"shipping-method,omitempty"`
}

type RuleConditions struct {
	RuleCondition []RuleCondition `xml:"rule-condition,omitempty"`
}

type Tax struct {
	TaxProvider []TaxProvider `xml:"tax-provider,omitempty"`
}

type Gateways struct {
	Checkout       string `xml:"checkout,omitempty"`
	Context        string `xml:"context,omitempty"`
	InAppPurchases string `xml:"inAppPurchases,omitempty"`
}

type TranslatableString []struct {
	Value string `xml:",chardata"`
	Lang  string `xml:"lang,attr,omitempty"`
}

func (t TranslatableString) GetValueByLanguage(lang []string) string {
	for _, v := range t {
		for _, l := range lang {
			if v.Lang == l {
				return v.Value
			}
		}
	}

	return ""
}

type ActionButton struct {
	Label  TranslatableString `xml:"label"`
	Action string             `xml:"action,attr"`
	Entity string             `xml:"entity,attr"`
	View   string             `xml:"view,attr"`
	URL    string             `xml:"url,attr"`
}

type Module struct {
	Label    TranslatableString `xml:"label"`
	Source   string             `xml:"source,attr,omitempty"`
	Name     string             `xml:"name,attr"`
	Parent   string             `xml:"parent,attr"`
	Position int                `xml:"position,attr,omitempty"`
}

type MainModule struct {
	Source string `xml:"source,attr"`
}

type CustomFieldSet struct {
	Name            string             `xml:"name"`
	Label           TranslatableString `xml:"label"`
	RelatedEntities EntityList         `xml:"related-entities"`
	Fields          CustomFieldList    `xml:"fields"`
	Global          bool               `xml:"global,attr,omitempty"`
}

type EntityList struct {
	Product             *struct{} `xml:"product,omitempty"`
	Order               *struct{} `xml:"order,omitempty"`
	Category            *struct{} `xml:"category,omitempty"`
	Customer            *struct{} `xml:"customer,omitempty"`
	CustomerAddress     *struct{} `xml:"customer_address,omitempty"`
	Media               *struct{} `xml:"media,omitempty"`
	ProductManufacturer *struct{} `xml:"product_manufacturer,omitempty"`
	SalesChannel        *struct{} `xml:"sales_channel,omitempty"`
	LandingPage         *struct{} `xml:"landing_page,omitempty"`
	Promotion           *struct{} `xml:"promotion,omitempty"`
	ProductStream       *struct{} `xml:"product_stream,omitempty"`
	PropertyGroup       *struct{} `xml:"property_group,omitempty"`
	PropertyGroupOption *struct{} `xml:"property_group_option,omitempty"`
	ProductReview       *struct{} `xml:"product_review,omitempty"`
	EventAction         *struct{} `xml:"event_action,omitempty"`
	Country             *struct{} `xml:"country,omitempty"`
	Currency            *struct{} `xml:"currency,omitempty"`
	CustomerGroup       *struct{} `xml:"customer_group,omitempty"`
	DeliveryTime        *struct{} `xml:"delivery_time,omitempty"`
	DocumentBaseConfig  *struct{} `xml:"document_base_config,omitempty"`
	Language            *struct{} `xml:"language,omitempty"`
	NumberRange         *struct{} `xml:"number_range,omitempty"`
	PaymentMethod       *struct{} `xml:"payment_method,omitempty"`
	Rule                *struct{} `xml:"rule,omitempty"`
	Salutation          *struct{} `xml:"salutation,omitempty"`
	ShippingMethod      *struct{} `xml:"shipping_method,omitempty"`
	Tax                 *struct{} `xml:"tax,omitempty"`
	Unit                *struct{} `xml:"unit,omitempty"`
	NewsletterRecipient *struct{} `xml:"newsletter_recipient,omitempty"`
}

type CustomFieldList struct {
	Int                []CustomFieldInt                `xml:"int,omitempty"`
	Float              []CustomFieldFloat              `xml:"float,omitempty"`
	Text               []CustomFieldText               `xml:"text,omitempty"`
	TextArea           []CustomFieldTextArea           `xml:"text-area,omitempty"`
	Bool               []CustomFieldBool               `xml:"bool,omitempty"`
	Datetime           []CustomFieldDatetime           `xml:"datetime,omitempty"`
	SingleSelect       []CustomFieldSingleSelect       `xml:"single-select,omitempty"`
	MultiSelect        []CustomFieldMultiSelect        `xml:"multi-select,omitempty"`
	SingleEntitySelect []CustomFieldSingleEntitySelect `xml:"single-entity-select,omitempty"`
	MultiEntitySelect  []CustomFieldMultiEntitySelect  `xml:"multi-entity-select,omitempty"`
	ColorPicker        []CustomFieldColorPicker        `xml:"color-picker,omitempty"`
	MediaSelection     []CustomFieldMedia              `xml:"media-selection,omitempty"`
	Price              []CustomFieldPrice              `xml:"price,omitempty"`
}

type CustomFieldBase struct {
	Label              TranslatableString `xml:"label"`
	HelpText           TranslatableString `xml:"help-text,omitempty"`
	Required           bool               `xml:"required,omitempty"`
	Position           int                `xml:"position,omitempty"`
	AllowCustomerWrite bool               `xml:"allow-customer-write,omitempty"`
	AllowCartExpose    bool               `xml:"allow-cart-expose,omitempty"`
}

type CustomFieldInt struct {
	CustomFieldBase
	XMLName     xml.Name           `xml:"int"`
	Name        string             `xml:"name,attr"`
	Placeholder TranslatableString `xml:"placeholder,omitempty"`
	Steps       int                `xml:"steps,omitempty"`
	Min         int                `xml:"min,omitempty"`
	Max         int                `xml:"max,omitempty"`
}

type CustomFieldFloat struct {
	CustomFieldBase
	XMLName     xml.Name           `xml:"float"`
	Name        string             `xml:"name,attr"`
	Placeholder TranslatableString `xml:"placeholder,omitempty"`
	Steps       float64            `xml:"steps,omitempty"`
	Min         float64            `xml:"min,omitempty"`
	Max         float64            `xml:"max,omitempty"`
}

type CustomFieldText struct {
	CustomFieldBase
	XMLName     xml.Name           `xml:"text"`
	Name        string             `xml:"name,attr"`
	Placeholder TranslatableString `xml:"placeholder,omitempty"`
}

type CustomFieldTextArea struct {
	CustomFieldBase
	XMLName     xml.Name           `xml:"text-area"`
	Name        string             `xml:"name,attr"`
	Placeholder TranslatableString `xml:"placeholder,omitempty"`
}

type CustomFieldBool struct {
	CustomFieldBase
	XMLName xml.Name `xml:"bool"`
	Name    string   `xml:"name,attr"`
}

type CustomFieldDatetime struct {
	CustomFieldBase
	XMLName xml.Name `xml:"datetime"`
	Name    string   `xml:"name,attr"`
}

type CustomFieldSingleSelect struct {
	CustomFieldBase
	XMLName     xml.Name           `xml:"single-select"`
	Name        string             `xml:"name,attr"`
	Placeholder TranslatableString `xml:"placeholder,omitempty"`
	Options     OptionCollection   `xml:"options"`
}

type CustomFieldMultiSelect struct {
	CustomFieldSingleSelect
	XMLName xml.Name `xml:"multi-select"`
}

type CustomFieldSingleEntitySelect struct {
	CustomFieldBase
	XMLName       xml.Name           `xml:"single-entity-select"`
	Name          string             `xml:"name,attr"`
	Placeholder   TranslatableString `xml:"placeholder,omitempty"`
	Entity        string             `xml:"entity"`
	LabelProperty string             `xml:"label-property"`
}

type CustomFieldMultiEntitySelect struct {
	CustomFieldSingleEntitySelect
	XMLName xml.Name `xml:"multi-entity-select"`
}

type CustomFieldColorPicker struct {
	CustomFieldBase
	XMLName xml.Name `xml:"color-picker"`
	Name    string   `xml:"name,attr"`
}

type CustomFieldMedia struct {
	CustomFieldBase
	XMLName xml.Name `xml:"media-selection"`
	Name    string   `xml:"name,attr"`
}

type CustomFieldPrice struct {
	CustomFieldBase
	XMLName xml.Name `xml:"price"`
	Name    string   `xml:"name,attr"`
}

type OptionCollection struct {
	Option []Option `xml:"option"`
}

type Option struct {
	Name  TranslatableString `xml:"name"`
	Value string             `xml:"value,attr"`
}

// Add more specific custom field types here...

type Webhook struct {
	Name            string `xml:"name,attr"`
	URL             string `xml:"url,attr"`
	Event           string `xml:"event,attr"`
	OnlyLiveVersion bool   `xml:"onlyLiveVersion,attr,omitempty"`
}

type Cookie struct {
	SnippetName        string `xml:"snippet-name"`
	SnippetDescription string `xml:"snippet-description,omitempty"`
	Cookie             string `xml:"cookie"`
	Value              string `xml:"value,omitempty"`
	Expiration         int    `xml:"expiration,omitempty"`
}

type CookieGroup struct {
	SnippetName        string        `xml:"snippet-name"`
	SnippetDescription string        `xml:"snippet-description,omitempty"`
	Entries            []CookieEntry `xml:"entries>cookie,omitempty"`
}

type CookieEntry struct {
	Cookie
}

type PaymentMethod struct {
	Identifier   string             `xml:"identifier"`
	Name         TranslatableString `xml:"name"`
	Description  TranslatableString `xml:"description,omitempty"`
	PayURL       string             `xml:"pay-url,omitempty"`
	FinalizeURL  string             `xml:"finalize-url,omitempty"`
	ValidateURL  string             `xml:"validate-url,omitempty"`
	CaptureURL   string             `xml:"capture-url,omitempty"`
	RefundURL    string             `xml:"refund-url,omitempty"`
	RecurringURL string             `xml:"recurring-url,omitempty"`
	Icon         string             `xml:"icon,omitempty"`
}

type ShippingMethod struct {
	Identifier   string             `xml:"identifier"`
	Name         TranslatableString `xml:"name"`
	Description  TranslatableString `xml:"description,omitempty"`
	Active       bool               `xml:"active,omitempty"`
	DeliveryTime DeliveryTime       `xml:"delivery-time"`
	Icon         string             `xml:"icon,omitempty"`
	Position     int                `xml:"position,omitempty"`
	TrackingURL  TranslatableString `xml:"tracking-url,omitempty"`
}

type DeliveryTime struct {
	ID   string             `xml:"id"`
	Name TranslatableString `xml:"name"`
	Min  int                `xml:"min"`
	Max  int                `xml:"max"`
	Unit string             `xml:"unit"`
}

type RuleCondition struct {
	Identifier  string             `xml:"identifier"`
	Name        TranslatableString `xml:"name"`
	Group       string             `xml:"group"`
	Script      string             `xml:"script"`
	Constraints []CustomFieldList  `xml:"constraints"`
}

type TaxProvider struct {
	Identifier string `xml:"identifier"`
	Name       string `xml:"name"`
	Priority   int    `xml:"priority"`
	ProcessURL string `xml:"process-url"`
}

type Requirements struct {
	InnerXML string `xml:",innerxml"`
}
