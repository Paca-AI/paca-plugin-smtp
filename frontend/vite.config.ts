import react from "@vitejs/plugin-react";
import federation from "@originjs/vite-plugin-federation";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [
    react(),
    federation({
      name: "com_paca_smtp",
      filename: "remoteEntry.js",
      exposes: {
        "./AdminSmtpSettingsPage": "./src/AdminSmtpSettingsPage.tsx",
        "./EmailPreferencesTab": "./src/EmailPreferencesTab.tsx"
      },
      shared: {
        react: { requiredVersion: "^19.0.0" },
        "react-dom": { requiredVersion: "^19.0.0" },
        "@tanstack/react-query": { requiredVersion: "^5.0.0" }
      }
    })
  ],
  build: {
    target: "esnext",
    minify: false,
    cssCodeSplit: false
  }
});
