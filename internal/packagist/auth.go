package packagist

import (
	"encoding/json"
	"maps"
	"os"
)

type ComposerAuthHttpBasic struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ComposerAuth struct {
	path           string                           `json:"-"`
	HTTPBasicAuth  map[string]ComposerAuthHttpBasic `json:"http-basic,omitempty"`
	BearerAuth     map[string]string                `json:"bearer,omitempty"`
	GitlabAuth     map[string]string                `json:"gitlab-token,omitempty"`
	GithubOAuth    map[string]string                `json:"github-oauth,omitempty"`
	BitbucketOauth map[string]map[string]string     `json:"bitbucket-oauth,omitempty"`
}

func (a *ComposerAuth) Save() error {
	content, err := a.Json(true)
	if err != nil {
		return err
	}

	return os.WriteFile(a.path, content, os.ModePerm)
}

func (a *ComposerAuth) Json(formatted bool) ([]byte, error) {
	if !formatted {
		return json.Marshal(a)
	}
	return json.MarshalIndent(a, "", "  ")
}

func fillAuthStruct(auth *ComposerAuth) *ComposerAuth {
	if auth.BearerAuth == nil {
		auth.BearerAuth = map[string]string{}
	}

	if auth.HTTPBasicAuth == nil {
		auth.HTTPBasicAuth = map[string]ComposerAuthHttpBasic{}
	}

	if auth.GitlabAuth == nil {
		auth.GitlabAuth = map[string]string{}
	}

	if auth.GithubOAuth == nil {
		auth.GithubOAuth = map[string]string{}
	}

	if auth.BitbucketOauth == nil {
		auth.BitbucketOauth = map[string]map[string]string{}
	}

	composerToken := os.Getenv("SHOPWARE_PACKAGES_TOKEN")
	composerAuth := os.Getenv("COMPOSER_AUTH")

	if composerToken != "" {
		auth.BearerAuth["packages.shopware.com"] = composerToken
	}

	if composerAuth != "" {
		var envAuth ComposerAuth
		if err := json.Unmarshal([]byte(composerAuth), &envAuth); err != nil {
			return auth
		}

		maps.Copy(auth.HTTPBasicAuth, envAuth.HTTPBasicAuth)
		maps.Copy(auth.BearerAuth, envAuth.BearerAuth)
		maps.Copy(auth.GitlabAuth, envAuth.GitlabAuth)
		maps.Copy(auth.GithubOAuth, envAuth.GithubOAuth)
		maps.Copy(auth.BitbucketOauth, envAuth.BitbucketOauth)
	}

	return auth
}

func ReadComposerAuth(authFile string) (*ComposerAuth, error) {
	content, err := os.ReadFile(authFile)
	if err != nil {
		if os.IsNotExist(err) {
			auth := fillAuthStruct(&ComposerAuth{})
			auth.path = authFile
			return auth, nil
		}
		return nil, err
	}

	var auth ComposerAuth
	if err := json.Unmarshal(content, &auth); err != nil {
		return nil, err
	}

	auth.path = authFile

	return fillAuthStruct(&auth), nil
}
