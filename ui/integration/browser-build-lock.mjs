export async function runSharedBrowserBuild({
  build,
  buildCacheKey,
  buildState = globalThis,
  ready,
}) {
  if (!buildState[buildCacheKey]) {
    const buildPromise = Promise.resolve()
      .then(async () => {
        if (!(await ready())) {
          await build();
        }
      })
      .catch((error) => {
        if (buildState[buildCacheKey] === buildPromise) {
          delete buildState[buildCacheKey];
        }
        throw error;
      });
    buildState[buildCacheKey] = buildPromise;
  }

  await buildState[buildCacheKey];
}
