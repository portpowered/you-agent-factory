import { execFile } from "node:child_process";
import { createReadStream } from "node:fs";
import { access, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { packAndVerify } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const uiRoot = path.resolve(packageRoot, "..", "..");
const require = createRequire(path.join(uiRoot, "package.json"));
const { chromium } = require("playwright");

const VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

function consumerManifest(tarballPath) {
  return {
    name: "components-installed-consumer",
    private: true,
    type: "module",
    dependencies: {
      "@you-agent-factory/components": pathToFileURL(tarballPath).href,
      react: "19.2.0",
      "react-dom": "19.2.0",
    },
    devDependencies: {
      "@tailwindcss/vite": "4.2.2",
      "@types/node": "24.0.0",
      "@types/react": "19.2.2",
      "@types/react-dom": "19.2.2",
      tailwindcss: "4.2.2",
      typescript: "5.9.3",
      vite: "7.1.7",
    },
  };
}

const CONSUMER_FILES = {
  "index.html": `<!doctype html>
<html lang="en" data-color-palette="factory-light">
  <head><meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" /><title>Installed components consumer</title></head>
  <body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body>
</html>
`,
  "src/styles.css": `@import "tailwindcss";
@import "@you-agent-factory/components/styles.css";
@source "../node_modules/@you-agent-factory/components/dist";

* { box-sizing: border-box; }
html, body { margin: 0; min-width: 0; }
body { background: var(--color-background); color: var(--color-on-surface); font-family: system-ui, sans-serif; }
main { display: grid; gap: 1rem; margin: 0 auto; max-width: 72rem; padding: 1rem; width: 100%; }
.consumer-grid { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr)); min-width: 0; }
.consumer-stack { display: grid; gap: .75rem; min-width: 0; }
.consumer-chart { min-height: 16rem; min-width: 0; }
.consumer-graph { min-height: 13rem; min-width: 0; padding: 1rem; }
.consumer-node { min-height: 9rem; }
.consumer-topology { height: 22rem; min-width: 0; }
`,
  "src/main.tsx": `import React, { useState, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { COMPONENTS_PACKAGE_NAME } from "@you-agent-factory/components";
import { Button, Heading, Text, type ButtonProps } from "@you-agent-factory/components/primitives";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { AlertPanel, AlertPanelText, AlertPanelTitle } from "@you-agent-factory/components/feedback";
import { ChartContainer, type ChartConfig } from "@you-agent-factory/components/charts";
import { GraphViewportSurface, type GraphEdgeWaypoint } from "@you-agent-factory/components/graphs";
import { FactoryTopologyReplay, type FactoryTopologyReplayMessages, type FactoryTopologyReplayProjection } from "@you-agent-factory/components/visualizers";
import { COMPONENTS_CATEGORY as ICONS_CATEGORY, type ComponentsCategory as IconsCategory } from "@you-agent-factory/components/icons";
import { cn } from "@you-agent-factory/components/utilities";
import { COMPONENTS_CATEGORY as TOKENS_CATEGORY, type ComponentsCategory as TokensCategory } from "@you-agent-factory/components/tokens";
import "./styles.css";

const chartData = [18, 42, 31, 64];
const chartConfig: ChartConfig = { throughput: { color: "var(--color-primary)", label: "Throughput" } };
const graphRoute: GraphEdgeWaypoint[] = [{ x: 20, y: 70 }, { x: 80, y: 70 }, { x: 220, y: 70 }, { x: 280, y: 70 }];
const categories: [IconsCategory, TokensCategory] = [ICONS_CATEGORY, TOKENS_CATEGORY];
const topologyMessages: FactoryTopologyReplayMessages = {
  activeDispatchCount: (count) => String(count) + " active Dispatches",
  connectionLabel: () => "Work type state",
  failedDescription: "Prepared topology failed.",
  failedTitle: "Topology unavailable",
  handleLabel: (id, role) => role + " " + id,
  inactiveDispatch: "No active Dispatches",
  nodeKind: (kind) => kind,
  occupancy: (occupied, capacity) => occupied + " of " + capacity + " occupied",
  occupancyUnavailable: "Occupancy unavailable",
  regionLabel: "Installed Factory topology",
  selectedNode: "Selected",
  selectedTick: (tick) => "Logical tick " + tick,
  workStateCount: (count) => count + " Work",
};
const topologyProjection: FactoryTopologyReplayProjection = {
  activity: { activeDispatches: [], activeWorkstationIds: [], issues: [], resourceOccupancy: [], selectedTick: 7 },
  topology: {
    connections: [{
      id: "type-to-state", kind: "work-type-state",
      source: { handleId: "work-type-state-source", nodeId: "work-type:task" },
      target: { handleId: "work-type-state-target", nodeId: "work-state:ready" },
    }],
    issues: [],
    nodes: [
      { entityId: "task", handles: [{ id: "work-type-state-source", role: "source" }], id: "work-type:task", kind: "work-type", label: "Task" },
      { entityId: "ready", handles: [{ id: "work-type-state-target", role: "target" }], id: "work-state:ready", kind: "work-state", label: "Ready" },
    ],
    selectedTick: 7,
  },
  workStateCounts: [{ count: 3, nodeId: "work-state:ready" }],
};

function Feedback({ children, semantic }: { children?: ReactNode; semantic: "loading" | "empty" | "error" | "success" }) {
  return (
    <AlertPanel semantic={semantic}>
      {children ?? <AlertPanelText>{semantic === "loading" ? "Loading caller data" : "No caller data available"}</AlertPanelText>}
    </AlertPanel>
  );
}

function App() {
  const [activations, setActivations] = useState(0);
  const buttonProps: ButtonProps = { onClick: () => setActivations((value) => value + 1) };
  const points = chartData.map((value, index) => String(index * 100 + 20) + "," + String(180 - value * 2)).join(" ");
  const graphPath = graphRoute.map((point, index) => (index === 0 ? "M " : "L ") + point.x + " " + point.y).join(" ");

  return (
    <main data-categories={categories.join(",")} data-package={COMPONENTS_PACKAGE_NAME}>
      <Heading as="h1">Installed package visualizer</Heading>
      <Text>Rendered entirely from the registry-format tarball.</Text>
      <Button {...buttonProps}>Activate visualizer</Button>
      <output aria-live="polite">Activations: {activations}</output>

      <section aria-label="Feedback states" className="consumer-grid">
        <Feedback semantic="loading" />
        <Feedback semantic="empty" />
        <Feedback semantic="error"><AlertPanelTitle>Could not load caller data</AlertPanelTitle><AlertPanelText>Retry is available from the host application.</AlertPanelText></Feedback>
        <Feedback semantic="success"><AlertPanelTitle>Caller data loaded</AlertPanelTitle><AlertPanelText>Four points are ready to visualize.</AlertPanelText></Feedback>
      </section>

      <section className="consumer-grid">
        <SurfacePanel className="consumer-stack" aria-label="Caller-provided chart">
          <Heading as="h2">Throughput chart</Heading>
          <ChartContainer className="consumer-chart" config={chartConfig} title="Caller throughput across four intervals">
            <svg aria-labelledby="chart-title" preserveAspectRatio="none" viewBox="0 0 340 200">
              <title id="chart-title">Throughput values 18, 42, 31, and 64</title>
              <polyline fill="none" points={points} stroke="var(--color-primary)" strokeWidth="6" />
            </svg>
          </ChartContainer>
        </SurfacePanel>

        <GraphViewportSurface aria-label="Caller-provided graph" className={cn("consumer-graph", "bg-surface-container-low")}>
          <article aria-label="Transform caller data node" className="consumer-node rounded-lg border border-outline bg-surface p-3">
            <Heading as="h2">Transform data</Heading>
            <Text>Caller-owned input and output handles.</Text>
            <svg aria-label="Caller graph connection" role="img" viewBox="0 0 300 140"><path d={graphPath} fill="none" stroke="var(--color-primary)" strokeWidth="5" /></svg>
          </article>
        </GraphViewportSurface>
      </section>

      <FactoryTopologyReplay className="consumer-topology" messages={topologyMessages} projection={topologyProjection} />
    </main>
  );
}

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("Missing consumer root element");
createRoot(rootElement).render(<App />);
`,
  "tsconfig.json": `${JSON.stringify(
    {
      compilerOptions: {
        jsx: "react-jsx",
        lib: ["ES2023", "ESNext.Disposable", "DOM", "DOM.Iterable"],
        module: "ESNext",
        moduleResolution: "Bundler",
        noEmit: true,
        strict: true,
        target: "ES2022",
        types: ["vite/client", "node"],
      },
      include: ["src", "vite.config.ts"],
    },
    null,
    2,
  )}\n`,
  "vite.config.ts": `import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
export default defineConfig({ plugins: [tailwindcss()] });
`,
};

async function writeConsumer(consumerRoot, tarballPath) {
  await writeFile(
    path.join(consumerRoot, "package.json"),
    `${JSON.stringify(consumerManifest(tarballPath), null, 2)}\n`,
  );
  await Promise.all(
    Object.entries(CONSUMER_FILES).map(async ([relativePath, contents]) => {
      const outputPath = path.join(consumerRoot, relativePath);
      await mkdir(path.dirname(outputPath), { recursive: true });
      await writeFile(outputPath, contents);
    }),
  );
}

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

async function runPhase(label, executable, args, cwd) {
  try {
    return await execFileAsync(executable, args, {
      cwd,
      env: { ...process.env, CI: "1" },
      maxBuffer: 20 * 1024 * 1024,
    });
  } catch (error) {
    throw new Error(
      `[components-installed-consumer] ${label} failed\n${error.stderr?.trim() || error.stdout?.trim() || error.message}`,
      { cause: error },
    );
  }
}

async function buildConsumer(consumerRoot) {
  const npm = await npmCommand();
  await runPhase(
    "installation",
    npm.executable,
    [...npm.args, "install", "--ignore-scripts", "--no-audit", "--no-fund"],
    consumerRoot,
  );
  const typescriptBin = path.join(
    consumerRoot,
    "node_modules",
    "typescript",
    "bin",
    "tsc",
  );
  const viteBin = path.join(
    consumerRoot,
    "node_modules",
    "vite",
    "bin",
    "vite.js",
  );
  await runPhase(
    "type resolution",
    process.execPath,
    [typescriptBin, "--pretty", "false"],
    consumerRoot,
  );
  await runPhase(
    "production bundle",
    process.execPath,
    [viteBin, "build"],
    consumerRoot,
  );
  await Promise.all([
    access(path.join(consumerRoot, "dist", "index.html")),
    access(
      path.join(
        consumerRoot,
        "node_modules",
        "@you-agent-factory",
        "components",
        "dist",
        "index.js",
      ),
    ),
  ]);
}

function contentType(filePath) {
  if (filePath.endsWith(".css")) return "text/css; charset=utf-8";
  if (filePath.endsWith(".js")) return "text/javascript; charset=utf-8";
  if (filePath.endsWith(".svg")) return "image/svg+xml";
  return "text/html; charset=utf-8";
}

async function startBuiltConsumer(distRoot) {
  const resolvedDistRoot = path.resolve(distRoot);
  const server = createServer(async (request, response) => {
    try {
      const pathname = new URL(request.url ?? "/", "http://127.0.0.1").pathname;
      const relativePath =
        pathname === "/" ? "index.html" : pathname.replace(/^\//, "");
      let filePath = path.resolve(resolvedDistRoot, relativePath);
      if (!filePath.startsWith(`${resolvedDistRoot}${path.sep}`)) {
        response.writeHead(403).end();
        return;
      }
      try {
        await access(filePath);
      } catch {
        filePath = path.join(resolvedDistRoot, "index.html");
      }
      response.writeHead(200, { "Content-Type": contentType(filePath) });
      createReadStream(filePath).pipe(response);
    } catch (error) {
      response
        .writeHead(500)
        .end(error instanceof Error ? error.message : "server error");
    }
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("Consumer server did not bind a TCP port");
  return { server, url: `http://127.0.0.1:${address.port}` };
}

function assertViewportSemantics(semantics, viewportLabel) {
  if (!semantics.cssPrimary)
    throw new Error(`${viewportLabel}: package CSS tokens did not load`);
  if (semantics.minButtonHeight < 40)
    throw new Error(`${viewportLabel}: package button styles did not compile`);
  if (semantics.alerts < 1 || semantics.statuses < 3 || semantics.busy !== 1) {
    throw new Error(
      `${viewportLabel}: feedback semantics were incomplete: ${JSON.stringify(semantics)}`,
    );
  }
  if (semantics.overflow > 4)
    throw new Error(
      `${viewportLabel}: horizontal overflow was ${semantics.overflow}px`,
    );
  if (semantics.topologyState !== "ready")
    throw new Error(`${viewportLabel}: installed topology did not render`);
}

async function verifyBrowser(distRoot) {
  const { server, url } = await startBuiltConsumer(distRoot);
  let browser;
  const failures = [];
  let page;
  try {
    browser = await chromium.launch({ headless: true });
    page = await browser.newPage();
    page.on("console", (message) => {
      if (message.type() === "error")
        failures.push(`console: ${message.text()}`);
    });
    page.on("pageerror", (error) => failures.push(`page: ${error.message}`));

    for (const viewport of VIEWPORTS) {
      await page.setViewportSize(viewport);
      await page.goto(url, { waitUntil: "networkidle" });
      await page
        .getByRole("heading", { name: "Installed package visualizer" })
        .waitFor();
      await page
        .getByRole("img", { name: "Caller throughput across four intervals" })
        .waitFor();
      await page
        .getByRole("region", { name: "Caller-provided graph" })
        .waitFor();
      await page
        .getByRole("region", { name: "Installed Factory topology" })
        .waitFor();
      await page.getByText("3 Work", { exact: true }).waitFor();

      const semantics = await page.evaluate(() => {
        const rootStyles = getComputedStyle(document.documentElement);
        const button = document.querySelector("button");
        const buttonStyles = button ? getComputedStyle(button) : null;
        return {
          alerts: document.querySelectorAll('[role="alert"]').length,
          busy: document.querySelectorAll('[role="status"][aria-busy="true"]')
            .length,
          cssPrimary: rootStyles.getPropertyValue("--color-primary").trim(),
          minButtonHeight: buttonStyles
            ? Number.parseFloat(buttonStyles.minHeight)
            : 0,
          overflow:
            document.documentElement.scrollWidth -
            document.documentElement.clientWidth,
          statuses: document.querySelectorAll('[role="status"]').length,
          topologyState: document
            .querySelector('[aria-label="Installed Factory topology"]')
            ?.getAttribute("data-factory-topology-state"),
        };
      });
      assertViewportSemantics(semantics, viewport.label);
    }

    const button = page.getByRole("button", { name: "Activate visualizer" });
    await button.focus();
    const focusVisible = await button.evaluate((element) => {
      const styles = getComputedStyle(element);
      return (
        element.matches(":focus-visible") &&
        (styles.outlineStyle !== "none" || styles.boxShadow !== "none")
      );
    });
    if (!focusVisible)
      throw new Error("keyboard focus treatment was not visible");
    await button.press("Enter");
    await page.getByText("Activations: 1", { exact: true }).waitFor();

    const runtimeFailures = failures.filter((failure) =>
      /invalid hook call|multiple copies of react|cannot read properties|uncaught/i.test(
        failure,
      ),
    );
    if (runtimeFailures.length > 0) throw new Error(runtimeFailures.join("\n"));
    if (failures.length > 0) throw new Error(failures.join("\n"));
  } catch (error) {
    const bodyText = page
      ? await page
          .locator("body")
          .innerText()
          .catch(() => "")
      : "";
    throw new Error(
      [
        "[components-installed-consumer] browser rendering failed",
        error instanceof Error ? error.message : String(error),
        ...failures,
        bodyText
          ? `Rendered body: ${bodyText.slice(0, 1_000)}`
          : "Rendered body was empty.",
      ].join("\n"),
      { cause: error },
    );
  } finally {
    await browser?.close();
    await new Promise((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
}

export async function verifyInstalledConsumer({
  packageDirectory = packageRoot,
} = {}) {
  const temporaryRoot = await mkdtemp(
    path.join(tmpdir(), "you-components-consumer-"),
  );
  const packRoot = path.join(temporaryRoot, "pack");
  const consumerRoot = path.join(temporaryRoot, "consumer");
  await Promise.all([mkdir(packRoot), mkdir(consumerRoot)]);
  try {
    const packed = await packAndVerify({
      packageDirectory,
      packDestination: packRoot,
    });
    await writeConsumer(consumerRoot, packed.tarballPath);
    await buildConsumer(consumerRoot);
    await verifyBrowser(path.join(consumerRoot, "dist"));
    return {
      packageName: packed.packageName,
      viewports: VIEWPORTS.map(({ label }) => label),
    };
  } finally {
    await rm(temporaryRoot, { force: true, recursive: true });
  }
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  try {
    const result = await verifyInstalledConsumer();
    process.stdout.write(
      `[components-installed-consumer] verified ${result.packageName} at ${result.viewports.join(" and ")} viewports\n`,
    );
  } catch (error) {
    process.stderr.write(
      `${error instanceof Error ? error.message : String(error)}\n`,
    );
    process.exitCode = 1;
  }
}
