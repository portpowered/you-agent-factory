import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { smokeFrontendPackages } from "./public-package-install-smoke.mjs";

export async function registryMetadata(
  packageName,
  version,
  { request = fetch } = {},
) {
  const registry =
    process.env.npm_config_registry ||
    process.env.NPM_CONFIG_REGISTRY ||
    "https://registry.npmjs.org/";
  const url = new URL(
    `${encodeURIComponent(packageName)}/${encodeURIComponent(version)}`,
    registry.endsWith("/") ? registry : `${registry}/`,
  );
  const response = await request(url, { signal: AbortSignal.timeout(15_000) });
  if (response.status === 404) return null;
  if (!response.ok)
    throw new Error(
      `Registry lookup failed for ${packageName}@${version}: HTTP ${response.status}`,
    );
  const metadata = await response.json();
  if (
    metadata.name !== packageName ||
    metadata.version !== version ||
    !/^[a-f0-9]{40}$/.test(metadata.dist?.shasum) ||
    typeof metadata.dist?.tarball !== "string"
  ) {
    throw new Error(
      `Registry returned invalid metadata for ${packageName}@${version}`,
    );
  }
  return metadata;
}

export async function registryShasum(packageName, version) {
  return (await registryMetadata(packageName, version))?.dist.shasum ?? null;
}

export async function verifyInstalledRegistryPackages(
  evidence,
  {
    metadata = registryMetadata,
    request = fetch,
    smoke = smokeFrontendPackages,
  } = {},
) {
  const root = await mkdtemp(path.join(tmpdir(), "you-registry-packages-"));
  try {
    for (const candidate of evidence.packages) {
      const entry = await metadata(candidate.name, candidate.version);
      if (entry?.dist.shasum !== candidate.shasum)
        throw new Error(`Registry digest conflict for ${candidate.name}`);
      const url = new URL(entry.dist.tarball);
      if (url.protocol !== "https:")
        throw new Error(`Unsafe registry tarball for ${candidate.name}`);
      const response = await request(url, {
        signal: AbortSignal.timeout(30_000),
      });
      if (!response.ok)
        throw new Error(
          `Registry tarball download failed for ${candidate.name}: HTTP ${response.status}`,
        );
      await writeFile(
        path.join(root, candidate.filename),
        Buffer.from(await response.arrayBuffer()),
      );
    }
    // The same integrity checks and public operations now consume downloaded registry artifacts.
    await smoke({ candidateDirectory: root, evidence });
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}
