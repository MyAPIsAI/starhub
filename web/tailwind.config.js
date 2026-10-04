export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: ["class", '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        background: "var(--canvas)",
        foreground: "var(--text)",
        primary: "var(--brand)",
        muted: "var(--muted)",
        border: "var(--border)",
      },
      borderRadius: { sm: "var(--radius-sm)", md: "var(--radius-md)" },
    },
  },
  plugins: [],
};
