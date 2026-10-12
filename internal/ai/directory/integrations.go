package directory

// integrations is the hardwired directory of Shopware AI integrations. There is
// no remote source and nothing is fetched at runtime, so the directory lives
// directly in Go. Repository is the GitHub "owner/repo" the skill is installed
// from; the shopware-cli skills ship in this repository.
var integrations = Directory{
	Integrations: []Integration{
		{
			Name:          "shopware-cli",
			DisplayName:   "Shopware CLI",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Use Shopware CLI effectively for project, extension, and account workflows.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/products/cli/",
			Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/shopware-cli"},
		},
		{
			Name:          "shopware-cli-docker",
			DisplayName:   "Shopware CLI (Docker)",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Run commands in Docker-backed Shopware projects through Shopware CLI.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/products/cli/",
			Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/shopware-cli"},
		},
		{
			Name:          "deployment-helper",
			DisplayName:   "Shopware Deployment Helper",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Use Shopware CLI and Deployment Helper together for build and deploy workflows.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/guides/hosting/installation-updates/deployments/deployment-helper/index.html",
			Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/deployment-helper"},
		},
		{
			Name:          "acceptance-test-suite",
			DisplayName:   "Shopware Acceptance Test Suite",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Write and run Playwright acceptance and accessibility tests for a Shopware project with the Acceptance Test Suite.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/guides/development/testing/e2e-playwright/",
			Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/acceptance-test-suite"},
		},
	},
}
