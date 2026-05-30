const HEADER_CATALOG_CARDS = [
  { compactChrome: true, name: "Work totals" },
  { compactChrome: false, name: "Provider session" },
  { compactChrome: false, name: "Add widget" },
  {
    compactChrome: false,
    name: "Submit work",
    primaryActionName: "Remove Submit work widget from dashboard",
  },
];

function rectsOverlap(left, right, tolerancePx = 2) {
  const horizontalOverlap =
    left.x + left.width > right.x + tolerancePx &&
    right.x + right.width > left.x + tolerancePx;
  const verticalOverlap =
    left.y + left.height > right.y + tolerancePx &&
    right.y + right.height > left.y + tolerancePx;

  return horizontalOverlap && verticalOverlap;
}

function rectIsVisible(rect) {
  return rect.width > 0 && rect.height > 0;
}

export async function collectCardHeaderMetrics(article) {
  return article.locator("header").evaluate((header) => {
    const title = header.querySelector("h3");
    const tools = header.lastElementChild;

    if (!(title instanceof HTMLElement)) {
      return { error: "Missing bento card title." };
    }

    if (header.getAttribute("data-bento-drag-handle") !== "true") {
      return { error: "Missing header drag handle attribute." };
    }

    if (!header.className.includes("cursor-grab")) {
      return { error: "Missing grab cursor affordance on header." };
    }

    const titleRect = title.getBoundingClientRect();
    const headerRect = header.getBoundingClientRect();
    const primaryActions = tools
      ? Array.from(tools.querySelectorAll("button")).filter(
          (button) => button.getAttribute("data-bento-drag-handle") !== "true",
        )
      : [];

    return {
      compactChrome:
        header.className.includes("min-h-11") &&
        header.className.includes("flex-wrap"),
      hasGrabCursor: header.className.includes("cursor-grab"),
      hasHeaderDragHandle: header.getAttribute("data-bento-drag-handle") === "true",
      headerRect: {
        height: headerRect.height,
        width: headerRect.width,
        x: headerRect.x,
        y: headerRect.y,
      },
      primaryActionRects: primaryActions.map((button) => {
        const rect = button.getBoundingClientRect();
        return {
          height: rect.height,
          label: button.getAttribute("aria-label") ?? button.textContent ?? "",
          width: rect.width,
          x: rect.x,
          y: rect.y,
        };
      }),
      titleRect: {
        height: titleRect.height,
        width: titleRect.width,
        x: titleRect.x,
        y: titleRect.y,
      },
      titleTag: title.tagName,
    };
  });
}

export function assertCardHeaderMetrics(cardName, metrics, { compactChrome }) {
  if (metrics.error) {
    throw new Error(`${cardName}: ${metrics.error}`);
  }

  if (metrics.titleTag !== "H3") {
    throw new Error(`${cardName}: expected an h3 title, found ${metrics.titleTag}.`);
  }

  if (!rectIsVisible(metrics.titleRect)) {
    throw new Error(`${cardName}: title was not visible.`);
  }

  if (!rectIsVisible(metrics.headerRect)) {
    throw new Error(`${cardName}: header drag surface was not visible.`);
  }

  if (!metrics.hasHeaderDragHandle) {
    throw new Error(`${cardName}: header was not marked as the drag handle.`);
  }

  if (!metrics.hasGrabCursor) {
    throw new Error(`${cardName}: header was missing the grab cursor affordance.`);
  }

  for (const actionRect of metrics.primaryActionRects) {
    if (!rectIsVisible(actionRect)) {
      throw new Error(
        `${cardName}: header action "${actionRect.label}" was not visible.`,
      );
    }

    if (rectsOverlap(metrics.titleRect, actionRect)) {
      throw new Error(
        `${cardName}: title overlapped header action "${actionRect.label}" at ${compactChrome ? "compact" : "default"} density.`,
      );
    }
  }

  if (compactChrome !== metrics.compactChrome) {
    throw new Error(
      `${cardName}: expected ${compactChrome ? "compact" : "default"} header chrome classes.`,
    );
  }
}

export async function verifyBentoCardCatalogHeader({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const board = page.getByRole("region", {
    name: "you-agent-factory bento board",
  });
  await expectVisible(board, "Header consistency bento board");

  for (const card of HEADER_CATALOG_CARDS) {
    const article = page.getByRole("article", { name: card.name });
    await expectVisible(article, `${card.name} bento card`);

    const dragHeader = article.locator('header[data-bento-drag-handle="true"]');
    await expectVisible(dragHeader, `${card.name} header drag handle`);

    const moveButton = article.getByRole("button", {
      name: `Move ${card.name}`,
    });
    if ((await moveButton.count()) > 0) {
      throw new Error(`${card.name}: Move button should not be rendered.`);
    }

    if (card.primaryActionName) {
      await expectVisible(
        article.getByRole("button", { name: card.primaryActionName }),
        `${card.name} primary header action`,
      );
    }

    const metrics = await collectCardHeaderMetrics(article);
    assertCardHeaderMetrics(card.name, metrics, {
      compactChrome: card.compactChrome,
    });
  }

  await expectNoHorizontalOverflow(
    page,
    `Bento card header catalog at ${viewport.label}`,
  );
}
