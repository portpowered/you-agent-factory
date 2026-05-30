/**
 * Bun node lane: align Node fs/url URL handling with Node 20+ so Vite config tests can import vite.
 */
import * as nodeFs from "node:fs";
import * as nodeUrl from "node:url";
import { mock } from "bun:test";

const originalReadFileSync = nodeFs.readFileSync.bind(nodeFs);
const originalFileURLToPath = nodeUrl.fileURLToPath.bind(nodeUrl);

function normalizeUrlInput(url: string | URL): string {
  return url instanceof URL ? url.href : url;
}

mock.module("node:url", () => ({
  ...nodeUrl,
  fileURLToPath(url: string | URL) {
    return originalFileURLToPath(normalizeUrlInput(url));
  },
}));

mock.module("node:fs", () => ({
  ...nodeFs,
  readFileSync(
    path: Parameters<typeof nodeFs.readFileSync>[0],
    options?: Parameters<typeof nodeFs.readFileSync>[1],
  ) {
    const normalized =
      path instanceof URL ? originalFileURLToPath(path.href) : path;
    return originalReadFileSync(normalized, options);
  },
}));
