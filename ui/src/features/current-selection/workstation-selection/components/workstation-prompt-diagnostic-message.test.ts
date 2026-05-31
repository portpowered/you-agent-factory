import { describe, expect, it } from "vitest";

import { formatSyntaxDiagnosticMessage } from "./workstation-prompt-diagnostic-message";

describe("formatSyntaxDiagnosticMessage", () => {
  it("returns API-normalized line-based messages unchanged", () => {
    expect(formatSyntaxDiagnosticMessage("line 2: unexpected EOF")).toBe(
      "line 2: unexpected EOF",
    );
  });

  it("normalizes legacy template parse error prefixes", () => {
    expect(
      formatSyntaxDiagnosticMessage("template: prompt:4: unexpected right paren"),
    ).toBe("line 4: unexpected right paren");
    expect(
      formatSyntaxDiagnosticMessage("template: prompt:2:3: function \"bad\" not defined"),
    ).toBe('line 2: function "bad" not defined');
  });

  it("returns other legacy messages unchanged", () => {
    expect(formatSyntaxDiagnosticMessage("Unexpected EOF in if block.")).toBe(
      "Unexpected EOF in if block.",
    );
  });
});
