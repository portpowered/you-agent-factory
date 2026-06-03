#!/usr/bin/env node
/**
 * One-shot migrator: transitional af-* Tailwind classes and aliased CSS vars
 * in ui/src/features → Material role utilities (US-009).
 */
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const FEATURES_ROOT = join(import.meta.dirname, "../src/features");

/** Longest-first so compound tokens replace before shorter prefixes. */
const REPLACEMENTS = [
  // CSS variables (aliased in color-role-aliases.css)
  ["var(--color-af-accent-surface)", "var(--color-primary-container)"],
  ["var(--color-af-accent-border)", "var(--color-primary)"],
  ["var(--color-af-accent)", "var(--color-primary)"],
  ["var(--color-af-surface-raised)", "var(--color-surface-container-high)"],
  ["var(--color-af-surface-subtle)", "var(--color-surface-container-low)"],
  ["var(--color-af-surface)", "var(--color-surface)"],
  ["var(--color-af-background)", "var(--color-background)"],
  ["var(--color-af-border-strong)", "var(--color-outline-variant)"],
  ["var(--color-af-border)", "var(--color-outline)"],
  ["var(--color-af-text-muted)", "var(--color-on-surface-variant)"],
  ["var(--color-af-text-subtle)", "var(--color-on-surface-subtle)"],
  ["var(--color-af-text)", "var(--color-on-surface)"],
  ["var(--color-af-success)", "var(--color-success)"],
  ["var(--color-af-warning-text)", "var(--color-on-warning-container)"],
  ["var(--color-af-danger-text)", "var(--color-on-error-container)"],
  ["var(--color-af-info-text)", "var(--color-on-info-container)"],
  ["var(--color-af-edge-muted-soft)", "var(--color-outline)"],
  ["var(--color-af-edge-muted)", "var(--color-outline-variant)"],
  ["var(--color-af-graph-controls-text-hover)", "var(--color-on-surface)"],
  ["var(--color-af-graph-controls-text)", "var(--color-on-surface-variant)"],
  [
    "var(--color-af-graph-controls-button-surface)",
    "var(--color-surface-container-high)",
  ],
  ["var(--color-af-graph-controls-surface)", "var(--color-surface)"],
  ["var(--color-af-chart-selection-stroke)", "var(--color-primary)"],
  ["var(--color-af-chart-selection-fill)", "var(--color-primary-container)"],
  ["var(--color-af-chart-active-dot-stroke)", "var(--color-background)"],
  ["var(--color-af-chart-cursor)", "var(--color-outline-variant)"],
  // Tailwind utilities
  ["bg-af-surface-subtle", "bg-surface-container-low"],
  ["bg-af-surface-raised", "bg-surface-container-high"],
  ["bg-af-surface", "bg-surface"],
  ["bg-af-background", "bg-background"],
  ["bg-af-bg", "bg-background"],
  ["border-af-border-strong", "border-outline-variant"],
  ["border-af-border", "border-outline"],
  ["text-af-text-muted", "text-on-surface-variant"],
  ["text-af-text-subtle", "text-on-surface-subtle"],
  ["text-af-text-disabled", "text-on-surface-disabled"],
  ["text-af-text-inverse", "text-on-inverse"],
  ["text-af-text", "text-on-surface"],
  ["bg-af-accent-surface", "bg-primary-container"],
  ["border-af-accent-border", "border-primary"],
  ["text-af-accent-hover", "text-on-primary-container"],
  ["text-af-accent", "text-primary"],
  ["bg-af-accent", "bg-primary"],
  ["text-af-on-accent", "text-on-primary"],
  ["bg-af-success-surface", "bg-success-container"],
  ["text-af-success-text", "text-on-success-container"],
  ["text-af-on-success", "text-on-success"],
  ["bg-af-success", "bg-success"],
  ["bg-af-warning-surface", "bg-warning-container"],
  ["text-af-warning-text", "text-on-warning-container"],
  ["text-af-on-warning", "text-on-warning"],
  ["bg-af-warning", "bg-warning"],
  ["bg-af-danger-surface", "bg-error-container"],
  ["text-af-danger-text", "text-on-error-container"],
  ["text-af-on-danger", "text-on-error"],
  ["bg-af-danger", "bg-error"],
  ["bg-af-info-surface", "bg-info-container"],
  ["text-af-info-text", "text-on-info-container"],
  ["text-af-on-info", "text-on-info"],
  ["text-af-info", "text-info"],
  ["bg-af-info", "bg-info"],
  ["bg-af-worker-surface", "bg-tertiary-container"],
  ["text-af-worker-text", "text-on-tertiary-container"],
  ["text-af-worker", "text-tertiary"],
  ["bg-af-worker", "bg-tertiary"],
  ["text-af-success", "text-success"],
  ["text-af-danger", "text-error"],
  ["text-af-ink", "text-on-surface"],
  ["hover:border-af-accent", "hover:border-primary"],
  [
    "has-[:focus-visible]:border-af-accent",
    "has-[:focus-visible]:border-primary",
  ],
  ["has-[:checked]:border-af-accent", "has-[:checked]:border-primary"],
  ["border-af-accent", "border-primary"],
  ["bg-af-panel", "bg-surface-container-high"],
  ["border-af-info", "border-info"],
  ["var(--color-af-danger-surface)", "var(--color-error-container)"],
  ["var(--color-af-success-surface)", "var(--color-success-container)"],
  ["var(--color-af-text-inverse)", "var(--color-on-inverse)"],
  ["var(--color-af-info)", "var(--color-info)"],
  ["bg-af-border-strong", "bg-outline-variant"],
  ["border-af-surface", "border-surface"],
  ["text-af-warning", "text-warning"],
  ["stroke-af-border-strong", "stroke-outline-variant"],
  ["stroke-af-background", "stroke-background"],
  ["fill-af-text-subtle", "fill-on-surface-subtle"],
];

function walk(dir) {
  const entries = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      entries.push(...walk(path));
    } else if (/\.(tsx?|jsx?)$/.test(name)) {
      entries.push(path);
    }
  }
  return entries;
}

let filesChanged = 0;
for (const filePath of walk(FEATURES_ROOT)) {
  const original = readFileSync(filePath, "utf8");
  let next = original;
  for (const [from, to] of REPLACEMENTS) {
    next = next.split(from).join(to);
  }
  if (next !== original) {
    writeFileSync(filePath, next);
    filesChanged += 1;
  }
}

console.log(`Updated ${filesChanged} files under ui/src/features`);
