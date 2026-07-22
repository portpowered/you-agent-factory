import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import { projectCliManifest } from "../lib/cli-command-projection";
import { projectCliCommandControls } from "../lib/cli-control-projection";
import { loadCliManifest } from "../lib/cli-manifest-adapter";
import { StaticCliControls } from "./static-cli-controls";

function canonicalRunControls() {
  const loaded = loadCliManifest(canonicalCliManifest);
  if (loaded.status !== "ready") throw new Error("Expected ready manifest.");
  const projected = projectCliCommandControls(
    projectCliManifest(loaded).commands["you.run"],
  );
  if (projected.status !== "ready") throw new Error("Expected controls.");
  return projected.model;
}

export default {
  title: "CLI Command Explorer/Static Controls",
};

export const RunCommand = {
  render: () => (
    <main className="mx-auto max-w-3xl p-6">
      <StaticCliControls model={canonicalRunControls()} />
    </main>
  ),
};
