import globals from "globals";
import pluginJs from "@eslint/js";
import tseslint from "typescript-eslint";
import pluginVue from "eslint-plugin-vue";
import pluginVueA11y from "eslint-plugin-vuejs-accessibility";
import adminRules from "@shopware-ag/admin-eslint-rules";
import pluginInclusiveLanguage from "eslint-plugin-inclusive-language";
import eslintConfigPrettier from "eslint-config-prettier";

/** @type {import('eslint').Linter.Config[]} */
export default [
	{
		languageOptions: {
			globals: {
				...globals.browser,
				Shopware: true,
				VueJS: true,
			},
		},
	},
	eslintConfigPrettier,
	...pluginVue.configs["flat/base"],
	tseslint.configs.base,
	tseslint.configs.eslintRecommended,
	{
		files: ["**/*.js"],
		plugins: adminRules.plugins,
		rules: {
			...pluginJs.configs.recommended.rules,
			...tseslint.configs.recommended[2].rules,
			...pluginVue.configs["flat/recommended"]
				.filter((e) => e.hasOwnProperty("rules") && !e.hasOwnProperty("files"))
				.map((e) => e.rules)
				.reduce((acc, curr) => ({ ...acc, ...curr }), {}),
			...pluginVueA11y.configs["flat/recommended"]
				.filter((e) => e.hasOwnProperty("rules") && !e.hasOwnProperty("files"))
				.map((e) => e.rules)
				.reduce((acc, curr) => ({ ...acc, ...curr }), {}),
			...adminRules.rules,
			"vue/require-prop-types": "error",
			"vue/require-default-prop": "error",
			"vue/no-mutating-props": "error",
			"vue/component-definition-name-casing": ["error", "kebab-case"],
			"vue/no-boolean-default": ["error", "default-false"],
			"vue/order-in-components": [
				"error",
				{
					order: [
						"el",
						"name",
						"parent",
						"functional",
						["template", "render"],
						"inheritAttrs",
						["provide", "inject"],
						"emits",
						"extends",
						"mixins",
						"model",
						["components", "directives", "filters"],
						["props", "propsData"],
						"data",
						"metaInfo",
						"computed",
						"watch",
						"LIFECYCLE_HOOKS",
						"methods",
						["delimiters", "comments"],
						"renderError",
					],
				},
			],
			"vue/no-deprecated-destroyed-lifecycle": "error",
			"vue/no-deprecated-events-api": "error",
			"vue/require-slots-as-functions": "error",
			"vue/no-deprecated-props-default-this": "error",
			"no-undef": "off",
			"no-alert": "error",
			"no-console": ["error", { allow: ["warn", "error"] }],
			"@typescript-eslint/no-unused-vars": "warn"
		},
	},
	{
		files: ["**/snippet/*.json"],
		plugins: {
			"inclusive-language": pluginInclusiveLanguage,
		},
		rules: {
			"inclusive-language/use-inclusive-words": "error",
		},
	},
];
