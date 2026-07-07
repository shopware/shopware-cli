package symfony

import (
	"encoding/xml"
	"fmt"
)

// Routes is the parsed representation of a Symfony routes.xml file.
type Routes struct {
	XMLName xml.Name       `xml:"routes"`
	Items   []xmlRouteItem `xml:",any"`
}

// xmlRouteItem keeps route, import and when elements in document order, as
// the definition order determines the route matching priority.
type xmlRouteItem struct {
	Kind   string
	Route  *xmlRoute
	Import *xmlRouteImport
	When   *xmlRoutesWhen
}

func (i *xmlRouteItem) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	i.Kind = start.Name.Local

	switch start.Name.Local {
	case "route":
		i.Route = &xmlRoute{}
		return d.DecodeElement(i.Route, &start)
	case "import":
		i.Import = &xmlRouteImport{}
		return d.DecodeElement(i.Import, &start)
	case "when":
		i.When = &xmlRoutesWhen{}
		return d.DecodeElement(i.When, &start)
	default:
		return d.Skip()
	}
}

type xmlRoutesWhen struct {
	Env   string         `xml:"env,attr"`
	Items []xmlRouteItem `xml:",any"`
}

type xmlRoute struct {
	ID         string `xml:"id,attr"`
	Path       string `xml:"path,attr"`
	Host       string `xml:"host,attr"`
	Controller string `xml:"controller,attr"`
	Schemes    string `xml:"schemes,attr"`
	Methods    string `xml:"methods,attr"`
	Locale     string `xml:"locale,attr"`
	Format     string `xml:"format,attr"`
	UTF8       string `xml:"utf8,attr"`
	Stateless  string `xml:"stateless,attr"`

	Paths        []xmlLocalizedValue `xml:"path"`
	Hosts        []xmlLocalizedValue `xml:"host"`
	Defaults     []xmlRouteDefault   `xml:"default"`
	Requirements []xmlKeyValue       `xml:"requirement"`
	Options      []xmlKeyValue       `xml:"option"`
	Condition    string              `xml:"condition"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []xmlUnknown `xml:",any"`
}

type xmlRouteImport struct {
	Resource            string `xml:"resource,attr"`
	Type                string `xml:"type,attr"`
	Prefix              string `xml:"prefix,attr"`
	NamePrefix          string `xml:"name-prefix,attr"`
	Host                string `xml:"host,attr"`
	Schemes             string `xml:"schemes,attr"`
	Methods             string `xml:"methods,attr"`
	Locale              string `xml:"locale,attr"`
	Format              string `xml:"format,attr"`
	UTF8                string `xml:"utf8,attr"`
	Stateless           string `xml:"stateless,attr"`
	TrailingSlashOnRoot string `xml:"trailing-slash-on-root,attr"`
	ExcludeAttr         string `xml:"exclude,attr"`

	Prefixes     []xmlLocalizedValue `xml:"prefix"`
	Hosts        []xmlLocalizedValue `xml:"host"`
	Defaults     []xmlRouteDefault   `xml:"default"`
	Requirements []xmlKeyValue       `xml:"requirement"`
	Options      []xmlKeyValue       `xml:"option"`
	Excludes     []string            `xml:"exclude"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []xmlUnknown `xml:",any"`
}

type xmlLocalizedValue struct {
	Locale string `xml:"locale,attr"`
	Value  string `xml:",chardata"`
}

type xmlKeyValue struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// xmlRouteDefault is a route default value, either given as plain text
// (which Symfony treats as a string), as xsi:nil or as one of the typed
// child elements (bool, int, float, string, list, map).
type xmlRouteDefault struct {
	Key   string          `xml:"key,attr"`
	Nil   string          `xml:"http://www.w3.org/2001/XMLSchema-instance nil,attr"`
	Value string          `xml:",chardata"`
	Typed []xmlTypedValue `xml:",any"`
}

type xmlTypedValue struct {
	XMLName  xml.Name
	Key      string          `xml:"key,attr"`
	Nil      string          `xml:"http://www.w3.org/2001/XMLSchema-instance nil,attr"`
	Value    string          `xml:",chardata"`
	Children []xmlTypedValue `xml:",any"`
}

// ParseRoutesXML parses the content of a Symfony routes.xml file.
func ParseRoutesXML(content []byte) (*Routes, error) {
	var routes Routes

	if err := xml.Unmarshal(content, &routes); err != nil {
		return nil, fmt.Errorf("failed to parse routes.xml: %w", err)
	}

	return &routes, nil
}

func isXSINil(value string) bool {
	return value == "true" || value == "1"
}
