import { runFocusedBrowserIntegration } from "./ui-integration-runner.mjs";
import {
  durableSessionRealBackendIntegrationFiles,
  durableSessionRealBackendIntegrationPhaseName,
} from "./ui-integration-targets.mjs";

runFocusedBrowserIntegration(durableSessionRealBackendIntegrationFiles, {
  phaseName: durableSessionRealBackendIntegrationPhaseName,
});
