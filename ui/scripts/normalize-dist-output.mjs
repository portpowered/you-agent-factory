import { createHash } from "node:crypto";
import { readdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const uiDir = path.resolve(scriptDir, "..");
const distDir = path.join(uiDir, "dist");
const assetsDir = path.join(distDir, "assets");
const distEmbedPath = path.join(uiDir, "dist_embed_generated.go");
const legacyDistStampPath = path.join(uiDir, "dist_stamp.go");

async function listFiles(rootDir, currentDir = rootDir) {
  const entries = await readdir(currentDir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(currentDir, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listFiles(rootDir, fullPath));
      continue;
    }

    files.push(path.relative(rootDir, fullPath).replaceAll("\\", "/"));
  }

  return files.sort();
}

async function normalizeSingleAsset(extension, targetName) {
  const assetNames = (await readdir(assetsDir)).filter((name) => name.endsWith(extension));
  if (assetNames.length !== 1) {
    throw new Error(`Expected exactly one ${extension} asset in dist/assets, found ${assetNames.length}.`);
  }

  const sourcePath = path.join(assetsDir, assetNames[0]);
  const targetPath = path.join(assetsDir, targetName);

  if (assetNames[0] !== targetName) {
    await rm(targetPath, { force: true });
    await rename(sourcePath, targetPath);
  }
}

async function rewriteIndexHtml() {
  const indexPath = path.join(distDir, "index.html");
  const current = await readFile(indexPath, "utf8");
  const normalized = current
    .replace(/\/dashboard\/ui\/assets\/[^"]+\.js/g, "/dashboard/ui/assets/index.js")
    .replace(/\/dashboard\/ui\/assets\/[^"]+\.css/g, "/dashboard/ui/assets/index.css");

  if (normalized !== current) {
    await writeFile(indexPath, normalized);
  }
}

async function writeDistEmbedRegistration() {
  const hash = createHash("sha256");
  for (const relativePath of await listFiles(distDir)) {
    const absolutePath = path.join(distDir, relativePath);
    const fileStat = await stat(absolutePath);
    if (!fileStat.isFile()) {
      continue;
    }

    hash.update(relativePath);
    hash.update("\n");
    hash.update(await readFile(absolutePath));
    hash.update("\n");
  }

  await writeFile(
    distEmbedPath,
    `package ui

import (
	"embed"
	"io/fs"
)

var (
	// distBuildStamp keeps Go's build cache aligned with embedded dist asset changes.
	distBuildStamp = "${hash.digest("hex")}"

	//go:embed dist dist/*
	generatedDist embed.FS
)

func init() {
	distFSProvider = func() (fs.FS, error) {
		return fs.Sub(generatedDist, "dist")
	}
}
`,
  );
}

await normalizeSingleAsset(".js", "index.js");
await normalizeSingleAsset(".css", "index.css");
await rewriteIndexHtml();
await rm(legacyDistStampPath, { force: true });
await writeDistEmbedRegistration();
