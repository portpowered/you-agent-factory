const root = document.getElementById("root");

if (root && root.childElementCount === 0) {
  root.innerHTML =
    '<main style="padding: 1.5rem; font-family: ui-sans-serif, system-ui, sans-serif;">' +
    "<h1>Agent Factory Dashboard</h1>" +
    "<p>The production dashboard assets are not built in this checkout yet.</p>" +
    "<p>Run <code>make ui-build</code> to generate the real embedded UI bundle.</p>" +
    "</main>";
}
