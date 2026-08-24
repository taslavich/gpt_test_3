import "@fontsource/inter/300.css";
import "@fontsource/inter/400.css";
import "@fontsource/inter/500.css";
import "@fontsource/inter/600.css";
import "@fontsource/inter/700.css";
import "@fontsource/inter/cyrillic-300.css";
import "@fontsource/inter/cyrillic-400.css";
import "@fontsource/inter/cyrillic-500.css";
import "@fontsource/inter/cyrillic-600.css";
import "@fontsource/inter/cyrillic-700.css";
import "./fonts.css";
import { createRoot } from "react-dom/client";
import App from "./App.tsx";
import "./index.css";
import { captureUtmSourceFromUrl } from "./lib/utmSource";

// Global toggle for surfacing API error toasts in the UI.
// Flip to `false` to silence error toasts (errors will still log to console).
(window as Window & { error_showed?: boolean }).error_showed = true;

// Persist ?utm_source=... as soon as possible so it survives navigation
// between the landing page and the auth dialog.
captureUtmSourceFromUrl();

createRoot(document.getElementById("root")!).render(<App />);
