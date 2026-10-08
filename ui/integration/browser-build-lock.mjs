import { readFile, stat } from "node:fs/promises";
import path from "node:path";

export async function browserDistReady(packageRoot) {
  try {
    for (const file of ["index.html", "assets/index.js", "assets/index.css"]) {
      const info = await stat(path.join(packageRoot, "dist", file));
      if (!info.isFile() || info.size === 0) return false;
    }
    const bundle = await readFile(
      path.join(packageRoot, "dist", "assets", "index.js"),
      "utf8",
    );
    return bundle.includes("/sync-preflight");
  } catch {
    return false;
  }
}

export async function runSharedBrowserBuild({
  build,
  buildCacheKey,
  buildState = globalThis,
  ready,
  prebuilt = false,
}) {
  if (!buildState[buildCacheKey]) {
    const buildPromise = Promise.resolve()
      .then(async () => {
        if (!(await ready())) {
          if (prebuilt)
            throw new Error("Prebuilt dashboard output is missing or invalid");
          await build();
        }
      })
      .catch((error) => {
        if (buildState[buildCacheKey] === buildPromise) {
          delete buildState[buildCacheKey];
        }
        throw error;
      });
    buildState[buildCacheKey] = buildPromise;
  }

  await buildState[buildCacheKey];
}
