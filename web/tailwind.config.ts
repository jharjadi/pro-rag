import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
    "./lib/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        background: "var(--background)",
        foreground: "var(--foreground)",
        surface: "var(--surface)",
        "surface-2": "var(--surface-2)",
        border: "var(--border-color)",
        accent: "var(--accent)",
        "accent-dim": "var(--accent-dim)",
        "text-dim": "var(--text-dim)",
      },
    },
  },
  plugins: [require("@tailwindcss/typography")],
};

export default config;
