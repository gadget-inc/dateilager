const { execSync } = require("child_process");

module.exports = () => ({
  getTagName: (pkg) => {
    const sha = execSync("git rev-parse --short HEAD", { encoding: "utf8" }).trim();
    return `${pkg.name}-v0.0.0-pre.${sha}-gitpkg`;
  },
});
