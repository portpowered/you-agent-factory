import { isAPIRecord } from "../../../api/transport";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { formatNumber } from "../../../i18n/formatters";
import { cn } from "../../../lib/cn";
import { getProviderSessionDetailMessages } from "../messages/provider-session-detail";
import { TranscriptContentPanel } from "./expandable-transcript-content";

const EXEC_COMMAND_SUMMARY_CHAR_LIMIT = 200;

export function FriendlyExecCommandOutput({
  locale,
  output,
  status,
  text,
}: {
  locale?: string;
  output: string;
  status?: string;
  text?: string;
}) {
  const messages = getProviderSessionDetailMessages(locale);
  const friendlyOutput = parseExecCommandOutput(output);

  if (friendlyOutput === null) {
    return (
      <div className="grid gap-3">
        {text ? <Text className="m-0">{text}</Text> : null}
        <ExecCommandContentSection
          label={messages.outputLabel}
          value={output}
        />
      </div>
    );
  }

  return (
    <div className="grid gap-3">
      {text ? <Text className="m-0">{text}</Text> : null}
      <SurfacePanel asChild className="grid gap-2" radius="lg" surface="low">
        <section>
          <Label>{messages.execCommandResultHeading}</Label>
          <div className="grid gap-3">
            {friendlyOutput.exitCode !== null ? (
              <SummaryMetric
                label={messages.execCommandExitCodeLabel}
                value={String(friendlyOutput.exitCode)}
              />
            ) : null}
            {friendlyOutput.wallTime ? (
              <SummaryMetric
                label={messages.execCommandWallTimeLabel}
                value={formatExecCommandWallTime(
                  friendlyOutput.wallTime,
                  locale,
                )}
              />
            ) : null}
            {status ? (
              <SummaryMetric
                label={messages.sessionStatusLabel}
                value={status}
              />
            ) : null}
            <SummaryMetric
              label={messages.execCommandOutputSummaryLabel}
              value={
                friendlyOutput.summary ?? messages.execCommandNoOutputSummary
              }
            />
          </div>
        </section>
      </SurfacePanel>
      <ExecCommandContentSection
        label={messages.execCommandRawOutputLabel}
        value={output}
      />
    </div>
  );
}

function ExecCommandContentSection({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <section className="grid gap-2">
      <Label>{label}</Label>
      <TranscriptContentPanel kind="code" value={value} />
    </section>
  );
}

function SummaryMetric({
  className,
  label,
  value,
}: {
  className?: string;
  label: string;
  value: string;
}) {
  return (
    <div className={cn("grid gap-1", className)}>
      <Label>{label}</Label>
      <Text className="m-0">{value}</Text>
    </div>
  );
}

function parseExecCommandOutput(output: string): {
  exitCode: number | null;
  summary: string | null;
  wallTime: string | null;
} | null {
  const parsedJSON = tryParseExecCommandJSON(output);
  if (parsedJSON) {
    return parsedJSON;
  }

  const normalizedOutput = output.replace(/\r\n/g, "\n");
  const lines = normalizedOutput.split("\n");
  const outputStartIndex = lines.findIndex((line) => line.trim() === "Output:");
  const hasKnownMetadata =
    lines.some((line) => line.startsWith("Wall time:")) ||
    lines.some((line) => /^Process exited with code -?\d+/.test(line));

  if (outputStartIndex === -1 || !hasKnownMetadata) {
    return null;
  }

  const wallTimeLine = lines.find((line) => line.startsWith("Wall time:"));
  const exitCodeLine = lines.find((line) =>
    /^Process exited with code -?\d+/.test(line),
  );
  const rawCommandOutput = lines
    .slice(outputStartIndex + 1)
    .join("\n")
    .trim();

  return {
    exitCode: exitCodeLine
      ? Number.parseInt(
          exitCodeLine.replace(/^Process exited with code /, ""),
          10,
        )
      : null,
    summary: summarizeExecCommandText(rawCommandOutput),
    wallTime: wallTimeLine ? wallTimeLine.replace(/^Wall time:\s*/, "") : null,
  };
}

function tryParseExecCommandJSON(output: string): {
  exitCode: number | null;
  summary: string | null;
  wallTime: string | null;
} | null {
  try {
    const parsed = JSON.parse(output) as unknown;
    if (!isAPIRecord(parsed)) {
      return null;
    }

    const exitCode =
      typeof parsed.exit_code === "number"
        ? parsed.exit_code
        : typeof parsed.exitCode === "number"
          ? parsed.exitCode
          : null;
    const wallTime =
      typeof parsed.wall_time_seconds === "number"
        ? `${parsed.wall_time_seconds} seconds`
        : typeof parsed.wallTimeSeconds === "number"
          ? `${parsed.wallTimeSeconds} seconds`
          : typeof parsed.wall_time === "string"
            ? parsed.wall_time
            : typeof parsed.wallTime === "string"
              ? parsed.wallTime
              : null;
    const rawOutput =
      typeof parsed.output === "string"
        ? parsed.output
        : typeof parsed.stdout === "string"
          ? parsed.stdout
          : typeof parsed.summary === "string"
            ? parsed.summary
            : null;

    if (exitCode === null && wallTime === null && rawOutput === null) {
      return null;
    }

    return {
      exitCode,
      summary: summarizeExecCommandText(rawOutput),
      wallTime,
    };
  } catch {
    return null;
  }
}

function summarizeExecCommandText(value: string | null): string | null {
  if (!value) {
    return null;
  }

  const firstMeaningfulLine = value
    .split("\n")
    .map((line) => line.trim())
    .find((line) => line.length > 0);

  if (!firstMeaningfulLine) {
    return null;
  }

  return firstMeaningfulLine.length > EXEC_COMMAND_SUMMARY_CHAR_LIMIT
    ? `${firstMeaningfulLine.slice(0, EXEC_COMMAND_SUMMARY_CHAR_LIMIT)}…`
    : firstMeaningfulLine;
}

function formatExecCommandWallTime(value: string, locale?: string): string {
  const secondsMatch = value.match(
    /^\s*(?<seconds>\d+(?:\.\d+)?)\s+seconds?\s*$/i,
  );
  const secondsText = secondsMatch?.groups?.seconds;
  if (!secondsText) {
    return value;
  }

  const secondsValue = Number(secondsText);
  if (!Number.isFinite(secondsValue)) {
    return value;
  }

  const fractionDigits = secondsText.includes(".")
    ? (secondsText.split(".")[1]?.length ?? 0)
    : 0;

  return formatNumber(secondsValue, locale, {
    style: "unit",
    unit: "second",
    unitDisplay: "long",
    maximumFractionDigits: fractionDigits,
    minimumFractionDigits: fractionDigits,
  });
}
