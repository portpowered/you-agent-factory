import { createHash } from 'node:crypto';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const distRoot = fileURLToPath(new URL('../dist/', import.meta.url));
const distStampGoPath = fileURLToPath(new URL('../dist_stamp.go', import.meta.url));
const textExtensions = new Set(['.css', '.html', '.js']);

async function* walk(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    const entryPath = join(directory, entry.name);
    if (entry.isDirectory()) {
      yield* walk(entryPath);
      continue;
    }
    yield entryPath;
  }
}

function hasTextExtension(path) {
  return [...textExtensions].some((extension) => path.endsWith(extension));
}

function normalizeBuiltText(path, source) {
  let normalized = source.replace(/[ \t]+(?=\r?\n)/g, '');

  if (path.endsWith('.js')) {
    normalized = normalized.replace(/fileName:"[^"]+"/g, 'fileName:"[stripped]"');
  }

  return normalized;
}

function distRelativePath(path) {
  return relative(distRoot, path).split(sep).join('/');
}

const distHash = createHash('sha256');

for await (const path of walk(distRoot)) {
  if (!hasTextExtension(path)) {
    continue;
  }

  const source = await readFile(path, 'utf8');
  const normalized = normalizeBuiltText(path, source);
  if (normalized !== source) {
    await writeFile(path, normalized);
  }
  distHash.update(`${distRelativePath(path)}\n${normalized}\n`);
}

await writeFile(
  distStampGoPath,
  `package ui

// distBuildStamp keeps Go's build cache aligned with embedded dist asset changes.
const distBuildStamp = "${distHash.digest('hex')}"
`,
);
