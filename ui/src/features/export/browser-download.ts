export interface DownloadBlobAsFileOptions {
  blob: Blob;
  filename: string;
}

export function downloadBlobAsFile({
  blob,
  filename,
}: DownloadBlobAsFileOptions): void {
  if (typeof document === "undefined") {
    throw new Error("Browser download is unavailable outside the document context.");
  }

  const downloadUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");

  anchor.download = filename;
  anchor.href = downloadUrl;
  anchor.rel = "noopener";
  anchor.style.display = "none";
  document.body.append(anchor);

  try {
    anchor.click();
  } finally {
    anchor.remove();
    URL.revokeObjectURL(downloadUrl);
  }
}

