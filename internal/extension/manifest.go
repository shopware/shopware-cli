package extension

type Manifest struct {
	Meta  Meta   `xml:"meta"`
	Setup *Setup `xml:"setup,omitempty"`
}

type Meta struct {
	Name          string             `xml:"name"`
	Label         TranslatableString `xml:"label"`
	Description   TranslatableString `xml:"description,omitempty"`
	Author        string             `xml:"author,omitempty"`
	Copyright     string             `xml:"copyright,omitempty"`
	Version       string             `xml:"version"`
	Icon          string             `xml:"icon,omitempty"`
	License       string             `xml:"license"`
	Compatibility string             `xml:"compatibility,omitempty"`
}

type Setup struct {
	RegistrationUrl string `xml:"registrationUrl"`
	Secret          string `xml:"secret,omitempty"`
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
