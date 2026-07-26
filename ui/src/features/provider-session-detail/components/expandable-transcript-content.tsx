import { CodePanel } from "../../../components/ui/code-panel";
import { AuthoredBodyText } from "../../../lib/authored-body-text";

export function TranscriptContentPanel({
  id,
  kind = "text",
  value,
}: {
  id?: string;
  kind?: "code" | "text";
  value: string;
}) {
  if (kind === "code") {
    return (
      <CodePanel id={id} padding="default" surface="low">
        {value}
      </CodePanel>
    );
  }

  return (
    <div id={id}>
      <AuthoredBodyText className="border-0 bg-transparent p-0" value={value} />
    </div>
  );
}
