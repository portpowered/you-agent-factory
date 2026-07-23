import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import {
  loadCliManifest,
  loadingCliManifest,
} from "../lib/cli-manifest-adapter";
import { StaticCliCommandExplorer } from "./static-cli-command-explorer";

export default {
  title: "CLI Command Explorer/Command Explorer",
};

export const Loading = {
  render: () => <StaticCliCommandExplorer state={loadingCliManifest()} />,
};

export const InvalidContract = {
  render: () => (
    <StaticCliCommandExplorer
      state={{
        status: "invalid-contract",
        diagnostics: [
          {
            code: "missing_field",
            path: ["rootPath"],
            message: "Expected required field rootPath.",
          },
        ],
      }}
    />
  ),
};

export const UnsupportedVersion = {
  render: () => (
    <StaticCliCommandExplorer
      state={{
        status: "unsupported-version",
        receivedVersion: "2.0.0",
        supportedVersions: ["1.0.0"],
      }}
    />
  ),
};

export const Ready = {
  render: () => (
    <main className="mx-auto max-w-7xl p-6">
      <StaticCliCommandExplorer state={loadCliManifest(canonicalCliManifest)} />
    </main>
  ),
};
