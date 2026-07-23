import {
  collectInvocationFieldErrors,
  extractInvocationFieldError,
  projectInvocationForm,
  serializeInvocationArgs,
} from "./factory-invocation-form";

const signature = {
  outputContract: {
    contentType: "text/plain",
    description: "Writes a summary file.",
    mode: "FILE",
    pathParameter: "output",
  },
  parameters: [
    {
      bindings: [{ kind: "POSITIONAL", position: 1 }],
      description: "Primary input text.",
      name: "input",
      required: true,
    },
    {
      aliases: ["o"],
      bindings: [{ kind: "NAMED" }],
      defaultValue: "/tmp/out.txt",
      externalName: "output",
      name: "outputPath",
      typeHint: "FILE_PATH",
    },
    {
      bindings: [{ kind: "NAMED" }],
      choices: ["low", "medium", "high"],
      name: "effort",
    },
    {
      bindings: [{ kind: "NAMED" }],
      name: "confirm",
      typeHint: "BOOLEAN_STRING",
    },
    {
      bindings: [{ kind: "NAMED" }],
      name: "tag",
      valueMode: "REPEATED",
    },
    {
      bindings: [{ kind: "NAMED" }],
      name: "attachment",
      typeHint: "FILE_PATH",
      valueMode: "FILE_CONTENTS",
    },
  ],
} as const;

describe("factory invocation form projection", () => {
  it("projects signature-backed dashboard controls from canonical parameter data", () => {
    const projection = projectInvocationForm(signature, [
      {
        args: { input: "hello" },
        description: { type: "LOCALIZABLE_ASSET", value: "Basic invocation" },
        name: "basic",
      },
    ]);

    expect(projection.examples).toHaveLength(1);
    expect(projection.outputContract?.mode).toBe("FILE");
    expect(projection.fields).toEqual([
      expect.objectContaining({
        kind: "text",
        label: "input",
        name: "input",
        position: 1,
        required: true,
      }),
      expect.objectContaining({
        aliases: ["o"],
        defaultValues: ["/tmp/out.txt"],
        kind: "text",
        label: "output",
        name: "outputPath",
        pathHint: "file",
      }),
      expect.objectContaining({
        choices: ["low", "medium", "high"],
        kind: "choice",
        name: "effort",
      }),
      expect.objectContaining({
        kind: "boolean",
        name: "confirm",
      }),
      expect.objectContaining({
        kind: "repeated",
        name: "tag",
      }),
      expect.objectContaining({
        kind: "text",
        name: "attachment",
        pathHint: "file",
      }),
    ]);
  });
});

describe("factory invocation form serialization", () => {
  it("serializes dashboard field state into InvocationRequest.args", () => {
    const args = serializeInvocationArgs(
      projectInvocationForm(signature).fields,
      {
        confirm: ["false"],
        input: ["hello world"],
        outputPath: ["/tmp/report.txt"],
        tag: ["alpha", "beta", ""],
        attachment: ["/tmp/source.txt"],
      },
    );

    expect(args).toEqual({
      attachment: "/tmp/source.txt",
      confirm: "false",
      input: "hello world",
      outputPath: "/tmp/report.txt",
      tag: ["alpha", "beta"],
    });
  });
});

describe("factory invocation form validation", () => {
  it("reports local field validation for required and repeated inputs", () => {
    const fieldErrors = collectInvocationFieldErrors(
      projectInvocationForm(signature).fields,
      {
        input: [""],
        tag: ["alpha", ""],
      },
      {
        repeatedItemRequired: "Each repeated value must be non-empty.",
        requiredFieldMessage: (label) => `Enter ${label} before invoking.`,
      },
    );

    expect(fieldErrors).toEqual({
      input: "Enter input before invoking.",
      tag: "Each repeated value must be non-empty.",
    });
  });
});

describe("factory invocation form backend error mapping", () => {
  it("maps backend invocation messages back to the matching field", () => {
    const projection = projectInvocationForm(signature);

    expect(
      extractInvocationFieldError(
        projection.fields,
        'required invocation parameter "input" is missing',
      ),
    ).toEqual({
      fieldName: "input",
      message: 'required invocation parameter "input" is missing',
    });
    expect(
      extractInvocationFieldError(
        projection.fields,
        "compatibility content cannot be combined with positional or stdin input",
      ),
    ).toBeNull();
  });
});
