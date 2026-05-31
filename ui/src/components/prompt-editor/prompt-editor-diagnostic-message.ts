const LINE_BASED_SYNTAX_MESSAGE_PATTERN = /^line \d+: /;

export function formatSyntaxDiagnosticMessage(message: string): string {
  if (LINE_BASED_SYNTAX_MESSAGE_PATTERN.test(message)) {
    return message;
  }

  const parsed = parseGoTemplateParseErrorMessage(message);
  if (parsed) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return `line ${parsed.line}: ${parsed.humanMessage}`;
  }

  return message;
}

function parseGoTemplateParseErrorMessage(
  text: string,
): { humanMessage: string; line: number } | null {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  const prefix = "template: prompt:";
  if (!text.startsWith(prefix)) {
    return null;
  }

  const rest = text.slice(prefix.length);
  const lineSeparator = rest.indexOf(":");
  if (lineSeparator < 0) {
    return null;
  }

  const linePart = rest.slice(0, lineSeparator);
  const parsedLine = Number.parseInt(linePart, 10);
  if (!Number.isFinite(parsedLine) || parsedLine < 1) {
    return null;
  }

  const afterLine = rest.slice(lineSeparator + 1);
  const columnSeparator = afterLine.indexOf(":");
  if (columnSeparator >= 0) {
    const columnPart = afterLine.slice(0, columnSeparator);
    const parsedColumn = Number.parseInt(columnPart, 10);
    if (Number.isFinite(parsedColumn)) {
      return {
        humanMessage: afterLine.slice(columnSeparator + 1).trim(),
        line: parsedLine,
      };
    }
  }

  return {
    humanMessage: afterLine.trim(),
    line: parsedLine,
  };
}
