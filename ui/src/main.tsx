import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./styles.css";
import { dashboardComponentsPackageName } from "./lib/components-package-resolution";
import { App } from "./App";

void dashboardComponentsPackageName;

const container = document.getElementById("root");
const queryClient = new QueryClient();

if (container === null) {
  throw new Error("dashboard root container is missing");
}

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
