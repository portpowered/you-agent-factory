import { existsSync, statSync } from "node:fs";
import path from "node:path";

const sourceExtensions = [".tsx", ".ts", ".jsx", ".js"];

function resolveExistingFile(candidatePath) {
  if (existsSync(candidatePath) && statSync(candidatePath).isFile()) {
    return candidatePath;
  }

  return null;
}

function resolveDirectoryIndex(directoryPath) {
  if (!existsSync(directoryPath) || !statSync(directoryPath).isDirectory()) {
    return null;
  }

  for (const extension of sourceExtensions) {
    const indexPath = path.join(directoryPath, `index${extension}`);
    const resolved = resolveExistingFile(indexPath);
    if (resolved) {
      return resolved;
    }
  }

  return directoryPath;
}

export function resolveRelativeImport(specifier, filePath) {
  if (!specifier.startsWith(".")) {
    return null;
  }

  const basePath = path.resolve(path.dirname(filePath), specifier);
  const extension = path.extname(basePath);

  if (extension.length > 0) {
    return resolveExistingFile(basePath) ?? basePath;
  }

  for (const candidateExtension of sourceExtensions) {
    const resolved = resolveExistingFile(`${basePath}${candidateExtension}`);
    if (resolved) {
      return resolved;
    }
  }

  const directoryIndexPath = resolveDirectoryIndex(basePath);
  if (directoryIndexPath) {
    return directoryIndexPath;
  }

  return basePath;
}
