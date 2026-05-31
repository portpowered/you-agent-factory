import { getWorkOutcomeMessages } from "../messages/work-outcome";
import { expectSingleWorkOutcomeCardHeader as expectSingleWorkOutcomeCardHeaderCore } from "./work-outcome-card-header-contract";

export function expectSingleWorkOutcomeCardHeader(
  card: HTMLElement,
  {
    headerActionLabel,
    locale,
  }: {
    headerActionLabel?: string;
    locale?: string;
  } = {},
): void {
  const chartMessages = getWorkOutcomeMessages(locale).chart;
  expectSingleWorkOutcomeCardHeaderCore(card, {
    cardRegionLabel: chartMessages.cardRegionLabel,
    cardTitle: chartMessages.cardTitle,
    headerActionLabel,
  });
}
