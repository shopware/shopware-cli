package symfony

import (
	"encoding/xml"
	"fmt"
)

// Container is the parsed representation of a Symfony services.xml file.
type Container struct {
	XMLName    xml.Name       `xml:"container"`
	Imports    *xmlImports    `xml:"imports"`
	Parameters *xmlParameters `xml:"parameters"`
	Services   *xmlServices   `xml:"services"`
	When       []xmlWhen      `xml:"when"`
	Unknown    []xmlUnknown   `xml:",any"`
}

// xmlUnknown captures elements the converter does not understand, so the
// conversion can be refused instead of silently dropping configuration.
type xmlUnknown struct {
	XMLName xml.Name
}

type xmlWhen struct {
	Env        string         `xml:"env,attr"`
	Imports    *xmlImports    `xml:"imports"`
	Parameters *xmlParameters `xml:"parameters"`
	Services   *xmlServices   `xml:"services"`
	Unknown    []xmlUnknown   `xml:",any"`
}

type xmlImports struct {
	Imports []xmlImport  `xml:"import"`
	Unknown []xmlUnknown `xml:",any"`
}

type xmlImport struct {
	Resource     string     `xml:"resource,attr"`
	Type         string     `xml:"type,attr"`
	IgnoreErrors string     `xml:"ignore-errors,attr"`
	UnknownAttrs []xml.Attr `xml:",any,attr"`
}

type xmlParameters struct {
	Parameters []xmlParameter `xml:"parameter"`
	Unknown    []xmlUnknown   `xml:",any"`
}

type xmlParameter struct {
	Key          string         `xml:"key,attr"`
	Type         string         `xml:"type,attr"`
	Value        string         `xml:",chardata"`
	Children     []xmlParameter `xml:"parameter"`
	UnknownAttrs []xml.Attr     `xml:",any,attr"`
	Unknown      []xmlUnknown   `xml:",any"`
}

type xmlServices struct {
	Defaults []xmlDefaults    `xml:"defaults"`
	Items    []xmlServiceItem `xml:",any"`
}

// xmlServiceItem keeps service, prototype and instanceof elements in document
// order, as the order of definitions is meaningful for overrides.
type xmlServiceItem struct {
	Kind    string
	Service *xmlService
}

func (i *xmlServiceItem) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	i.Kind = start.Name.Local

	switch start.Name.Local {
	case "service", "prototype", "instanceof":
		i.Service = &xmlService{}
		return d.DecodeElement(i.Service, &start)
	default:
		return d.Skip()
	}
}

type xmlDefaults struct {
	Public        string        `xml:"public,attr"`
	Autowire      string        `xml:"autowire,attr"`
	Autoconfigure string        `xml:"autoconfigure,attr"`
	Binds         []xmlArgument `xml:"bind"`
	Tags          []xmlTag      `xml:"tag"`
	UnknownAttrs  []xml.Attr    `xml:",any,attr"`
	Unknown       []xmlUnknown  `xml:",any"`
}

type xmlService struct {
	ID        string `xml:"id,attr"`
	Class     string `xml:"class,attr"`
	Alias     string `xml:"alias,attr"`
	Namespace string `xml:"namespace,attr"`
	Resource  string `xml:"resource,attr"`

	Public        string `xml:"public,attr"`
	Shared        string `xml:"shared,attr"`
	Synthetic     string `xml:"synthetic,attr"`
	Lazy          string `xml:"lazy,attr"`
	Abstract      string `xml:"abstract,attr"`
	Autowire      string `xml:"autowire,attr"`
	Autoconfigure string `xml:"autoconfigure,attr"`
	Parent        string `xml:"parent,attr"`
	Constructor   string `xml:"constructor,attr"`

	Decorates           string `xml:"decorates,attr"`
	DecorationInnerName string `xml:"decoration-inner-name,attr"`
	DecorationPriority  string `xml:"decoration-priority,attr"`
	DecorationOnInvalid string `xml:"decoration-on-invalid,attr"`

	ExcludeAttr string   `xml:"exclude,attr"`
	Excludes    []string `xml:"exclude"`

	Arguments    []xmlArgument  `xml:"argument"`
	Properties   []xmlProperty  `xml:"property"`
	Calls        []xmlCall      `xml:"call"`
	Tags         []xmlTag       `xml:"tag"`
	Binds        []xmlArgument  `xml:"bind"`
	Factory      *xmlCallable   `xml:"factory"`
	Configurator *xmlCallable   `xml:"configurator"`
	Deprecated   *xmlDeprecated `xml:"deprecated"`
	File         string         `xml:"file"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []xmlUnknown `xml:",any"`
}

type xmlArgument struct {
	Type      string `xml:"type,attr"`
	ID        string `xml:"id,attr"`
	Key       string `xml:"key,attr"`
	Index     string `xml:"index,attr"`
	OnInvalid string `xml:"on-invalid,attr"`

	Tag                   string `xml:"tag,attr"`
	IndexBy               string `xml:"index-by,attr"`
	DefaultIndexMethod    string `xml:"default-index-method,attr"`
	DefaultPriorityMethod string `xml:"default-priority-method,attr"`
	ExcludeSelf           string `xml:"exclude-self,attr"`

	Value          string        `xml:",chardata"`
	Children       []xmlArgument `xml:"argument"`
	Excludes       []string      `xml:"exclude"`
	InlineServices []xmlService  `xml:"service"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []xmlUnknown `xml:",any"`
}

type xmlProperty struct {
	Name string `xml:"name,attr"`
	xmlArgument
}

type xmlCall struct {
	Method       string        `xml:"method,attr"`
	ReturnsClone string        `xml:"returns-clone,attr"`
	Arguments    []xmlArgument `xml:"argument"`
	UnknownAttrs []xml.Attr    `xml:",any,attr"`
	Unknown      []xmlUnknown  `xml:",any"`
}

type xmlTag struct {
	NameAttr   string            `xml:"name,attr"`
	Value      string            `xml:",chardata"`
	Attributes []xmlTagAttribute `xml:"attribute"`
	OtherAttrs []xml.Attr        `xml:",any,attr"`
}

type xmlTagAttribute struct {
	Name     string            `xml:"name,attr"`
	Value    string            `xml:",chardata"`
	Children []xmlTagAttribute `xml:"attribute"`
}

type xmlCallable struct {
	Class        string       `xml:"class,attr"`
	Service      string       `xml:"service,attr"`
	Method       string       `xml:"method,attr"`
	Function     string       `xml:"function,attr"`
	Expression   string       `xml:"expression,attr"`
	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []xmlUnknown `xml:",any"`
}

type xmlDeprecated struct {
	Package string `xml:"package,attr"`
	Version string `xml:"version,attr"`
	Message string `xml:",chardata"`
}

// ParseServicesXML parses the content of a Symfony services.xml file.
func ParseServicesXML(content []byte) (*Container, error) {
	var container Container

	if err := xml.Unmarshal(content, &container); err != nil {
		return nil, fmt.Errorf("failed to parse services.xml: %w", err)
	}

	return &container, nil
}

func unknownElementError(parent string, unknown []xmlUnknown) error {
	if len(unknown) == 0 {
		return nil
	}

	return fmt.Errorf("unsupported element <%s> inside <%s>", unknown[0].XMLName.Local, parent)
}

func unknownAttributeError(parent string, attrs []xml.Attr) error {
	for _, attr := range attrs {
		// Namespace declarations and schema locations carry no configuration.
		if attr.Name.Space == "xmlns" || attr.Name.Local == "xmlns" || attr.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" {
			continue
		}

		return fmt.Errorf("unsupported attribute %q on <%s>", attr.Name.Local, parent)
	}

	return nil
}
