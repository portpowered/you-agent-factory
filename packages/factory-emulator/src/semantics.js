/**
 * Pure rule analysis and resolution for scenarios that have passed the public
 * parser. These helpers never create Factory events or emulator activity.
 */
export function scenarioSemanticDiagnostics(scenario, factory) {
  const diagnostics = [];
  const initialSubmissions = scenario.initialSubmissions ?? [];
  const factoryWorkTypes = factoryWorkTypeNames(factory);
  const initialSubmissionIndexes = indexesById(initialSubmissions);
  const ruleIndexes = indexesById(scenario.rules);
  const cyclicLineageOutcomeKeys = cyclicScriptedOutcomeKeys(
    scenario.rules,
    ruleIndexes,
  );

  duplicateIdDiagnostics(
    initialSubmissionIndexes,
    "/initialSubmissions",
    "initial submission",
    diagnostics,
  );
  duplicateIdDiagnostics(ruleIndexes, "/rules", "rule", diagnostics);

  for (const [index, submission] of initialSubmissions.entries()) {
    if (!factoryWorkTypes.has(submission.workType)) {
      diagnostics.push(
        diagnostic(
          "UNKNOWN_FACTORY_WORK_TYPE",
          `/initialSubmissions/${index}/workType`,
          `Initial submission work type ${JSON.stringify(submission.workType)} is not available in the Factory definition.`,
          "a work type declared by the Factory definition",
        ),
      );
    }
  }

  for (const [index, rule] of scenario.rules.entries()) {
    validateRuleMatcher(
      rule,
      index,
      factoryWorkTypes,
      initialSubmissionIndexes,
      diagnostics,
    );
    validateLineageCursors(
      rule,
      index,
      scenario.rules,
      ruleIndexes,
      initialSubmissionIndexes,
      cyclicLineageOutcomeKeys,
      diagnostics,
    );

    for (let earlierIndex = 0; earlierIndex < index; earlierIndex += 1) {
      if (shadows(scenario.rules[earlierIndex], rule, initialSubmissions)) {
        diagnostics.push(
          diagnostic(
            "SHADOWED_RULE",
            `/rules/${index}`,
            `Rule ${JSON.stringify(rule.id)} at /rules/${index} is unreachable because earlier rule ${JSON.stringify(scenario.rules[earlierIndex].id)} at /rules/${earlierIndex} covers its supported match domain.`,
            `a match domain not covered by /rules/${earlierIndex}`,
          ),
        );
        break;
      }
    }
  }

  return diagnostics.sort(compareDiagnostics);
}

/** Returns the first authored rule that applies to a submission. */
export function selectEmulatorRule(scenario, submission) {
  return scenario.rules.find((rule) => ruleMatches(rule.match, submission));
}

/**
 * Resolves one zero-based matching invocation without mutating scenario state.
 * The invocation index counts prior matches for the selected rule only.
 */
export function resolveEmulatorScenarioResult(scenario, submission, invocationIndex) {
  if (!Number.isSafeInteger(invocationIndex) || invocationIndex < 0) {
    throw new RangeError("invocationIndex must be a zero-based safe integer.");
  }
  const rule = selectEmulatorRule(scenario, submission);
  if (rule === undefined) {
    return { kind: "unmatched", behavior: scenario.unmatchedBehavior };
  }
  if (invocationIndex < rule.outcomes.length) {
    return { kind: "outcome", rule, outcome: rule.outcomes[invocationIndex] };
  }
  if (rule.exhaustionBehavior.kind === "repeatLast") {
    return {
      kind: "outcome",
      rule,
      outcome: rule.outcomes[rule.outcomes.length - 1],
    };
  }
  if (rule.exhaustionBehavior.kind === "useUnmatchedBehavior") {
    return { kind: "unmatched", behavior: scenario.unmatchedBehavior };
  }
  return { kind: "exhausted", rule, behavior: rule.exhaustionBehavior };
}

function factoryWorkTypeNames(factory) {
  if (!isRecord(factory) || !Array.isArray(factory.workTypes)) {
    return new Set();
  }
  return new Set(
    factory.workTypes.flatMap((workType) =>
      isRecord(workType) && typeof workType.name === "string"
        ? [workType.name]
        : [],
    ),
  );
}

function indexesById(items) {
  const indexes = new Map();
  for (const [index, item] of items.entries()) {
    const values = indexes.get(item.id) ?? [];
    values.push(index);
    indexes.set(item.id, values);
  }
  return indexes;
}

function duplicateIdDiagnostics(indexes, rootPath, itemName, diagnostics) {
  for (const [id, occurrences] of indexes) {
    if (occurrences.length > 1) {
      for (const index of occurrences.slice(1)) {
        diagnostics.push(
          diagnostic(
            "DUPLICATE_SCENARIO_IDENTIFIER",
            `${rootPath}/${index}/id`,
            `${itemName[0].toUpperCase()}${itemName.slice(1)} id ${JSON.stringify(id)} duplicates ${rootPath}/${occurrences[0]}/id.`,
            `a unique ${itemName} id`,
          ),
        );
      }
    }
  }
}

function validateRuleMatcher(
  rule,
  ruleIndex,
  factoryWorkTypes,
  initialSubmissionIndexes,
  diagnostics,
) {
  if (rule.match.kind === "workType" && !factoryWorkTypes.has(rule.match.workType)) {
    diagnostics.push(
      diagnostic(
        "UNKNOWN_FACTORY_WORK_TYPE",
        `/rules/${ruleIndex}/match/workType`,
        `Rule work type ${JSON.stringify(rule.match.workType)} is not available in the Factory definition.`,
        "a work type declared by the Factory definition",
      ),
    );
  }
  if (
    rule.match.kind === "submissionId" &&
    initialSubmissionIndexes.get(rule.match.submissionId)?.length !== 1
  ) {
    diagnostics.push(
      diagnostic(
        "UNKNOWN_INITIAL_SUBMISSION",
        `/rules/${ruleIndex}/match/submissionId`,
        `Rule submission id ${JSON.stringify(rule.match.submissionId)} does not identify one initial submission.`,
        "exactly one initial submission id",
      ),
    );
  }
}

function validateLineageCursors(
  rule,
  ruleIndex,
  rules,
  ruleIndexes,
  initialSubmissionIndexes,
  cyclicLineageOutcomeKeys,
  diagnostics,
) {
  for (const [outcomeIndex, outcome] of rule.outcomes.entries()) {
    if (outcome.kind !== "complete" || outcome.lineageCursor === undefined) {
      continue;
    }
    const cursor = outcome.lineageCursor;
    const path = `/rules/${ruleIndex}/outcomes/${outcomeIndex}/lineageCursor`;
    if (cursor.kind === "initialSubmission") {
      if (initialSubmissionIndexes.get(cursor.submissionId)?.length !== 1) {
        diagnostics.push(
          diagnostic(
            "MISSING_LINEAGE_CURSOR_TARGET",
            path,
            `Lineage cursor submission ${JSON.stringify(cursor.submissionId)} does not identify one initial submission.`,
            "exactly one initial submission id",
          ),
        );
      }
      continue;
    }

    if (cyclicLineageOutcomeKeys.has(outcomeKey(ruleIndex, outcomeIndex))) {
      diagnostics.push(
        diagnostic(
          "CYCLIC_LINEAGE_CURSOR",
          path,
          "Lineage cursor participates in a cycle and cannot resolve to a prior result.",
          "an acyclic reference to a previous complete scripted outcome",
        ),
      );
      continue;
    }

    const targetIndexes = ruleIndexes.get(cursor.ruleId);
    if (targetIndexes?.length !== 1) {
      diagnostics.push(
        diagnostic(
          "MISSING_LINEAGE_CURSOR_TARGET",
          path,
          `Lineage cursor rule ${JSON.stringify(cursor.ruleId)} does not identify one scripted rule.`,
          "exactly one earlier scripted rule id",
        ),
      );
      continue;
    }

    const targetRuleIndex = targetIndexes[0];
    if (
      targetRuleIndex > ruleIndex ||
      (targetRuleIndex === ruleIndex && cursor.outcomeIndex >= outcomeIndex)
    ) {
      diagnostics.push(
        diagnostic(
          "FORWARD_LINEAGE_CURSOR",
          path,
          `Lineage cursor targets /rules/${targetRuleIndex}/outcomes/${cursor.outcomeIndex}, which is not a previously scripted result.`,
          "a previous complete scripted outcome",
        ),
      );
      continue;
    }

    const targetOutcome = rules[targetRuleIndex].outcomes[cursor.outcomeIndex];
    if (targetOutcome?.kind !== "complete") {
      diagnostics.push(
        diagnostic(
          "INCOMPATIBLE_LINEAGE_CURSOR",
          path,
          `Lineage cursor target /rules/${targetRuleIndex}/outcomes/${cursor.outcomeIndex} is not a complete scripted outcome.`,
          "a previous complete scripted outcome",
        ),
      );
    }
  }
}

function cyclicScriptedOutcomeKeys(rules, ruleIndexes) {
  const links = new Map();
  for (const [ruleIndex, rule] of rules.entries()) {
    for (const [outcomeIndex, outcome] of rule.outcomes.entries()) {
      const cursor = outcome.kind === "complete" ? outcome.lineageCursor : undefined;
      if (cursor?.kind !== "scriptedOutcome") {
        continue;
      }
      const targetIndexes = ruleIndexes.get(cursor.ruleId);
      if (targetIndexes?.length !== 1) {
        continue;
      }
      const targetRuleIndex = targetIndexes[0];
      const targetOutcome = rules[targetRuleIndex].outcomes[cursor.outcomeIndex];
      if (targetOutcome?.kind === "complete") {
        links.set(
          outcomeKey(ruleIndex, outcomeIndex),
          outcomeKey(targetRuleIndex, cursor.outcomeIndex),
        );
      }
    }
  }

  const cyclic = new Set();
  for (const start of links.keys()) {
    const positions = new Map();
    const path = [];
    for (let current = start; current !== undefined; current = links.get(current)) {
      const cycleStart = positions.get(current);
      if (cycleStart !== undefined) {
        for (const cycleKey of path.slice(cycleStart)) {
          cyclic.add(cycleKey);
        }
        break;
      }
      positions.set(current, path.length);
      path.push(current);
    }
  }
  return cyclic;
}

function outcomeKey(ruleIndex, outcomeIndex) {
  return `${ruleIndex}:${outcomeIndex}`;
}

function shadows(earlierRule, laterRule, initialSubmissions) {
  const earlier = earlierRule.match;
  const later = laterRule.match;
  if (earlier.kind === "all") {
    return true;
  }
  if (earlier.kind === later.kind) {
    return (
      (earlier.kind === "workType" && earlier.workType === later.workType) ||
      (earlier.kind === "submissionId" &&
        earlier.submissionId === later.submissionId)
    );
  }
  return (
    earlier.kind === "workType" &&
    later.kind === "submissionId" &&
    initialSubmissions.some(
      (submission) =>
        submission.id === later.submissionId &&
        submission.workType === earlier.workType,
    )
  );
}

function ruleMatches(match, submission) {
  return (
    match.kind === "all" ||
    (match.kind === "workType" && match.workType === submission.workType) ||
    (match.kind === "submissionId" && match.submissionId === submission.id)
  );
}

function diagnostic(code, path, message, expectation) {
  return { code, path, message, expectation };
}

function compareDiagnostics(left, right) {
  return (
    left.path.localeCompare(right.path) ||
    left.code.localeCompare(right.code) ||
    left.expectation.localeCompare(right.expectation)
  );
}

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
