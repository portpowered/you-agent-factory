import { FactoryGraphNodeBadge } from "./semantic-support-nodes.js";

export function FactoryGraphPlaceTokenCount({
  ariaLabel,
  count,
}: {
  ariaLabel: string;
  count: number;
}) {
  return (
    <FactoryGraphNodeBadge
      aria-label={ariaLabel}
      className="w-fit"
      data-place-token-count
      role="status"
    >
      {count}
    </FactoryGraphNodeBadge>
  );
}
