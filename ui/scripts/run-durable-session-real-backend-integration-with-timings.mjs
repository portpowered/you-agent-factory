import {
  buildRealBackendBrowserHarness,
  realBackendHarnessArtifactEnvironmentVariable,
} from "../integration/browser-test-harness.mjs";
import { runFocusedBrowserIntegration } from "./ui-integration-runner.mjs";
import {
  durableSessionRealBackendIntegrationFiles,
  durableSessionRealBackendIntegrationPhaseName,
} from "./ui-integration-targets.mjs";

async function main() {
  const artifact = await buildRealBackendBrowserHarness();
  const previousArtifactPath =
    process.env[realBackendHarnessArtifactEnvironmentVariable];
  process.env[realBackendHarnessArtifactEnvironmentVariable] =
    artifact.artifactPath;

  try {
    runFocusedBrowserIntegration(durableSessionRealBackendIntegrationFiles, {
      exitOnFailure: false,
      phaseName: durableSessionRealBackendIntegrationPhaseName,
    });
  } finally {
    if (previousArtifactPath === undefined) {
      delete process.env[realBackendHarnessArtifactEnvironmentVariable];
    } else {
      process.env[realBackendHarnessArtifactEnvironmentVariable] =
        previousArtifactPath;
    }
    await artifact.cleanup();
  }
}

main().catch((error) => {
  console.error(
    `[real-backend-browser-harness] ${error instanceof Error ? error.message : String(error)}`,
  );
  process.exitCode = 1;
});
