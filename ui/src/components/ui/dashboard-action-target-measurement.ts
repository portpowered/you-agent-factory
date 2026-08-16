export const DASHBOARD_ACTION_TARGET_MINIMUM = 44;

export interface DashboardActionTargetMeasurement {
  bounds: Pick<DOMRect, "bottom" | "left" | "right" | "top">;
  context: string;
  height: number;
  label: string;
  width: number;
}

export function measureDashboardActionTargets(
  root: HTMLElement,
): DashboardActionTargetMeasurement[] {
  return Array.from(root.querySelectorAll<HTMLButtonElement>("button")).map(
    (button) => {
      const bounds = button.getBoundingClientRect();
      const context = button.closest<HTMLElement>(
        "[data-dashboard-action-context]",
      )?.dataset.dashboardActionContext;

      return {
        bounds,
        context: context ?? "unscoped",
        height: bounds.height,
        label:
          button.getAttribute("aria-label") ??
          button.textContent?.trim() ??
          "(unnamed)",
        width: bounds.width,
      };
    },
  );
}

export function assertDashboardActionTargets(root: HTMLElement): void {
  const measurements = measureDashboardActionTargets(root);
  const belowMinimum = measurements.filter(
    ({ height, width }) =>
      width < DASHBOARD_ACTION_TARGET_MINIMUM ||
      height < DASHBOARD_ACTION_TARGET_MINIMUM,
  );

  if (belowMinimum.length > 0) {
    throw new Error(
      `Dashboard action targets below ${DASHBOARD_ACTION_TARGET_MINIMUM}px: ${belowMinimum.length}/${measurements.length} ${JSON.stringify(belowMinimum)}`,
    );
  }

  const overlaps = measurements.flatMap((left, leftIndex) =>
    measurements
      .slice(leftIndex + 1)
      .flatMap((right) =>
        rectanglesOverlap(left.bounds, right.bounds)
          ? [
              `${left.context}:${left.label} overlaps ${right.context}:${right.label}`,
            ]
          : [],
      ),
  );

  if (overlaps.length > 0) {
    throw new Error(`Dashboard action targets overlap: ${overlaps.join(", ")}`);
  }
}

function rectanglesOverlap(
  left: Pick<DOMRect, "bottom" | "left" | "right" | "top">,
  right: Pick<DOMRect, "bottom" | "left" | "right" | "top">,
): boolean {
  return (
    left.left < right.right &&
    left.right > right.left &&
    left.top < right.bottom &&
    left.bottom > right.top
  );
}
