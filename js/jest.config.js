// For a detailed explanation regarding each configuration property, visit:
// https://jestjs.io/docs/en/configuration.html

/** @type {import('@jest/types').Config.InitialOptions} */
const config = {
  // A map from regular expressions to paths to transformers.
  transform: { "^.+\\.[jt]sx?$": ["@swc/jest", { jsc: { target: "es2020" } }] },

  // An array of regexp pattern strings that are matched against all test paths before executing the test.
  // If the test path matches any of the patterns, it will be skipped.
  testPathIgnorePatterns: ["/node_modules/", "/dist/"],
};

module.exports = config;
