import { appendFile, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { prepareDataPackageCandidate } from "./public-release-package-candidate.mjs";
import { assertDevelopmentPackageAction } from "./package-development-policy.mjs";
import {
  isolatedConsumerEnvironment,
  runSmokeCommand,
} from "./public-package-install-smoke.mjs";
import { timePackagePhase } from "./package-phase-timing.mjs";
import {
  check,
  packAndVerify,
  verifyCleanConsumer,
} from "./model-provider-package.mjs";

const families = [
  {
    key: "api",
    selection: "run_api_package",
    packageName: "@you-agent-factory/api",
    directory: "api",
    packageResult: "api_package_result",
    candidateResult: "api_candidate_result",
  },
  {
    key: "packaged",
    selection: "run_packaged_factories_package",
    packageName: "@you-agent-factory/packaged-factories",
    directory: "packaged-factories",
    packageResult: "packaged_factories_package_result",
    candidateResult: "packaged_factories_candidate_result",
  },
  {
    key: "providers",
    selection: "run_model_providers_package",
    packageResult: "model_providers_package_result",
  },
];

async function prepareFamily(family, input) {
  if (family.key === "providers") {
    await runSmokeCommand("bun", ["install", "--frozen-lockfile"], {
      cwd: join(input.workspaceDirectory, "ui"),
    });
    await check(input.workspaceDirectory);
    await mkdir(input.outputDirectory, { recursive: true });
    return packAndVerify(input.workspaceDirectory, input.outputDirectory, {
      packageManager: "bun",
    });
  }
  return prepareDataPackageCandidate({
    packageName: family.packageName,
    packageDirectory: join(
      input.workspaceDirectory,
      "packages",
      family.directory,
    ),
    outputDirectory: input.outputDirectory,
    runId: input.runId,
    sourceCommit: input.sourceCommit,
    distTag: "dev",
  });
}

async function verifyFamily(family, prepared, input) {
  if (family.key === "providers") {
    return verifyCleanConsumer(input.workspaceDirectory, prepared.tarballPath, {
      packageManager: "bun",
    });
  }
  const root = await mkdtemp(join(tmpdir(), "you-selected-package-"));
  try {
    const consumer = join(root, "consumer");
    await mkdir(consumer);
    const dependencies = {
      [family.packageName]: prepared.tarballPath.replaceAll("\\", "/"),
    };
    if (family.key === "packaged")
      Object.assign(dependencies, {
        ajv: "8.20.0",
        "ajv-formats": "3.0.1",
        yaml: "2.9.0",
      });
    await writeFile(
      join(consumer, "package.json"),
      JSON.stringify({ private: true, dependencies }),
    );
    const env = await isolatedConsumerEnvironment(root);
    await runSmokeCommand("bun", ["install", "--ignore-scripts"], {
      cwd: consumer,
      env,
    });
    const input_ = {
      consumerDirectory: consumer,
      packageName: family.packageName,
      packedFiles: prepared.evidence.inventory,
      workspaceDirectory: input.workspaceDirectory,
      expectedVersion: prepared.evidence.candidateVersion,
      expectedSourceCommit: input.sourceCommit,
    };
    const module = new URL(
      family.key === "api"
        ? "./api-package-consumer.mjs"
        : "./packaged-factories-package-consumer.mjs",
      import.meta.url,
    ).href;
    const source = `import { verifyInstalledPackage } from ${JSON.stringify(module)}; await verifyInstalledPackage(${JSON.stringify(input_)});`;
    await runSmokeCommand("node", ["--input-type=module", "-e", source], {
      cwd: consumer,
      env,
    });
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

// One prepared artifact and installed check per family serve both legacy results.
// Each family settles independently; no publishing dependency exists on this path.
export async function executePackageWorkflow(
  inputs,
  {
    runId,
    workspaceDirectory = process.cwd(),
    outputDirectory = ".artifacts/selected-package-candidates",
    prepare = prepareFamily,
    verify = verifyFamily,
    log = (message) => console.error(message),
  } = {},
) {
  const outputs = Object.fromEntries(
    families.flatMap((family) =>
      [family.packageResult, family.candidateResult]
        .filter(Boolean)
        .map((name) => [name, ""]),
    ),
  );
  const errors = [];
  if (!inputs.is_ci_call) return { outputs, exitCode: 0 };
  await Promise.all(
    families
      .filter((family) => inputs[family.selection])
      .map(async (family) => {
        const candidateSelected =
          !!family.candidateResult && inputs.run_candidates === true;
        let candidateAllowed = candidateSelected;
        if (candidateSelected && !inputs.behavior_test) {
          try {
            assertDevelopmentPackageAction(
              {
                eventName: "pull_request",
                sourceCommit: inputs.source_commit,
                pullRequestHeadSha: inputs.pull_request_head_sha,
                ref: inputs.source_ref,
                repository: inputs.repository,
              },
              "dry-run",
            );
          } catch (error) {
            candidateAllowed = false;
            outputs[family.candidateResult] = "failure";
            errors.push(error);
          }
        }
        try {
          if (inputs.behavior_test) {
            outputs[family.packageResult] =
              inputs.behavior_test_failure_lane === family.key
                ? "failure"
                : "success";
            if (candidateSelected)
              outputs[family.candidateResult] =
                inputs.behavior_test_failure_lane === `${family.key}_candidate`
                  ? "failure"
                  : "success";
            return;
          }
          const input = {
            runId,
            sourceCommit: inputs.source_commit,
            workspaceDirectory: resolve(workspaceDirectory),
            outputDirectory: resolve(
              outputDirectory,
              family.directory ?? "model-providers",
            ),
          };
          const prepared = await timePackagePhase(
            `selected ${family.key} prepare`,
            () => prepare(family, input),
            { log },
          );
          await timePackagePhase(
            `selected ${family.key} installed consumer`,
            () => verify(family, prepared, input),
            { log },
          );
          outputs[family.packageResult] = "success";
          if (candidateAllowed) outputs[family.candidateResult] = "success";
        } catch (error) {
          outputs[family.packageResult] = "failure";
          if (candidateSelected) outputs[family.candidateResult] = "failure";
          errors.push(error);
        }
      }),
  );
  for (const error of errors) log(error.message);
  return {
    outputs,
    exitCode: Object.values(outputs).includes("failure") ? 1 : 0,
  };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const inputs = JSON.parse(process.env.PACKAGE_WORKFLOW_INPUTS);
  executePackageWorkflow(inputs, { runId: process.env.RUN_ID })
    .then(async ({ outputs, exitCode }) => {
      if (process.env.GITHUB_OUTPUT)
        await appendFile(
          process.env.GITHUB_OUTPUT,
          Object.entries(outputs)
            .map(([name, value]) => `${name}=${value}\n`)
            .join(""),
        );
      console.log(JSON.stringify(outputs));
      process.exitCode = exitCode;
    })
    .catch((error) => {
      console.error(error.message);
      process.exitCode = 1;
    });
}
