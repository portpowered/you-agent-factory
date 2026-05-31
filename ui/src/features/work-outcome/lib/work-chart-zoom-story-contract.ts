import { fireEvent } from "storybook/test";

export async function dragWorkChart(
  chart: HTMLElement,
  startFraction: number,
  endFraction: number,
): Promise<void> {
  const rect = chart.getBoundingClientRect();
  const startX = rect.left + rect.width * startFraction;
  const endX = rect.left + rect.width * endFraction;
  const y = rect.top + rect.height * 0.7;

  fireEvent.mouseDown(chart, { clientX: startX, clientY: y });
  fireEvent.mouseMove(chart, { clientX: endX, clientY: y });
  fireEvent.mouseUp(chart, { clientX: endX, clientY: y });
}
