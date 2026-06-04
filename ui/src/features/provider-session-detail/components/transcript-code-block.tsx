import {
  ExpandableTranscriptContent,
  TranscriptContentPanel,
} from "./expandable-transcript-content";

export function ExpandableCodeBlock({
  label,
  locale,
  value,
}: {
  label: string;
  locale?: string;
  value: string;
}) {
  return (
    <ExpandableTranscriptContent
      kind="code"
      label={label}
      locale={locale}
      value={value}
    />
  );
}

export function CodePanel({ value }: { value: string }) {
  return <TranscriptContentPanel kind="code" value={value} />;
}
