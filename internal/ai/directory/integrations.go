package directory

// integrations is the hardwired directory of Shopware AI integrations. There is
// no remote source and nothing is fetched at runtime, so the directory lives
// directly in Go.
var integrations = Directory{
	Integrations: []Integration{
		{
			Name:        "shopware-cli",
			Description: "Use Shopware CLI effectively for project, extension, and account workflows.",
			Repository:  "https://github.com/shopware/shopware-cli",
		},
		{
			Name:        "shopware-cli-docker",
			Description: "Run commands in Docker-backed Shopware projects through Shopware CLI.",
			Repository:  "https://github.com/shopware/shopware-cli",
		},
		{
			Name:        "deployment-helper",
			Description: "Use Shopware CLI and Deployment Helper together for build and deploy workflows.",
			Repository:  "https://github.com/shopware/deployment-helper",
		},
	},
}
