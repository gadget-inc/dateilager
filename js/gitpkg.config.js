const { execSync } = require("child_process");

module.exports = () => ({ 
  getTagName: (pkg) => {
    if (!pkg.version.includes('pre')) {
      console.error(`Version ${pkg.version} does not include 'pre', please use 'make prerelease'`);
      process.exit(1);
    }
    return `${pkg.name}-${pkg.version}-gitpkg`;
  },
});
