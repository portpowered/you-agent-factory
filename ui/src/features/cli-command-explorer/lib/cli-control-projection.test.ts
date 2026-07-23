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
    expect(
      run.controls.some(({ inputId }) => inputId === "you.run.flag.port"),
    ).toBe(false);
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
});

describe("schema-valid static CLI control variants", () => {
  it("projects schema-valid positional types, rich flags, and dependencies", () => {
    const manifest = structuredClone(canonicalCliManifest) as unknown as {
      commands: Record<
        string,
        {
          arguments: Record<string, Record<string, unknown>>;
          flags: Record<string, Record<string, unknown>>;
          relationships: Record<string, Record<string, unknown>>;
        }
      >;
    };
    const create = manifest.commands["you.factory.create"];
    const argument = { ...create.arguments["you.factory.create.arg.0"] };
    delete argument.channels;
    create.arguments["you.factory.create.arg.0"] = {
      ...argument,
      valueType: "int",
      defaultValue: { int: 7 },
      scope: "local",
      acceptedSources: ["cli", "manifest-default"],
      handlerBindingId: "you.factory.create.arg.0.handler",
      visibility: "visible",
      lifecycle: {
        ...create.flags["you.factory.create.flag.dir"].lifecycle,
        itemId: "you.factory.create.arg.0",
      },
    };
    const localFlag = create.flags["you.factory.create.flag.dir"];
    create.flags["you.factory.create.flag.dir"] = {
      id: localFlag.id,
      kind: "named",
      long: localFlag.long,
      shorthand: localFlag.shorthand,
      aliases: localFlag.aliases,
      scope: localFlag.scope,
      valueType: "stringArray",
      required: true,
      minCardinality: 2,
      maxCardinality: 3,
      defaultValue: { stringArray: ["one", "two"] },
      repeatable: true,
      normalization: localFlag.normalization,
      completion: localFlag.completion,
      acceptedSources: ["cli", "manifest-default"],
      handlerBindingId: "you.factory.create.flag.dir.handler",
      visibility: localFlag.visibility,
      lifecycle: localFlag.lifecycle,
    };
    create.relationships = {
      ...(create.relationships ?? {}),
      "you.factory.create.rel.name-dependency": {
        id: "you.factory.create.rel.name-dependency",
        kind: "dependency",
        participants: [{ type: "argument", id: "you.factory.create.arg.0" }],
        when: { type: "flag", id: "you.factory.create.flag.set-current" },
      },
    };

    const loaded = loadCliManifest(manifest);
    expect(loaded.status).toBe("ready");
    if (loaded.status !== "ready") return;
    const command = projectCliManifest(loaded).commands["you.factory.create"];
    const projected = projectCliCommandControls(command);
    expect(projected.status).toBe("ready");
    if (projected.status !== "ready") return;

    expect(
      projected.model.controls.find(
        ({ inputId }) => inputId === "you.factory.create.arg.0",
      ),
    ).toMatchObject({ kind: "number", defaultValue: "7" });
    expect(
      projected.model.controls.find(
        ({ inputId }) => inputId === "you.factory.create.flag.dir",
      ),
    ).toMatchObject({
      kind: "repeated",
      cardinality: { minimum: 2, maximum: 3 },
      defaultValue: ["one", "two"],
    });
    expect(
      validateCliControlValues(
        projected.model,
        {
          "you.factory.create.arg.0": "",
          "you.factory.create.flag.dir": ["one", "two"],
          "you.factory.create.flag.set-current": true,
        },
        new Set(["you.factory.create.flag.set-current"]),
      ),
    ).toContainEqual({
      code: "relationship",
      inputId: "you.factory.create.arg.0",
      relationshipId: "you.factory.create.rel.name-dependency",
      relationshipKind: "dependency",
      relatedInputIds: ["you.factory.create.flag.set-current"],
    });
  });
});

describe("schema-valid positional value types", () => {
  it.each([
    ["bool", { boolean: true }, "boolean", true],
    ["int64", { int64: 9 }, "number", "9"],
    ["stringArray", { stringArray: ["one"] }, "repeated", ["one"]],
  ] as const)(
    "projects %s positional values without a text fallback",
    (valueType, defaultValue, expectedKind, expectedDefault) => {
      const command = {
        id: "you.example",
        effectiveInputs: [
          {
            id: "you.example.arg.0",
            kind: "argument",
            inherited: false,
            source: {
              commandId: "you.example",
              inputId: "you.example.arg.0",
              scope: "local",
            },
            cardinality: { minimum: 0, maximum: 1 },
            manifestInput: {
              id: "you.example.arg.0",
              name: "value",
              valueType,
              required: false,
              defaultValue,
            },
          },
        ],
        relationships: [],
      } as unknown as Parameters<typeof projectCliCommandControls>[0];
      const projected = projectCliCommandControls(command);
      expect(projected.status).toBe("ready");
      if (projected.status !== "ready") return;
      expect(projected.model.controls[0]).toMatchObject({
        kind: expectedKind,
        defaultValue: expectedDefault,
      });
    },
  );
});

describe("static CLI control validation", () => {
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

  it("validates relationships from explicit input state instead of displayed defaults", () => {
    const run = commandControls("you.run");
    const values = {
      "you.run.flag.dir": "factory",
      "you.run.flag.named": "demo",
    };

    expect(
      validateCliControlValues(run, values, new Set(["you.run.flag.named"])),
    ).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ code: "relationship" }),
      ]),
    );
    expect(
      validateCliControlValues(
        run,
        values,
        new Set(["you.run.flag.dir", "you.run.flag.named"]),
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "relationship",
          relationshipId: "you.run.rel.selectors",
        }),
      ]),
    );
  });
});
