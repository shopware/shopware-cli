/** @type {import('stylelint').Config} */
export default {
	extends: ["stylelint-config-recommended-scss"],
	customSyntax: "postcss-scss",
	plugins: ["stylelint-scss", "@shopware-ag/admin-stylelint-rules"],
	rules: {
		"selector-class-pattern": null,
		"import-notation": null,
		"declaration-property-value-no-unknown": null,
		"at-rule-no-unknown": null,
		"shopware-administration/no-scss-extension-import": true,
		"no-descending-specificity": null,
		"max-nesting-depth": [
			3,
			{
				ignore: ["blockless-at-rules", "pseudo-classes"],
				severity: "warning",
			},
		],
	},
};
