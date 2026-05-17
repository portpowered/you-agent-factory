import { mkdtemp, mkdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { expect, test } from "vitest";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "normalize-dist-output.mjs",
);

test("normalize-dist-output prunes empty dist directories before generating Go embeds", async () => {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "normalize-dist-output-"));
  const uiDir = path.join(tempRoot, "ui");
  const distDir = path.join(uiDir, "dist");
  const assetsDir = path.join(distDir, "assets");
  const emptyDashboardDir = path.join(distDir, "dashboard", "ui");

  try {
    await mkdir(assetsDir, { recursive: true });
    await mkdir(emptyDashboardDir, { recursive: true });
    await writeFile(path.join(assetsDir, "index-abc123.js"), "console.log('ok');\n");
    await writeFile(path.join(assetsDir, "index-def456.css"), "body{}\n");
    await writeFile(
      path.join(distDir, "index.html"),
      '<script src="/dashboard/ui/assets/index-abc123.js"></script><link rel="stylesheet" href="/dashboard/ui/assets/index-def456.css">',
    );
    await writeFile(path.join(uiDir, "dist_stamp.go"), "package ui\n");

    await execFileAsync(process.execPath, [scriptPath], {
      env: { ...process.env, AGENT_FACTORY_UI_DIR: uiDir },
    });

    await expect(readFile(path.join(assetsDir, "index.js"), "utf8")).resolves.toBe(
      "console.log('ok');\n",
    );
    await expect(readFile(path.join(assetsDir, "index.css"), "utf8")).resolves.toBe("body{}\n");

    const normalizedHtml = await readFile(path.join(distDir, "index.html"), "utf8");
    expect(normalizedHtml).toMatch(/\/dashboard\/ui\/assets\/index\.js/);
    expect(normalizedHtml).toMatch(/\/dashboard\/ui\/assets\/index\.css/);
    expect(normalizedHtml).not.toMatch(/index-abc123\.js|index-def456\.css/);

    await expect(stat(emptyDashboardDir)).rejects.toThrow();
    await expect(stat(path.join(uiDir, "dist_stamp.go"))).rejects.toThrow();

    const generatedEmbed = await readFile(path.join(uiDir, "dist_embed_generated.go"), "utf8");
    expect(generatedEmbed).toMatch(/\/\/go:embed dist dist\/\*/);
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
});
