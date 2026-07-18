import { execFileSync } from "node:child_process";
import { copyFileSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import process from "node:process";

const repositoryRoot = resolve(import.meta.dirname, "..");
const uiRoot = join(repositoryRoot, "ui");
const inputPath = join(repositoryRoot, "api", "openapi.yaml");
const outputPath = join(
  repositoryRoot,
  "packages",
  "client",
  "src",
  "generated",
  "openapi.ts",
);
const recordingSchemaPath = join(
  repositoryRoot,
  "packages",
  "api",
  "generated",
  "schemas",
  "factory-recording.schema.json",
);
const clientRecordingSchemaPath = join(
  repositoryRoot,
  "packages",
  "client",
  "src",
  "generated",
  "factory-recording.schema.json",
);

function generate(destination) {
  execFileSync(
    process.execPath,
    ["scripts/generate-openapi-types.mjs", inputPath, destination],
    { cwd: uiRoot, stdio: "inherit" },
  );
}

function checkFreshness() {
  const temporaryDirectory = mkdtempSync(join(tmpdir(), "factory-client-"));
  const candidatePath = join(temporaryDirectory, "openapi.ts");

  try {
    generate(candidatePath);
    const committed = readFileSync(outputPath);
    const candidate = readFileSync(candidatePath);
    if (!committed.equals(candidate)) {
      console.error(
        "[client] Generated OpenAPI types are stale. Run `make generate-client`.",
      );
      process.exitCode = 1;
      return;
    }
    if (
      !readFileSync(clientRecordingSchemaPath).equals(
        readFileSync(recordingSchemaPath),
      )
    ) {
      console.error(
        "[client] Generated Factory Recording schema is stale. Run `make generate-client`.",
      );
      process.exitCode = 1;
      return;
    }
    console.log("[client] Generated OpenAPI types are fresh.");
  } finally {
    rmSync(temporaryDirectory, { recursive: true, force: true });
  }
}

if (process.argv.includes("--check")) {
  checkFreshness();
} else {
  generate(outputPath);
  copyFileSync(recordingSchemaPath, clientRecordingSchemaPath);
  console.log("[client] Generated OpenAPI types updated.");
}
