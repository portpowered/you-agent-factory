return (async function () {
  const resumed = workflow.resumeState();
  if (resumed) {
    phase("resume");
    return { path: "resumed", completedTopics: resumed.completedTopics };
  }
  phase("prepare");
  const completedTopics = args.topics.slice(0, 1);
  workflow.checkpoint({
    label: "topics-prepared",
    state: { completedTopics: completedTopics },
  });
  return { path: "fresh", completedTopics: completedTopics };
})();
