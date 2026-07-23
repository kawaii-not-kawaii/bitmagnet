import { fixupConfigRules, fixupPluginRules } from "@eslint/compat";
import typescriptEslintEslintPlugin from "@typescript-eslint/eslint-plugin";
import angular from "angular-eslint";
import _import from "eslint-plugin-import";
import globals from "globals";
import tsParser from "@typescript-eslint/parser";
import path from "node:path";
import { fileURLToPath } from "node:url";
import js from "@eslint/js";
import { FlatCompat } from "@eslint/eslintrc";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const compat = new FlatCompat({
  baseDirectory: __dirname,
  recommendedConfig: js.configs.recommended,
  allConfig: js.configs.all,
});

export default [
  {
    ignores: [
      ".angular/**/*.*",
      "dist/**/*.*",
      "src/app/graphql/generated/**/*.*",
    ],
  },
  ...angular.configs.tsRecommended.map((config) => ({
    ...config,
    files: ["**/*.ts"],
  })),
  {
    files: ["**/*.ts"],
    processor: angular.processInlineTemplates,
    rules: {
      "@angular-eslint/directive-selector": [
        "error",
        { type: "attribute", prefix: "app", style: "camelCase" },
      ],
      "@angular-eslint/component-selector": [
        "error",
        { type: "element", prefix: "app", style: "kebab-case" },
      ],
    },
  },
  ...[
    ...angular.configs.templateRecommended,
    ...angular.configs.templateAccessibility,
  ].map((config) => ({
    ...config,
    files: ["src/**/*.html"],
  })),
  {
    files: ["src/**/*.html"],
    rules: {
      // null-check idiom (x == null) is used deliberately in templates
      "@angular-eslint/template/eqeqeq": [
        "error",
        { allowNullOrUndefined: true },
      ],
      // labels wrap mat-select rather than native controls
      "@angular-eslint/template/label-has-associated-control": [
        "error",
        { controlComponents: ["mat-select"] },
      ],
    },
  },
  ...fixupConfigRules(
    compat.extends(
      "plugin:@typescript-eslint/recommended",
      "plugin:@typescript-eslint/recommended-requiring-type-checking",
      "plugin:import/errors",
      "plugin:import/typescript",
      "plugin:prettier/recommended",
    ),
  ).map((config) => ({
    ...config,
    files: ["**/*.ts"],
  })),
  {
    files: ["**/*.ts"],

    plugins: {
      "@typescript-eslint": fixupPluginRules(typescriptEslintEslintPlugin),
      import: fixupPluginRules(_import),
    },

    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.jest,
      },

      parser: tsParser,
      ecmaVersion: 2020,
      sourceType: "module",

      parserOptions: {
        project: "./tsconfig.json",
      },
    },

    settings: {
      "import/resolver": {
        typescript: true,
        node: true,
      },
    },

    rules: {
      "@typescript-eslint/return-await": "error",
      "import/order": "error",
      "no-console": "error",
      "prettier/prettier": "warn",
    },
  },
];
