const featureChecks = [
  "Builds the embedded dashboard shell with Vite and React",
  "Keeps the committed UI bundle aligned with Go embed assets",
  "Leaves richer dashboard interactions to later UI stories",
];

const eventStreamPath = "/events";

export function App() {
  return (
    <main className="app-shell">
      <section className="hero-card">
        <p className="eyebrow">Agent Factory</p>
        <h1>Dashboard UI baseline</h1>
        <p className="lede">
          This branch restores the minimal embedded dashboard entrypoint so the
          documented UI build command is a real review gate in CI.
        </p>
        <p className="api-contract">
          Canonical event stream endpoint: <code>{eventStreamPath}</code>
        </p>
      </section>

      <section className="details-grid" aria-label="Current dashboard baseline">
        {featureChecks.map((item) => (
          <article className="detail-card" key={item}>
            <h2>{item}</h2>
          </article>
        ))}
      </section>
    </main>
  );
}
