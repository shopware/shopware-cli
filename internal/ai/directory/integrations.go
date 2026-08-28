package directory

// integrations is the hardwired directory of Shopware AI integrations. There is
// no remote source and nothing is fetched at runtime, so the directory lives
// directly in Go. Validate (exercised by tests) guards its invariants.
var integrations = Directory{
	Version: 1,
	Integrations: []Integration{
		{
			Name:          "shopware-cli",
			DisplayName:   "Shopware CLI",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Use Shopware CLI effectively for project, extension, and account workflows.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/products/cli/",
			Delivery:      Delivery{Kind: DeliveryBundled},
		},
		{
			Name:          "shopware-cli-docker",
			DisplayName:   "Shopware CLI (Docker)",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Run commands in Docker-backed Shopware projects through Shopware CLI.",
			Status:        StatusActive,
			Documentation: "https://developer.shopware.com/docs/products/cli/",
			Delivery:      Delivery{Kind: DeliveryBundled},
		},
		{
			Name:          "deployment-helper",
			DisplayName:   "Shopware Deployment Helper",
			Type:          TypeSkill,
			Provider:      "shopware",
			Description:   "Use Shopware CLI and Deployment Helper together for build and deploy workflows.",
			Status:        StatusComingSoon,
			Documentation: "https://developer.shopware.com/docs/guides/hosting/installation-updates/deployments/deployment-helper/index.html",
			Delivery: Delivery{
				Kind:       DeliveryGit,
				Repository: "https://github.com/shopware/deployment-helper",
			},
			Compatibility: &Compatibility{Source: "owner"},
		},
	},
}
