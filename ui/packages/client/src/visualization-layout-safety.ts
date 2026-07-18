import { marked, type Token } from "marked";
import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

type InputPath = readonly (string | number)[];

export const MIN_ANNOTATION_COORDINATE = -100_000;
export const MAX_ANNOTATION_COORDINATE = 100_000;
export const MAX_ANNOTATION_DIMENSION = 10_000;
export const MAX_NOTE_TITLE_LENGTH = 160;
export const MAX_NOTE_BODY_LENGTH = 4_000;
export const MAX_EMPTY_STATE_TEXT_LENGTH = 500;

type UnsafeTextMatch = {
  code: "unsafe_html" | "unsafe_markdown" | "unsafe_uri";
  message: string;
};

function containsFormattedToken(tokens: readonly Token[]): boolean {
  for (const token of tokens) {
    if (
      token.type !== "paragraph" &&
      token.type !== "text" &&
      token.type !== "space"
    ) {
      return true;
    }
    if (
      "tokens" in token &&
      token.tokens &&
      containsFormattedToken(token.tokens)
    ) {
      return true;
    }
  }
  return false;
}

function containsMarkdown(value: string): boolean {
  // Undefined reference labels are plain text to a renderer, but still carry
  // Markdown syntax that could become active when combined with a definition.
  return (
    containsFormattedToken(marked.lexer(value, { gfm: true })) ||
    /!?\[[^\]\n]*\]\[[^\]\n]*\]/u.test(value)
  );
}

function unsafeTextMatch(value: string): UnsafeTextMatch | undefined {
  if (/<!--|<!doctype\b|<\/?[a-z][^>]*>/iu.test(value)) {
    return {
      code: "unsafe_html",
      message: "Expected inert plain text without HTML or media markup.",
    };
  }
  if (
    /(?:\b(?:https?|ftp|file|data|javascript|mailto):|\bwww\.)\S*/iu.test(value)
  ) {
    return {
      code: "unsafe_uri",
      message: "Expected inert plain text without URI-bearing content.",
    };
  }
  if (containsMarkdown(value)) {
    return {
      code: "unsafe_markdown",
      message: "Expected inert plain text without Markdown or links.",
    };
  }
  return undefined;
}

export function validatePlainText(
  value: string,
  path: InputPath,
  maxLength: number,
  label: string,
  issues: FactoryVisualizationLayoutIssue[],
  requireNonWhitespace: boolean,
): void {
  if (requireNonWhitespace && value.trim().length === 0) {
    issues.push({
      category: "semantic",
      code: "empty_text",
      path,
      message: `Expected ${label} to contain non-whitespace text.`,
    });
  }
  if ([...value].length > maxLength) {
    issues.push({
      category: "semantic",
      code: "text_too_long",
      path,
      message: `Expected ${label} to contain at most ${maxLength} characters.`,
    });
  }
  const unsafe = unsafeTextMatch(value);
  if (unsafe) {
    issues.push({
      category: "semantic",
      code: unsafe.code,
      path,
      message: unsafe.message,
    });
  }
}

export function isUnsafeContentField(field: string): boolean {
  return /^(?:callback|command|connection|connections|edge|edges|event|handler|href|html|link|markdown|onclick|route|script|src|uri|url)$/iu.test(
    field,
  );
}

export function isInvalidCoordinate(value: number): boolean {
  return (
    !Number.isFinite(value) ||
    value < MIN_ANNOTATION_COORDINATE ||
    value > MAX_ANNOTATION_COORDINATE
  );
}

export function isInvalidDimension(value: number): boolean {
  return (
    !Number.isFinite(value) || value <= 0 || value > MAX_ANNOTATION_DIMENSION
  );
}

export function validateDuplicateAnnotationIds(
  annotations: readonly unknown[],
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const indexesById = new Map<string, number[]>();
  for (const [index, annotation] of annotations.entries()) {
    if (
      typeof annotation !== "object" ||
      annotation === null ||
      !("id" in annotation) ||
      typeof annotation.id !== "string"
    ) {
      continue;
    }
    const indexes = indexesById.get(annotation.id) ?? [];
    indexes.push(index);
    indexesById.set(annotation.id, indexes);
  }
  for (const [id, indexes] of indexesById) {
    if (indexes.length < 2) continue;
    for (const index of indexes) {
      issues.push({
        category: "semantic",
        code: "duplicate_annotation_id",
        path: ["annotations", index, "id"],
        message: `Annotation ID ${id} is duplicated at annotation indexes ${indexes.join(", ")}.`,
      });
    }
  }
}

export function validateDuplicateNodeIds(
  emptyStates: readonly unknown[],
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const indexesById = new Map<string, number[]>();
  for (const [index, emptyState] of emptyStates.entries()) {
    if (
      typeof emptyState !== "object" ||
      emptyState === null ||
      !("nodeId" in emptyState) ||
      typeof emptyState.nodeId !== "string" ||
      emptyState.nodeId.trim().length === 0
    ) {
      continue;
    }
    const indexes = indexesById.get(emptyState.nodeId) ?? [];
    indexes.push(index);
    indexesById.set(emptyState.nodeId, indexes);
  }
  for (const [nodeId, indexes] of indexesById) {
    if (indexes.length < 2) continue;
    for (const index of indexes) {
      issues.push({
        category: "semantic",
        code: "duplicate_node_id",
        path: ["nodeEmptyStates", index, "nodeId"],
        message: `Canonical node ID ${nodeId} is duplicated at empty-state indexes ${indexes.join(", ")}.`,
      });
    }
  }
}
