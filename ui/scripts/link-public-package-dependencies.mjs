import { lstat, mkdir, realpath, symlink } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const packageScope = path.join(uiRoot, "node_modules", "@you-agent-factory");
const workspacePackages = ["client", "factory-replay"];

async function pathExists(targetPath) {
  try {
    await lstat(targetPath);
    return true;
  } catch (error) {
    if (error && typeof error === "object" && error.code === "ENOENT") {
      return false;
    }
    throw error;
  }
}

await mkdir(packageScope, { recursive: true });

for (const packageName of workspacePackages) {
  const source = path.join(uiRoot, "packages", packageName);
  const destination = path.join(packageScope, packageName);

  if (await pathExists(destination)) {
    const resolvedDestination = await realpath(destination);
    const resolvedSource = await realpath(source);
    if (resolvedDestination !== resolvedSource) {
      throw new Error(
        `Refusing to replace unexpected package dependency at ${destination}.`,
      );
    }
    continue;
  }

  await symlink(
    source,
    destination,
    process.platform === "win32" ? "junction" : "dir",
  );
  console.log(`[public-package-link] ${packageName} -> ${source}`);
}
