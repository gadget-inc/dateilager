module.exports = {
  parser: "@typescript-eslint/parser",
  parserOptions: {
    project: ["./tsconfig.eslint.json", "./spec/tsconfig.json"],
  },
  plugins: ["@typescript-eslint", "import", "jest", "jsdoc"],
  extends: [
    "eslint:recommended",
    "plugin:@typescript-eslint/recommended",
    "plugin:@typescript-eslint/recommended-requiring-type-checking",
    "prettier",
    "plugin:import/typescript",
    "plugin:jest/recommended",
    "plugin:jsdoc/recommended",
  ],
  env: {
    node: true,
    jest: true,
  },
  rules: {
    "@typescript-eslint/explicit-member-accessibility": ["warn"],
    "@typescript-eslint/member-ordering": ["warn"],
    "@typescript-eslint/naming-convention": [
      "warn",
      {
        selector: "memberLike",
        modifiers: ["private"],
        format: ["camelCase"],
        leadingUnderscore: "require",
      },
    ],
    "@typescript-eslint/no-unused-vars": ["error", { varsIgnorePattern: "^_", argsIgnorePattern: "^_" }],
    "@typescript-eslint/consistent-type-imports": ["error", { prefer: "type-imports" }],

    "jsdoc/check-line-alignment": ["warn", "always"],
    "jsdoc/no-types": ["warn"],
    "jsdoc/require-jsdoc": [
      "warn",
      {
        publicOnly: true,
        contexts: ["PropertyDefinition", "TSInterfaceDeclaration", "TSPropertySignature"],
        require: {
          ArrowFunctionExpression: true,
          ClassDeclaration: true,
          ClassExpression: true,
          FunctionDeclaration: true,
          FunctionExpression: true,
          MethodDefinition: true,
        },
      },
    ],
    "jsdoc/require-param-type": ["off"],
    "jsdoc/require-returns": ["warn"],
    "jsdoc/require-returns-check": ["off"],
    "jsdoc/require-returns-description": ["warn"],
    "jsdoc/require-returns-type": ["off"],
    "jsdoc/sort-tags": ["warn"],
  },
  settings: {
    "import/extensions": [".js", ".jsx", ".ts", ".tsx"],
    jsdoc: { ignoreInternal: true },
  },
  ignorePatterns: ["/node_modules/", "/dist/"],
};
