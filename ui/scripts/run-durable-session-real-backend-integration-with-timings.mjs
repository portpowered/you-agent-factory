import {
  durableSessionRealBackendIntegrationFiles,
  durableSessionRealBackendIntegrationPhaseName,
} from "./ui-integration-targets.mjs";
import { runFocusedBrowserIntegration } from "./ui-integration-runner.mjs";

runFocusedBrowserIntegration(durableSessionRealBackendIntegrationFiles, {
  phaseName: durableSessionRealBackendIntegrationPhaseName,
});
