#!/usr/bin/env -S  node --import @swc-node/register/esm-register
import { execSync } from "child_process";
import fs from "fs";
import path from "path";
import yargs from "yargs";
import { hideBin } from "yargs/helpers";

//Define interface for package.json structure
interface PackageJson {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [key: string]: any;
  name: string;
  version: string;
}

// Path to package.json (defaults to current directory)
const packagePath: string = path.resolve(process.cwd(), "package.json");
const defaultNixPath: string = path.resolve(process.cwd(), "../default.nix");
// Get the current git commit SHA
function getGitCommitSha(): string {
  try {
    // Execute git command to get the current commit SHA
    const sha: string = execSync("git rev-parse HEAD", { encoding: "utf8" }).trim();
    // Return only the first 7 characters
    return sha.substring(0, 7);
  } catch (error) {
    console.error("Error getting git commit SHA:", (error as Error).message);
    process.exit(1);
  }
}

function preReleaseVersion(baseVersion: string): string {
  const sha = getGitCommitSha();
  return `${baseVersion}-pre.${sha}`;
}

// Read and update package.json
function updatePackageVersion(version: string): void {
  try {
    // Read the package.json file
    const packageData: string = fs.readFileSync(packagePath, "utf8");
    const packageJson: PackageJson = JSON.parse(packageData) as PackageJson;

    // Store the original version for logging
    const originalVersion: string = packageJson.version;

    // Update the version with the git SHA
    packageJson.version = version;

    // Write the updated package.json back to file
    fs.writeFileSync(packagePath, JSON.stringify(packageJson, null, 2) + "\n", "utf8");

    execSync(`npm install`, { stdio: "inherit" });

    console.log(`Package version updated from "${originalVersion}" to "${version}"`);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      console.error(`Error: ${packagePath} not found`);
    } else {
      console.error("Error updating package.json:", (error as Error).message);
    }
    process.exit(1);
  }
}

function updateDefaultNixVersion(version: string): void {
  try {
    // Read the package.json file
    const defaultNix: string = fs.readFileSync(defaultNixPath, "utf8");
    fs.writeFileSync(defaultNixPath, defaultNix.replace(/version = "[0-9.]+(-pre.*)?";/, `version = "${version}";`), "utf8");

    console.log(`Default nix version updated from to "${version}"`);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      console.error(`Error: ${defaultNixPath} not found`);
    } else {
      console.error("Error updating package.json:", (error as Error).message);
    }
    process.exit(1);
  }
}

function gitAdd(version: string): void {
  execSync(`git add package.json package-lock.json ../default.nix`, { stdio: "inherit" });
  execSync(`git commit -m "Update version to ${version}"`, { stdio: "inherit" });
  execSync(`git push origin HEAD`, { stdio: "inherit" });
}

function tagGit(version: string): void {
  try {
    execSync(`git tag -f v${version} $(git rev-parse HEAD)`, { stdio: "inherit" });
    execSync(`git push origin v${version}`, { stdio: "inherit" });
  } catch (error) {
    console.error("Error tagging git:", (error as Error).message);
    process.exit(1);
  }
}

function publishPreReleaseToGithub(): void {
  execSync(`npm run prerelease`, { stdio: "inherit" });
}

function doPreRelease(): void {
  console.log(`Running prerelease with version: ${process.argv.toString()}`);
  const args = yargs(hideBin(process.argv))
    .option("t", {
      description: "Version tag to release",
      type: "string",
      alias: "version-tag",
      demandOption: true,
    })
    .help().argv;

  console.log(`Running prerelease with version: ${args.t}`);

  const version = preReleaseVersion(args.t);
  console.log(`Setting prerelease version to: ${version}`);

  // Update the package version and publish to github
  updatePackageVersion(version);
  updateDefaultNixVersion(version);
  gitAdd(version);
  tagGit(version); // To kick off the prerelease build
  publishPreReleaseToGithub();

  console.log(`Prerelease version ${version} published`);
  console.warn(`The package.json version has been updated and is now ${version}`);
}

// Run the function
if (require.main === module) {
  doPreRelease();
}
