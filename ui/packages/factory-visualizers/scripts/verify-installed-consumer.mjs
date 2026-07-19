import { execFile } from "node:child_process";
import { createReadStream } from "node:fs";
import { access, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { packAndVerify as packClient } from "../../client/scripts/verify-package-pack.mjs";
import { packAndVerify as packComponents } from "../../components/scripts/verify-package-pack.mjs";
import { packAndVerify as packReplay } from "../../factory-replay/scripts/verify-package-pack.mjs";
import { packAndVerify as packVisualizers } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const mainSource = `import { createRoot } from "react-dom/client";
import {
  FactoryRecordingTopologyReplay,
  FactoryTimelineScrubber,
  FactoryTopologyReplay,
  WorkProgressVisualizer,
  type FactoryRecordingTopologyReplayMessages,
  type FactoryTimelineScrubberMessages,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
  type FactoryVisualizerError,
  type WorkProgressVisualizerMessages,
} from "@you-agent-factory/factory-visualizers";
import type { FactoryWorkProgressProjection } from "@you-agent-factory/factory-replay";
import supportPlayback from "@you-agent-factory/factory-visualizers/examples/support-playback.factory-recording.v1.json";
import "@you-agent-factory/components/styles.css";
import "@you-agent-factory/factory-visualizers/styles.css";
import "./styles.css";

const progress: FactoryWorkProgressProjection = {
  active: [{ id: "work-active" }],
  completed: [{ id: "work-completed-1" }, { id: "work-completed-2" }],
  counts: { active: 1, completed: 2, failed: 0, queued: 3, unclassified: 0 },
  failed: [],
  queued: [{ id: "work-queued-1" }, { id: "work-queued-2" }, { id: "work-queued-3" }],
  selectedTick: 4,
  total: 6,
  unclassified: [],
};
const category = (name: string) => ({ plural: (count: string) => count + " " + name, singular: (count: string) => count + " " + name });
const progressMessages: WorkProgressVisualizerMessages = {
  categories: { active: category("active"), completed: category("completed"), failed: category("failed"), queued: category("queued"), unclassified: category("unclassified") },
  empty: "No Work", regionLabel: "Work progress", title: "Work progress", total: (count) => count + " total",
};
const timelineMessages: FactoryTimelineScrubberMessages = {
  alreadyFollowingLatest: "Already current", currentMode: "Current", disabled: "Disabled", followLatest: "Follow latest",
  historyMode: "History", position: (selected, latest) => selected + " of " + latest, regionLabel: "Replay timeline",
  sliderLabel: "Selected tick", title: "Replay timeline", unavailable: "Unavailable",
};
const topologyMessages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) => count + " active Dispatches", empty: "No topology", failed: "Topology failed",
  inactiveDispatches: "No active Dispatch", loading: "Loading topology", nodeLabel: (kind, label) => kind + ": " + label,
  regionLabel: "Factory topology", resourceOccupancy: (occupied, capacity) => occupied + " of " + capacity + " occupied",
  resourceOccupancyUnavailable: "Occupancy unavailable", retry: "Retry", selectedNode: "Selected",
  workStateCount: (count) => count + " Work", workStateCountUnavailable: "Work unavailable",
};
const recordingMessages: FactoryRecordingTopologyReplayMessages = {
  progress: progressMessages,
  regionLabel: "Recorded Factory playback",
  selectedTick: (tick) => "Selected logical tick " + tick,
  timeline: timelineMessages,
  topology: topologyMessages,
  validationFailed: "Recording validation failed",
};
const topology: FactoryTopologyReplayProjection = {
  activity: { activeDispatchOverlays: [], activeWorkstationNodeIds: [], issues: [], resourceOccupancy: [], selectedTick: 4 },
  load: { issues: [], resourceOccupancy: [], selectedTick: 4, workStateCounts: [] },
  topology: { connections: [], issues: [], nodes: [], ok: true, selectedTick: 4 },
};
const reportError = (_error: FactoryVisualizerError) => {};

function App() {
  return <main>
    <FactoryRecordingTopologyReplay formatNumber={String} messages={recordingMessages} recording={supportPlayback} />
    <FactoryRecordingTopologyReplay defaultSelectedTick={1} formatNumber={String} messages={recordingMessages} recording={supportPlayback} />
    <FactoryTopologyReplay messages={topologyMessages} onError={reportError} state={{ projection: topology, status: "ready" }} />
    <FactoryTimelineScrubber formatTick={String} messages={timelineMessages} onFollowLatest={() => {}} onSelectTick={() => {}} state={{ earliestTick: 0, latestTick: 4, mode: "history", selectedTick: 2, status: "available" }} />
    <WorkProgressVisualizer formatNumber={(value) => new Intl.NumberFormat("en").format(value)} messages={progressMessages} projection={progress} />
  </main>;
}

const root = document.getElementById("root");
if (!root) throw new Error("Missing consumer root");
createRoot(root).render(<App />);
`;

async function npmCommand() {
  if (process.platform !== "win32") return { args: [], executable: "npm" };
  const { stdout } = await execFileAsync("where.exe", ["npm.cmd"]);
  const command = stdout.trim().split(/\r?\n/, 1)[0];
  return {
    args: [
      path.join(
        path.dirname(command),
        "node_modules",
        "npm",
        "bin",
        "npm-cli.js",
      ),
    ],
    executable: process.execPath,
  };
}

async function run(label, executable, args, cwd) {
  try {
    return await execFileAsync(executable, args, {
      cwd,
      env: { ...process.env, CI: "1" },
      maxBuffer: 20 * 1024 * 1024,
    });
  } catch (error) {
    throw new Error(
      `[factory-visualizers-consumer] ${label} failed\n${error.stderr?.trim() || error.stdout?.trim() || error.message}`,
      { cause: error },
    );
  }
}

async function writeConsumer(root, tarballs) {
  const manifest = {
    name: "factory-visualizers-installed-consumer",
    private: true,
    type: "module",
    dependencies: {
      "@you-agent-factory/client": pathToFileURL(tarballs.client).href,
      "@you-agent-factory/components": pathToFileURL(tarballs.components).href,
      "@you-agent-factory/factory-replay": pathToFileURL(tarballs.replay).href,
      "@you-agent-factory/factory-visualizers": pathToFileURL(
        tarballs.visualizers,
      ).href,
      react: "19.2.0",
      "react-dom": "19.2.0",
    },
    devDependencies: {
      "@types/react": "19.2.2",
      "@types/react-dom": "19.2.2",
      typescript: "5.9.3",
      vite: "7.1.7",
    },
  };
  const files = {
    "package.json": `${JSON.stringify(manifest, null, 2)}\n`,
    "index.html":
      '<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Visualizer consumer</title></head><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>\n',
    "src/main.tsx": mainSource,
    "src/styles.css":
      "* { box-sizing: border-box; } html, body { margin: 0; } main { display: grid; gap: 1rem; margin: auto; max-width: 72rem; padding: 1rem; } .factory-topology-replay { min-height: 18rem; }\n",
    "tsconfig.json": `${JSON.stringify(
      {
        compilerOptions: {
          jsx: "react-jsx",
          lib: ["ES2022", "DOM", "DOM.Iterable"],
          module: "ESNext",
          moduleResolution: "Bundler",
          noEmit: true,
          resolveJsonModule: true,
          strict: true,
          target: "ES2022",
          types: ["vite/client"],
        },
        include: ["src"],
      },
      null,
      2,
    )}\n`,
  };
  await Promise.all(
    Object.entries(files).map(async ([relative, contents]) => {
      const output = path.join(root, relative);
      await mkdir(path.dirname(output), { recursive: true });
      await writeFile(output, contents);
    }),
  );
}

async function startServer(distRoot) {
  const resolvedRoot = path.resolve(distRoot);
  const server = createServer(async (request, response) => {
    const pathname = new URL(request.url ?? "/", "http://127.0.0.1").pathname;
    const relative =
      pathname === "/" ? "index.html" : pathname.replace(/^\//, "");
    let file = path.resolve(resolvedRoot, relative);
    if (!file.startsWith(`${resolvedRoot}${path.sep}`))
      return response.writeHead(403).end();
    try {
      await access(file);
    } catch {
      file = path.join(resolvedRoot, "index.html");
    }
    response.writeHead(200, {
      "Content-Type": file.endsWith(".js")
        ? "text/javascript"
        : file.endsWith(".css")
          ? "text/css"
          : "text/html",
    });
    createReadStream(file).pipe(response);
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("Consumer server did not bind");
  return { server, url: `http://127.0.0.1:${address.port}` };
}

async function verifyBrowser(distRoot) {
  const { chromium } = await import(
    pathToFileURL(
      path.resolve(
        packageRoot,
        "..",
        "..",
        "node_modules",
        "playwright",
        "index.mjs",
      ),
    ).href
  );
  const { server, url } = await startServer(distRoot);
  let browser;
  try {
    browser = await chromium.launch({ headless: true });
    const page = await browser.newPage({
      viewport: { height: 900, width: 1200 },
    });
    const failures = [];
    page.on("console", (message) => {
      if (message.type() === "error") failures.push(message.text());
    });
    page.on("pageerror", (error) => failures.push(error.message));
    await page.goto(url, { waitUntil: "networkidle" });
    for (const name of ["Factory topology", "Replay timeline", "Work progress"])
      await page.getByRole("region", { name }).first().waitFor();
    const recordings = page.getByRole("region", {
      name: "Recorded Factory playback",
    });
    if ((await recordings.count()) !== 2)
      throw new Error("expected current and historical recording examples");
    await recordings.nth(0).waitFor();
    if ((await recordings.nth(0).getAttribute("data-selected-tick")) !== "2")
      throw new Error("installed current recording did not select tick 2");
    if ((await recordings.nth(1).getAttribute("data-selected-tick")) !== "1")
      throw new Error("installed historical recording did not select tick 1");
    await page.getByText("6 total", { exact: true }).waitFor();
    if (failures.length > 0) throw new Error(failures.join("\n"));
  } finally {
    await browser?.close();
    await new Promise((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
}

const temporaryRoot = await mkdtemp(
  path.join(tmpdir(), "you-visualizers-consumer-"),
);
try {
  const roots = Object.fromEntries(
    ["client", "components", "replay", "visualizers", "consumer"].map(
      (name) => [name, path.join(temporaryRoot, name)],
    ),
  );
  await Promise.all(Object.values(roots).map((root) => mkdir(root)));
  const client = await packClient(roots.client);
  const replay = await packReplay(roots.replay);
  const components = await packComponents({
    packDestination: roots.components,
  });
  const visualizers = await packVisualizers(roots.visualizers);
  await writeConsumer(roots.consumer, {
    client: client.tarballPath,
    components: components.tarballPath,
    replay: replay.tarballPath,
    visualizers: visualizers.tarballPath,
  });
  const npm = await npmCommand();
  await run(
    "installation",
    npm.executable,
    [...npm.args, "install", "--ignore-scripts", "--no-audit", "--no-fund"],
    roots.consumer,
  );
  await run(
    "typecheck",
    process.execPath,
    [
      path.join(roots.consumer, "node_modules", "typescript", "bin", "tsc"),
      "--pretty",
      "false",
    ],
    roots.consumer,
  );
  await run(
    "production build",
    process.execPath,
    [
      path.join(roots.consumer, "node_modules", "vite", "bin", "vite.js"),
      "build",
    ],
    roots.consumer,
  );
  await verifyBrowser(path.join(roots.consumer, "dist"));
  process.stdout.write(
    "[factory-visualizers-consumer] installed, typechecked, built, and rendered current and historical packaged recordings\n",
  );
} finally {
  await rm(temporaryRoot, { force: true, recursive: true });
}
