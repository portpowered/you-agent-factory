import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import {
  loadCliManifest,
  loadingCliManifest,
  loadPublishedCliManifest,
} from "./cli-manifest-adapter";

function lifecycle(id: string) {
  return {
    formatVersion: "1.0.0",
    itemId: id,
    since: "1.0.0",
    state: "active",
  };
}

function documentation(id: string) {
  return {
    formatVersion: "1.0.0",
    itemId: id,
    documentation: {
      title: { id: `${id}.title`, canonicalEnglish: `Title for ${id}` },
      description: {
        id: `${id}.description`,
        canonicalEnglish: `Description for ${id}`,
      },
    },
    examples: [id],
    visibility: "public",
    sourceHash:
      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  };
}

function flag(id: string, scope: "inherited" | "local" | "persistent") {
  return {
    id,
    long: "verbose",
    shorthand: "v",
    aliases: [],
    scope,
    valueType: "bool",
    required: false,
    default: "false",
    changedDefault: false,
    noOptionDefault: "true",
    repeatable: false,
    normalization: "",
    completion: "none",
    binding: "",
    visibility: "visible",
    lifecycle: lifecycle(id),
    ...(scope === "inherited"
      ? { inheritedFromInputId: "you.flag.verbose" }
      : {}),
  };
}

function command(id: string, path: string) {
  return {
    id,
    name: path.split(" ").at(-1),
    path,
    aliases: [],
    documentation: documentation(id),
    lifecycle: lifecycle(id),
    visibility: "visible",
    runnable: true,
    usage: { line: path },
  };
}

function validManifest() {
  return {
    formatVersion: "1.0.0",
    rootPath: "you",
    commands: {
      you: {
        ...command("you", "you"),
        flags: { "you.flag.verbose": flag("you.flag.verbose", "persistent") },
      },
      "you.run": {
        ...command("you.run", "you run"),
        flags: {
          "you.run.flag.verbose": flag("you.run.flag.verbose", "inherited"),
          "you.run.flag.work": {
            ...flag("you.run.flag.work", "local"),
            long: "work",
            shorthand: "w",
            valueType: "string",
            default: "",
            noOptionDefault: "",
          },
        },
        arguments: {
          "you.run.arg.prompt": {
            id: "you.run.arg.prompt",
            name: "prompt",
            position: 0,
            kind: "positional",
            valueType: "string",
            required: false,
            minCardinality: 0,
            maxCardinality: 1,
            variadic: false,
            enum: [],
            pattern: "",
            completion: "none",
            channels: ["cli"],
            doubleDash: "terminates-flags",
          },
        },
        relationships: {
          "you.run.rel.choice": {
            id: "you.run.rel.choice",
            kind: "mutually-exclusive",
            participants: [
              { type: "argument", id: "you.run.arg.prompt" },
              { type: "flag", id: "you.run.flag.work" },
            ],
          },
        },
      },
    },
  };
}

function diagnosticCodes(input: unknown) {
  const result = loadCliManifest(input);
  expect(result.status).toBe("invalid-contract");
  return result.status === "invalid-contract"
    ? result.diagnostics.map((diagnostic) => diagnostic.code)
    : [];
}

function diagnosticMessages(input: unknown, locale: string) {
  const result = loadCliManifest(input, locale);
  expect(result.status).toBe("invalid-contract");
  return result.status === "invalid-contract"
    ? result.diagnostics.map((diagnostic) => diagnostic.message)
    : [];
}

function expectLocalizedIdentityAndHierarchyDiagnostics() {
  const identity = validManifest();
  identity.commands["you.run"].id = "you";
  identity.commands["you.run"].flags["you.run.flag.work"].id =
    "you.flag.verbose";
  expect(diagnosticMessages(identity, "zh-CN")).toEqual(
    expect.arrayContaining([
      "稳定标识 you 与 commands.you.id 重复。",
      "命令键 you.run 必须与稳定标识 you 一致。",
      "输入键 you.run.flag.work 必须与稳定标识 you.flag.verbose 一致。",
    ]),
  );

  const hierarchy = validManifest();
  hierarchy.rootPath = "other";
  hierarchy.commands["you.run"].path = "elsewhere wrong";
  expect(diagnosticMessages(hierarchy, "zh-CN")).toEqual(
    expect.arrayContaining([
      "根路径 other 无法解析为命令。",
      "命令路径 elsewhere wrong 必须由非空段组成，并以命令名 run 结尾。",
      "父命令路径 elsewhere 无法解析。",
      "命令路径 elsewhere wrong 不在根路径 other 下。",
    ]),
  );

  const duplicatePath = validManifest();
  duplicatePath.commands["you.run"].path = "you";
  expect(diagnosticMessages(duplicatePath, "zh-CN")).toContain(
    "命令路径 you 重复。",
  );
}

function expectLocalizedCardinalityAndRelationshipDiagnostics() {
  const cardinality = validManifest();
  cardinality.commands["you.run"].arguments["you.run.arg.prompt"].variadic =
    true;
  cardinality.commands["you.run"].arguments[
    "you.run.arg.prompt"
  ].maxCardinality = -1;
  cardinality.commands["you.run"].arguments["you.run.arg.prompt"].position = 1;
  cardinality.commands["you.run"].arguments["you.run.arg.extra"] = {
    ...cardinality.commands["you.run"].arguments["you.run.arg.prompt"],
    id: "you.run.arg.extra",
    name: "extra",
    position: 2,
    variadic: false,
    maxCardinality: 1,
  };
  expect(diagnosticMessages(cardinality, "zh-CN")).toEqual(
    expect.arrayContaining([
      "参数位置必须从零开始连续且不重复；此处应为 0。",
      "只有最后一个位置参数可以是可变参数。",
    ]),
  );

  const relationship = validManifest();
  relationship.commands["you.run"].relationships[
    "you.run.rel.choice"
  ].participants[0] = {
    type: "flag",
    id: "you.run.flag.missing",
  };
  expect(diagnosticMessages(relationship, "zh-CN")).toContain(
    "关系参与者 you.run.flag.missing 无法解析为此命令的 flag 输入。",
  );
}

function expectLocalizedInheritanceAndSchemaDiagnostics() {
  const contradictoryInheritance = validManifest();
  contradictoryInheritance.commands.you.flags["you.flag.verbose"].default =
    "true";
  expect(diagnosticMessages(contradictoryInheritance, "zh-CN")).toContain(
    "继承标志 --verbose 与其持久祖先定义相矛盾。",
  );

  const missingInheritance = validManifest();
  missingInheritance.commands.you.flags = {};
  expect(diagnosticMessages(missingInheritance, "zh-CN")).toContain(
    "继承标志 --verbose 没有对应的持久祖先定义。",
  );

  const schemaConstraint = validManifest();
  schemaConstraint.commands["you.run"].id = "invalid id";
  expect(diagnosticMessages(schemaConstraint, "zh-CN")).toContain(
    "commands.you.run.id 应符合 CLI 清单契约：必须满足 pattern 约束。",
  );

  expect(
    diagnosticMessages(
      { formatVersion: "1.0.0", rootPath: 4, commands: {} },
      "zh-CN",
    ),
  ).toContain("rootPath 应符合 CLI 清单契约：必须是字符串。");
}

describe("CLI manifest adapter", () => {
  it("exposes an explicit loading state", () => {
    expect(loadingCliManifest()).toEqual({ status: "loading" });
  });

  it("accepts the canonical rich manifest without copying or mutating it", () => {
    const before = JSON.stringify(canonicalCliManifest);
    const result = loadCliManifest(canonicalCliManifest);

    expect(result.status).toBe("ready");
    if (result.status !== "ready") return;
    expect(result.manifest).toBe(canonicalCliManifest);
    expect(result.manifest.commands.you.path).toBe("you");
    expect(
      result.manifest.commands["you.run"].flags?.["you.run.flag.verbose"]
        ?.scope,
    ).toBe("inherited");
    expect(JSON.stringify(canonicalCliManifest)).toBe(before);
  });

  it("returns empty only for a valid supported manifest with no commands", () => {
    const manifest = { formatVersion: "1.0.0", rootPath: "you", commands: {} };
    expect(loadCliManifest(manifest)).toEqual({ status: "empty", manifest });
    expect(
      diagnosticCodes({ ...manifest, rootPath: "you  invalid" }),
    ).toContain("invalid_hierarchy");
  });

  it("accepts a representative root and nested command graph", () => {
    expect(loadCliManifest(validManifest())).toMatchObject({ status: "ready" });
  });

  it("rejects unknown versions without creating a partial projection", () => {
    expect(
      loadCliManifest({ ...validManifest(), formatVersion: "2.0.0" }),
    ).toEqual({
      status: "unsupported-version",
      receivedVersion: "2.0.0",
      supportedVersions: ["1.0.0"],
    });
  });

  it("loads the supported rich graph from the published package artifact", () => {
    const result = loadPublishedCliManifest();
    expect(result).toMatchObject({ status: "ready" });
    if (result.status !== "ready") return;
    expect(result.manifest.commands.you.path).toBe("you");
    expect(result.manifest.commands["you.run"].path).toBe("you run");
  });

  it("returns deterministic actionable structural diagnostics", () => {
    const result = loadCliManifest({
      formatVersion: "1.0.0",
      rootPath: 4,
      commands: [],
    });
    expect(result).toEqual({
      status: "invalid-contract",
      diagnostics: [
        {
          code: "invalid_type",
          path: ["commands"],
          message:
            "Expected commands to satisfy the CLI manifest contract: must be object.",
        },
        {
          code: "invalid_type",
          path: ["rootPath"],
          message:
            "Expected rootPath to satisfy the CLI manifest contract: must be string.",
        },
      ],
    });
  });

  it("rejects unstable and duplicate command or input identities", () => {
    const unstable = validManifest();
    unstable.commands["you.run"].id = "You Run";
    expect(diagnosticCodes(unstable)).toContain("unstable_id");

    const duplicate = validManifest();
    duplicate.commands.you.flags = { you: flag("you", "local") };
    expect(diagnosticCodes(duplicate)).toContain("duplicate_id");
  });

  it("rejects invalid roots and missing parent command paths", () => {
    const invalidRoot = { ...validManifest(), rootPath: "other" };
    expect(diagnosticCodes(invalidRoot)).toContain("invalid_hierarchy");

    const missingParent = validManifest();
    missingParent.commands["you.run"].path = "you factory run";
    expect(diagnosticCodes(missingParent)).toContain("invalid_hierarchy");
  });

  it("rejects inherited flags without a matching persistent ancestor", () => {
    const manifest = validManifest();
    manifest.commands.you.flags["you.flag.verbose"].default = "true";
    expect(diagnosticCodes(manifest)).toContain("contradictory_inheritance");
  });

  it("rejects impossible argument and repeatable-flag cardinality", () => {
    const manifest = validManifest();
    manifest.commands["you.run"].arguments["you.run.arg.prompt"].variadic =
      true;
    manifest.commands["you.run"].flags["you.run.flag.work"].repeatable = true;
    expect(diagnosticCodes(manifest)).toEqual(
      expect.arrayContaining(["invalid_cardinality", "invalid_cardinality"]),
    );
  });

  it("rejects relationships with missing or invalid participants", () => {
    const manifest = validManifest();
    manifest.commands["you.run"].relationships[
      "you.run.rel.choice"
    ].participants = [
      { type: "flag", id: "you.run.flag.missing" },
      { type: "argument", id: "you.run.arg.prompt" },
    ];
    const codes = diagnosticCodes(manifest);
    expect(codes).toContain("invalid_reference");
    expect(codes.filter((code) => code === "invalid_reference")).toHaveLength(
      1,
    );
  });

  it("localizes every semantic diagnostic family deterministically", () => {
    expectLocalizedIdentityAndHierarchyDiagnostics();
    expectLocalizedCardinalityAndRelationshipDiagnostics();
    expectLocalizedInheritanceAndSchemaDiagnostics();
  });
});
