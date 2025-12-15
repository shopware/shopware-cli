package packagist

import (
	"encoding/json"
	"os"
)

type ComposerJsonAuthor struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	Role     string `json:"role,omitempty"`
}

type ComposerJsonSupport struct {
	Email    string `json:"email,omitempty"`
	Issues   string `json:"issues,omitempty"`
	Forum    string `json:"forum,omitempty"`
	Wiki     string `json:"wiki,omitempty"`
	IRC      string `json:"irc,omitempty"`
	Source   string `json:"source,omitempty"`
	Docs     string `json:"docs,omitempty"`
	RSS      string `json:"rss,omitempty"`
	Chat     string `json:"chat,omitempty"`
	Security string `json:"security,omitempty"`
}

type ComposerFunding struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

type ComposerPackageLink map[string]string

type ComposerJsonAutoload struct {
	Psr0     map[string]string `json:"psr-0,omitempty"`
	Psr4     map[string]string `json:"psr-4,omitempty"`
	Classmap []string          `json:"classmap,omitempty"`
	Files    []string          `json:"files,omitempty"`
	Exclude  []string          `json:"exclude-from-classmap,omitempty"`
}

type ComposerJsonRepository struct {
	Type    string         `json:"type,omitempty"`
	URL     string         `json:"url,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

type ComposerJsonRepositories []ComposerJsonRepository

func (e *ComposerJsonRepositories) UnmarshalJSON(data []byte) error {
	var asMap map[string]ComposerJsonRepository

	if err := json.Unmarshal(data, &asMap); err == nil {
		*e = ComposerJsonRepositories{}

		for _, v := range asMap {
			*e = append(*e, v)
		}

		return nil
	}

	var asArray []ComposerJsonRepository
	err := json.Unmarshal(data, &asArray)

	if err != nil {
		return err
	}

	*e = asArray

	return nil
}

func (r *ComposerJsonRepositories) HasRepository(url string) bool {
	for _, repository := range *r {
		if repository.URL == url {
			return true
		}
	}

	return false
}

type ComposerJson struct {
	path               string                   `json:"-"`
	Name               string                   `json:"name"`
	Abandoned          bool                     `json:"abandoned,omitempty"`
	Bin                []string                 `json:"bin,omitempty"`
	Description        string                   `json:"description,omitempty"`
	Version            string                   `json:"version,omitempty"`
	Type               string                   `json:"type,omitempty"`
	Keywords           []string                 `json:"keywords,omitempty"`
	Homepage           string                   `json:"homepage,omitempty"`
	Readme             string                   `json:"readme,omitempty"`
	Time               string                   `json:"time,omitempty"`
	License            string                   `json:"license,omitempty"`
	MinimumStability   string                   `json:"minimum-stability,omitempty"`
	PreferStable       bool                     `json:"prefer-stable,omitempty"`
	Authors            []ComposerJsonAuthor     `json:"authors,omitempty"`
	Support            *ComposerJsonSupport     `json:"support,omitempty"`
	Funding            []ComposerFunding        `json:"funding,omitempty"`
	Require            ComposerPackageLink      `json:"require,omitempty"`
	RequireDev         ComposerPackageLink      `json:"require-dev,omitempty"`
	Conflict           ComposerPackageLink      `json:"conflict,omitempty"`
	Replace            ComposerPackageLink      `json:"replace,omitempty"`
	Provide            ComposerPackageLink      `json:"provide,omitempty"`
	Autoload           ComposerJsonAutoload     `json:"autoload,omitempty"`
	AutoloadDev        ComposerJsonAutoload     `json:"autoload-dev,omitempty"`
	Repositories       ComposerJsonRepositories `json:"repositories,omitempty"`
	Config             map[string]any           `json:"config,omitempty"`
	Scripts            map[string]any           `json:"scripts,omitempty"`
	Extra              map[string]any           `json:"extra,omitempty"`
	Suggest            map[string]string        `json:"suggest,omitempty"`
	NonFeatureBranches []string                 `json:"non-feature-branches,omitempty"`
}

func (c *ComposerJson) HasPackage(name string) bool {
	_, ok := c.Require[name]
	return ok
}

func (c *ComposerJson) HasPackageDev(name string) bool {
	_, ok := c.RequireDev[name]
	return ok
}

func (c *ComposerJson) HasConfig(key string) bool {
	_, ok := c.Config[key]
	return ok
}

func (c *ComposerJson) EnableComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		allowedPlugins = map[string]any{}
	}

	allowedPlugins[name] = true

	c.Config["allow-plugins"] = allowedPlugins
}

func (c *ComposerJson) RemoveComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		return
	}

	delete(allowedPlugins, name)

	c.Config["allow-plugins"] = allowedPlugins
}

func (c *ComposerJson) Save() error {
	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, content, os.ModePerm)
}

func ReadComposerJson(composerPath string) (*ComposerJson, error) {
	var composerJson ComposerJson
	composerJson.path = composerPath

	content, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(content, &composerJson); err != nil {
		return nil, err
	}

	if composerJson.Extra == nil {
		composerJson.Extra = map[string]any{}
	}

	return &composerJson, nil
}
