import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";

import "./index.css";
import App from "./App.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/* Enables URL-based navigation throughout the portal. */}
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
