import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import { projectCliManifest } from "./cli-command-projection";
import {
  projectCliCommandControls,
  validateCliControlValues,
} from "./cli-control-projection";
import { loadCliManifest } from "./cli-manifest-adapter";

function commandControls(commandId: string) {
  const loaded = loadCliManifest(canonicalCliManifest);
  expect(loaded.status).toBe("ready");
  if (loaded.status !== "ready") throw new Error("Expected ready manifest.");
  const command = projectCliManifest(loaded).commands[commandId];
  const projected = projectCliCommandControls(command);
  expect(projected.status).toBe("ready");
  if (projected.status !== "ready") throw new Error("Expected controls.");
  return projected.model;
}

describe("static CLI control projection", () => {
  it("maps booleans, choices, scalars, required and repeated inputs from manifest metadata", () => {
    const root = commandControls("you");
    const docs = commandControls("you.docs");
    const create = commandControls("you.factory.create");
    const run = commandControls("you.run");

    expect(
      root.controls.find(({ inputId }) => inputId === "you.flag.debug"),
    ).toMatchObject({ kind: "boolean", label: "--debug", defaultValue: false });
    expect(
      docs.controls.find(({ inputId }) => inputId === "you.docs.arg.0"),
    ).toMatchObject({
      kind: "choice",
      required: false,
      choices: expect.arrayContaining(["agents", "run"]),
    });
    expect(
      create.controls.find(
        ({ inputId }) => inputId === "you.factory.create.arg.0",
      ),
    ).toMatchObject({ kind: "text", required: true });
    expect(
      run.controls.find(({ inputId }) => inputId === "you.run.arg.0"),
    ).toMatchObject({
      kind: "repeated",
      cardinality: { minimum: 0, maximum: null },
    });
    expect(
      run.controls.find(({ inputId }) => inputId === "you.run.flag.verbose"),
    ).toMatchObject({ kind: "boolean", inherited: true });
  });

  it("fails explicitly for an unsupported input kind", () => {
    const run = commandControls("you.run");
    const command = {
      id: "unsupported",
      effectiveInputs: [
        {
          ...run.controls[0],
          id: "unsupported",
          kind: "flag",
          inherited: false,
          source: {
            commandId: "unsupported",
            inputId: "unsupported",
            scope: "local",
          },
          manifestInput: {
            id: "unsupported",
            long: "unsupported",
            valueType: "duration",
            required: false,
            default: "",
          },
        },
      ],
      relationships: [],
    } as unknown as Parameters<typeof projectCliCommandControls>[0];

    expect(projectCliCommandControls(command)).toEqual({
      status: "unsupported-input",
      inputId: "unsupported",
      valueType: "duration",
    });
  });

  it("reports deterministic cardinality and relationship violations on affected inputs", () => {
    const create = commandControls("you.factory.create");
    expect(validateCliControlValues(create, {})).toContainEqual({
      code: "cardinality",
      inputId: "you.factory.create.arg.0",
      relatedInputIds: [],
    });

    const run = commandControls("you.run");
    const violations = validateCliControlValues(run, {
      "you.run.flag.dir": "factory",
      "you.run.flag.named": "demo",
    });
    expect(violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "relationship",
          inputId: "you.run.flag.dir",
          relationshipId: "you.run.rel.selectors",
        }),
        expect.objectContaining({
          code: "relationship",
          inputId: "you.run.flag.named",
          relationshipId: "you.run.rel.selectors",
        }),
      ]),
    );
  });
});
