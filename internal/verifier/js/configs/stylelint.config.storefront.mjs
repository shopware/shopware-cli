/** @type {import('stylelint').Config} */
export default {
	extends: ["stylelint-config-recommended-scss"],
	customSyntax: "postcss-scss",
	plugins: ["stylelint-scss"],
	rules: {
		"selector-class-pattern": null,
		"import-notation": null,
		"declaration-property-value-no-unknown": null,
		"at-rule-no-unknown": null,
		"no-descending-specificity": null,
		"max-nesting-depth": [
			3,
			{
				ignore: ["blockless-at-rules", "pseudo-classes"],
				severity: "warning",
			},
		],
		"selector-max-type": [
			0,
			{
				severity: "warning",
				message:
					'Selectors containing elements like "%s" should be avoided because the element type might change. Prefer to use .classes, #ids and [data-attributes] instead.',
			},
		],
		"declaration-no-important": [
			true,
			{
				severity: "warning",
				message:
					"Avoid using !important in declarations. It makes the CSS harder to maintain and let others override your styles.",
			},
		],
		"scss/load-partial-extension": null,
		"scss/at-extend-no-missing-placeholder": [
			true,
			{
				severity: "warning",
			},
		],
	},
};
