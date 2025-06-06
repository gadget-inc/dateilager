import { execSync } from "child_process";
import path from "path";

const rootDir = path.resolve(__dirname, "..");

// Ensure the current git state is clean and the current commit has been pushed
function ensureGitState(): void {
  const status = execSync("git status --porcelain", { encoding: "utf8" }).trim();
  if (status !== "") {
    console.error("You have uncommitted changes");
    console.error("Please commit or stash them before pre-releasing");
    process.exit(1);
  }

  execSync("git push origin HEAD", { encoding: "utf8" }).trim();
}

function preReleaseVersion(): string {
  const gitSha = execSync("git rev-parse --short HEAD", { encoding: "utf8" }).trim();
  return `0.0.0-pre.${gitSha}`;
}

function buildDockerContainer(versionTag: string): void {
  execSync(`make upload-prerelease-container-image version_tag=${versionTag}`, { stdio: "inherit" });
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

function publishPreReleaseJsPackageToGithub(): void {
  execSync(`npm run prerelease`, { stdio: "inherit", cwd: path.join(rootDir, "js") });
}

function doPreRelease(): void {
  ensureGitState();
  const version = preReleaseVersion();
  console.log(`Running prerelease with version: ${version}`);

  buildDockerContainer(version); // build and push docker container with the prerelease version
  tagGit(version); // tag the current commit and push the tag to run the prerelease github action
  publishPreReleaseJsPackageToGithub(); // publish the prerelease js package to github

  console.log(`Prerelease version ${version} published`);
  console.warn(`The package.json version has been updated and is now ${version}`);
}

// Run the function
if (require.main === module) {
  doPreRelease();
}
