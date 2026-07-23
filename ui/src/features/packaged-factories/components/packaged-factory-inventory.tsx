import { type KeyboardEvent, useId, useRef } from "react";

import {
  AlertPanel,
  AlertPanelText,
  Heading,
  StandardListSelection,
  StandardListSelectionItem,
  SurfacePanel,
} from "../../../components/ui";
import { useAppLocale } from "../../../i18n";
import { usePackagedFactoryInventory } from "../hooks/use-packaged-factory-inventory";
import type { PackagedFactoryPublicDataSource } from "../lib/public-contract";
import { getPackagedFactoryInventoryMessages } from "../messages/inventory";
import {
  type PackagedFactoryCopyText,
  PackagedFactoryDetail,
} from "./packaged-factory-detail";

export interface PackagedFactoryInventoryProps {
  readonly copyText?: PackagedFactoryCopyText;
  readonly locale?: string;
  readonly source: PackagedFactoryPublicDataSource;
}

function CatalogStatus({
  children,
  role = "status",
  tone = "neutral",
}: {
  readonly children: string;
  readonly role?: "alert" | "status";
  readonly tone?: "danger" | "neutral";
}) {
  return (
    <AlertPanel className="min-w-0" role={role} tone={tone} variant="empty">
      <AlertPanelText>{children}</AlertPanelText>
    </AlertPanel>
  );
}

async function copyToClipboard(value: string) {
  if (!navigator.clipboard?.writeText) {
    throw new Error("Clipboard API unavailable.");
  }
  await navigator.clipboard.writeText(value);
}

function focusRelativeItem(
  event: KeyboardEvent<HTMLElement>,
  buttons: readonly (HTMLButtonElement | null)[],
) {
  const currentIndex = buttons.indexOf(event.target as HTMLButtonElement);
  if (currentIndex < 0) {
    return;
  }

  let nextIndex: number | undefined;
  if (event.key === "ArrowDown" || event.key === "ArrowRight") {
    nextIndex = (currentIndex + 1) % buttons.length;
  } else if (event.key === "ArrowUp" || event.key === "ArrowLeft") {
    nextIndex = (currentIndex - 1 + buttons.length) % buttons.length;
  } else if (event.key === "Home") {
    nextIndex = 0;
  } else if (event.key === "End") {
    nextIndex = buttons.length - 1;
  }

  if (nextIndex !== undefined) {
    event.preventDefault();
    buttons[nextIndex]?.focus();
  }
}

export function PackagedFactoryInventory({
  copyText = copyToClipboard,
  locale: localeOverride,
  source,
}: PackagedFactoryInventoryProps) {
  const { locale } = useAppLocale(localeOverride);
  const messages = getPackagedFactoryInventoryMessages(locale);
  const { select, state } = usePackagedFactoryInventory(source, locale);
  const detailHeadingID = useId();
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  if (state.status === "loading") {
    return <CatalogStatus>{messages.loading}</CatalogStatus>;
  }
  if (state.status === "empty") {
    return <CatalogStatus>{messages.empty}</CatalogStatus>;
  }
  if (state.status === "invalid-contract") {
    return (
      <CatalogStatus role="alert" tone="danger">
        {messages.invalidContract}
      </CatalogStatus>
    );
  }
  if (state.status === "unsupported-version") {
    return (
      <CatalogStatus role="alert" tone="danger">
        {messages.unsupportedVersion(state.formatVersion)}
      </CatalogStatus>
    );
  }

  const selectedItem = state.inventory.byIdentity[state.selectedIdentity];
  return (
    <section
      aria-label={messages.catalogLabel}
      className="grid min-w-0 gap-layout-section"
    >
      <Heading as="h2">{messages.catalogTitle}</Heading>
      <div className="grid min-w-0 gap-layout-section lg:grid-cols-3">
        <nav aria-label={messages.inventoryLabel} className="min-w-0">
          <StandardListSelection
            selectionAnnouncement={
              selectedItem
                ? messages.selected(selectedItem.stableName)
                : undefined
            }
          >
            <ul
              className="grid min-w-0 gap-layout-tight"
              onKeyDown={(event) => focusRelativeItem(event, itemRefs.current)}
            >
              {state.inventory.items.map((item, index) => {
                const selected = item.identity === state.selectedIdentity;
                return (
                  <li className="min-w-0" key={item.identity}>
                    <StandardListSelectionItem
                      aria-current={selected ? "true" : undefined}
                      className="min-w-0"
                      onClick={() => select(item.identity)}
                      ref={(element) => {
                        itemRefs.current[index] = element;
                      }}
                      selected={selected}
                      textRole="none"
                    >
                      <span className="grid min-w-0 gap-1">
                        <span className="break-words font-semibold">
                          {item.stableName}
                        </span>
                        <span className="break-words text-sm text-on-surface-variant">
                          {item.description.status === "available"
                            ? item.description.value
                            : messages.descriptionUnavailable}
                        </span>
                      </span>
                    </StandardListSelectionItem>
                  </li>
                );
              })}
            </ul>
          </StandardListSelection>
        </nav>
        <SurfacePanel className="min-w-0 lg:col-span-2">
          {state.selection.status === "loading" ? (
            <CatalogStatus>{messages.detailLoading}</CatalogStatus>
          ) : state.selection.status === "error" ? (
            <CatalogStatus role="alert" tone="danger">
              {messages.detailError}
            </CatalogStatus>
          ) : (
            <PackagedFactoryDetail
              copyText={copyText}
              detail={state.selection.detail}
              headingID={detailHeadingID}
              messages={messages}
            />
          )}
        </SurfacePanel>
      </div>
    </section>
  );
}
