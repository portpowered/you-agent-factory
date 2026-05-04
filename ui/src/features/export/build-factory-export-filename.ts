const EXPORT_NAME_FALLBACK = "infinite-you";

export function buildFactoryExportFilename(factoryName: string): string {
  const slug = factoryName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

  return `${slug || EXPORT_NAME_FALLBACK}.png`;
}
