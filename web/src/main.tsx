import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { I18nProvider } from "@/i18n";
import "./index.css";

// 默认深色主题（终端工具风格）
document.documentElement.classList.add("dark");

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>
);
