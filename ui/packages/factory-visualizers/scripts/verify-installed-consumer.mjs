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
import { packAndVerify as packEmulator } from "../../factory-emulator/scripts/verify-package-pack.mjs";
import { packAndVerify as packReplay } from "../../factory-replay/scripts/verify-package-pack.mjs";
import { packAndVerify as packVisualizers } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const mainSource = `import { useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  FactoryEmulatorView,
  FactoryRecordingTopologyReplay,
  FactoryTimelineScrubber,
  FactoryTopologyReplay,
  WorkProgressVisualizer,
  type FactoryRecordingTopologyReplayError,
  type FactoryRecordingTopologyReplayMessages,
  type FactoryEmulatorViewProps,
  type FactoryTimelineScrubberMessages,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayError,
  type FactoryTopologyReplayProjection,
  type WorkProgressVisualizerMessages,
} from "@you-agent-factory/factory-visualizers";
import { parseFactoryRecording, type FactoryDefinition, type FactoryEvent } from "@you-agent-factory/client";
import {
  createFactoryEmulatorSession,
  parseFactoryEmulatorScenario,
  type FactoryEmulatorSession,
  type FactoryEventSink,
} from "@you-agent-factory/factory-emulator";
import {
  canonicalizeFactoryEvents,
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
  type FactoryWorkProgressProjection,
} from "@you-agent-factory/factory-replay";
import emulatorScenario from "@you-agent-factory/factory-emulator/examples/customer-support.scenario.v1.json";
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
  activeDispatches: (count) => count + " active Dispatches", annotationsHidden: "Show annotations", annotationsVisible: "Hide annotations",
  empty: "No topology", failed: "Topology failed", imageFailed: "Annotation image failed", imageLoading: "Loading annotation image",
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
  validationFailed: "The Factory recording could not be validated.",
};
const recording = parseFactoryRecording(supportPlayback);
const events = canonicalizeFactoryEvents(recording.events);
const selectedTick = events.at(-1)?.context.tick ?? 0;
const topology: FactoryTopologyReplayProjection = {
  activity: projectFactoryActivityAtTick({ events, tick: selectedTick }),
  load: projectFactoryLoadAtTick({ events, tick: selectedTick }),
  topology: projectFactoryTopologyAtTick({ events, tick: selectedTick }),
};
const reportError = (_error: FactoryTopologyReplayError) => {};
const reportRecordingError = (_error: FactoryRecordingTopologyReplayError) => {};

const emulatorFactory = {
  name: "customer-support",
  workTypes: [
    { name: "ticket", states: [{ name: "new", type: "INITIAL" }, { name: "done", type: "TERMINAL" }, { name: "failed", type: "FAILED" }] },
    { name: "audit", states: [{ name: "recorded", type: "TERMINAL" }] },
  ],
  resources: [{ name: "agent-slot", capacity: 1 }],
  workers: [{ name: "support-agent", type: "AGENT_WORKER" }],
  workstations: [{
    name: "triage", type: "AGENT_RUN", behavior: "STANDARD", worker: "support-agent",
    inputs: [{ workType: "ticket", state: "new" }],
    outputs: [{ workType: "ticket", state: "done" }, { workType: "audit", state: "recorded" }],
    onFailure: [{ workType: "ticket", state: "failed" }],
    resources: [{ name: "agent-slot", capacity: 1 }],
  }],
} satisfies FactoryDefinition;
const scenario = parseFactoryEmulatorScenario(emulatorScenario, emulatorFactory);

interface InstalledEmulatorState {
  error?: string;
  events: readonly FactoryEvent[];
  isPlaying: boolean;
  latestTick: number;
  selectedTick: number;
  submissions: number;
}

const initialEmulatorState: InstalledEmulatorState = {
  events: [], isPlaying: false, latestTick: 0, selectedTick: 0, submissions: 0,
};

function latestEventTick(events: readonly FactoryEvent[]): number {
  return events.reduce((latest, event) => Math.max(latest, event.context.tick), 0);
}

function InstalledEmulator({ id }: { id: string }) {
  const [state, setState] = useState<InstalledEmulatorState>(initialEmulatorState);
  const history = useRef<readonly FactoryEvent[]>([]);
  const session = useRef<FactoryEmulatorSession | undefined>(undefined);
  if (!session.current) {
    const sink: FactoryEventSink = {
      write: async (batch) => {
        const events = [...history.current, ...structuredClone(batch)];
        history.current = events;
        const latestTick = latestEventTick(events);
        setState((current) => ({
          ...current,
          error: undefined,
          events,
          latestTick,
          selectedTick: current.selectedTick === current.latestTick ? latestTick : current.selectedTick,
        }));
      },
    };
    session.current = createFactoryEmulatorSession({ factory: emulatorFactory, scenario, sink });
  }
  const runtime = session.current;

  useEffect(() => {
    void runtime.start().catch((error) => setState((current) => ({ ...current, error: String(error) })));
  }, [runtime]);

  const replay = useMemo<FactoryTopologyReplayProjection>(() => ({
    activity: projectFactoryActivityAtTick({ events: state.events, tick: state.selectedTick }),
    load: projectFactoryLoadAtTick({ events: state.events, tick: state.selectedTick }),
    topology: projectFactoryTopologyAtTick({ events: state.events, tick: state.selectedTick }),
  }), [state.events, state.selectedTick]);
  const workProgress = useMemo<FactoryWorkProgressProjection>(
    () => projectFactoryWorkProgressAtTick({ events: state.events, tick: state.selectedTick }),
    [state.events, state.selectedTick],
  );

  const run = async (command: () => Promise<unknown>) => {
    try {
      await command();
    } catch (error) {
      setState((current) => ({ ...current, error: error instanceof Error ? error.message : String(error), isPlaying: false }));
    }
  };
  const restart = async () => {
    runtime.reset();
    history.current = [];
    setState(initialEmulatorState);
    await run(() => runtime.start());
  };
  const submit = async () => {
    const ordinal = state.submissions + 1;
    await run(async () => {
      await runtime.submit({ name: id + "-ticket-" + ordinal, workType: "ticket", state: "new", input: "Installed submission " + ordinal });
      setState((current) => ({ ...current, submissions: ordinal }));
    });
  };
  const showLocalError = () =>
    setState((current) => ({
      ...current,
      error: "Caller-owned example error",
      isPlaying: false,
    }));
  const controls: FactoryEmulatorViewProps["controls"] = {
    formatTick: String,
    isPlaying: state.isPlaying,
    onFollowLatest: () => setState((current) => ({ ...current, selectedTick: current.latestTick })),
    onPause: () => setState((current) => ({ ...current, isPlaying: false })),
    onPlay: () => setState((current) => ({ ...current, isPlaying: true, selectedTick: current.latestTick })),
    onRestart: () => void restart(),
    onSelectTick: (selectedTick) => setState((current) => ({ ...current, isPlaying: false, selectedTick })),
    onSpeedChange: () => {},
    onStep: () => void run(() => runtime.advanceToNext()),
    runtimeStatus: { label: state.error ? "Error" : state.isPlaying ? "Playing" : "Ready", tone: state.error ? "danger" : "neutral" },
    speed: 1,
    timeline: {
      messages: timelineMessages,
      state: { earliestTick: 0, latestTick: state.latestTick, mode: state.selectedTick < state.latestTick ? "history" : "current", selectedTick: state.selectedTick, status: "available" },
    },
  };

  return <article
    aria-label={"Emulator " + id}
    data-consumer-emulator={id}
    data-error={state.error ? "true" : "false"}
    data-events={state.events.length}
    data-history={state.events.map((event) => event.id).join(",")}
    data-latest={state.latestTick}
    data-playing={state.isPlaying ? "true" : "false"}
    data-selected={state.selectedTick}
    data-submissions={state.submissions}
  >
    <h2>{"Emulator " + id}</h2>
    {state.error ? <p role="alert">{state.error}</p> : null}
    <FactoryEmulatorView
      controls={controls}
      submission={<div><button onClick={() => void submit()} type="button">Submit Work</button><button onClick={showLocalError} type="button">Show local error</button><output aria-label="Submission count">{state.submissions}</output></div>}
      topology={{ messages: topologyMessages, state: replay.topology.nodes.length === 0 ? { status: "empty" } : { projection: replay, status: "ready" } }}
      workProgress={{ formatNumber: String, messages: progressMessages, projection: workProgress }}
    />
  </article>;
}

function App() {
  return <main>
    <section aria-label="Installed emulator examples"><InstalledEmulator id="alpha" /><InstalledEmulator id="beta" /></section>
    <section aria-label="Valid packaged recording">
      <FactoryTopologyReplay messages={topologyMessages} onError={reportError} state={{ projection: topology, status: "ready" }} />
    </section>
    <FactoryTimelineScrubber formatTick={String} messages={timelineMessages} onFollowLatest={() => {}} onSelectTick={() => {}} state={{ earliestTick: 0, latestTick: 4, mode: "history", selectedTick: 2, status: "available" }} />
    <WorkProgressVisualizer formatNumber={(value) => new Intl.NumberFormat("en").format(value)} messages={progressMessages} projection={progress} />
    <section aria-label="Invalid packaged recording">
      <FactoryRecordingTopologyReplay formatNumber={String} messages={recordingMessages} onError={reportRecordingError} recording={{ schemaVersion: "factory-recording/v1", id: "invalid", title: 42, events: [] }} />
      <p>Valid packaged recording remains available</p>
    </section>
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
      "@you-agent-factory/factory-emulator": pathToFileURL(tarballs.emulator)
        .href,
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

async function readEmulatorState(target) {
  return {
    error: await target.getAttribute("data-error"),
    events: await target.getAttribute("data-events"),
    history: await target.getAttribute("data-history"),
    latest: await target.getAttribute("data-latest"),
    playing: await target.getAttribute("data-playing"),
    selected: await target.getAttribute("data-selected"),
    submissions: await target.getAttribute("data-submissions"),
  };
}

async function verifyInstalledEmulators(page) {
  const emulator = (id) => page.locator(`[data-consumer-emulator="${id}"]`);
  await page.waitForFunction(
    () =>
      [...document.querySelectorAll("[data-consumer-emulator]")].length === 2 &&
      [...document.querySelectorAll("[data-consumer-emulator]")].every(
        (element) => Number(element.getAttribute("data-events")) > 0,
      ),
  );
  const alpha = emulator("alpha");
  const beta = emulator("beta");
  const initialAlpha = await readEmulatorState(alpha);
  const initialBeta = await readEmulatorState(beta);
  if (JSON.stringify(initialAlpha) !== JSON.stringify(initialBeta))
    throw new Error(
      "installed emulator instances did not start deterministically",
    );

  await alpha.getByRole("button", { name: "Step", exact: true }).click();
  await page.waitForFunction(
    (count) =>
      Number(
        document
          .querySelector('[data-consumer-emulator="alpha"]')
          ?.getAttribute("data-events"),
      ) > Number(count),
    initialAlpha.events,
  );
  await alpha.getByRole("button", { name: "Play", exact: true }).click();
  await page
    .locator('[data-consumer-emulator="alpha"][data-playing="true"]')
    .waitFor();
  await alpha.getByRole("button", { name: "Submit Work", exact: true }).click();
  await page
    .locator('[data-consumer-emulator="alpha"][data-submissions="1"]')
    .waitFor();
  await alpha
    .getByRole("button", { name: "Show local error", exact: true })
    .click();
  await page
    .locator('[data-consumer-emulator="alpha"][data-error="true"]')
    .waitFor();
  await alpha.getByRole("slider", { name: "Selected tick" }).press("Home");
  await page.waitForFunction(
    () =>
      document
        .querySelector('[data-consumer-emulator="alpha"]')
        ?.getAttribute("data-selected") === "0",
  );
  if (
    JSON.stringify(await readEmulatorState(beta)) !==
    JSON.stringify(initialBeta)
  )
    throw new Error(
      "actions in one installed emulator changed the other instance",
    );

  await alpha.getByRole("button", { name: "Restart", exact: true }).click();
  await page.waitForFunction((expectedHistory) => {
    const element = document.querySelector('[data-consumer-emulator="alpha"]');
    return (
      element?.getAttribute("data-history") === expectedHistory &&
      element.getAttribute("data-error") === "false" &&
      element.getAttribute("data-playing") === "false" &&
      element.getAttribute("data-submissions") === "0"
    );
  }, initialAlpha.history);
  if (
    JSON.stringify(await readEmulatorState(alpha)) !==
    JSON.stringify(initialAlpha)
  )
    throw new Error(
      "targeted reset did not restore the deterministic initial emulator state",
    );
  if (
    JSON.stringify(await readEmulatorState(beta)) !==
    JSON.stringify(initialBeta)
  )
    throw new Error(
      "targeted reset changed the other installed emulator instance",
    );
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
    await verifyInstalledEmulators(page);
    for (const name of ["Factory topology", "Replay timeline", "Work progress"])
      await page.getByRole("region", { name }).first().waitFor();
    const topology = page
      .getByRole("region", { name: "Factory topology" })
      .first();
    await topology.getByLabel("worker: support-agent").waitFor();
    await topology.getByLabel("workstation: triage").waitFor();
    await page.getByText("6 total", { exact: true }).waitFor();
    const invalidRecording = page.getByRole("region", {
      name: "Invalid packaged recording",
    });
    await invalidRecording.getByRole("alert").waitFor();
    await invalidRecording
      .getByText("Topology failed", { exact: true })
      .waitFor();
    await invalidRecording
      .getByText("Valid packaged recording remains available", { exact: true })
      .waitFor();
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
    [
      "client",
      "components",
      "emulator",
      "replay",
      "visualizers",
      "consumer",
    ].map((name) => [name, path.join(temporaryRoot, name)]),
  );
  await Promise.all(Object.values(roots).map((root) => mkdir(root)));
  const client = await packClient(roots.client);
  const replay = await packReplay(roots.replay);
  const components = await packComponents({
    packDestination: roots.components,
  });
  const emulator = await packEmulator(roots.emulator);
  const visualizers = await packVisualizers(roots.visualizers);
  await writeConsumer(roots.consumer, {
    client: client.tarballPath,
    components: components.tarballPath,
    emulator: emulator.tarballPath,
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
    "[factory-visualizers-consumer] installed, typechecked, built, and rendered isolated static and interactive consumers from packaged APIs\n",
  );
} finally {
  await rm(temporaryRoot, { force: true, recursive: true });
}
