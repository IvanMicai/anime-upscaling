import type { Preview } from "@storybook/nextjs-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import "../app/globals.css";
import "./fetch-mock";

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    backgrounds: {
      default: "app",
      values: [
        { name: "app", value: "var(--background)" },
        { name: "white", value: "#ffffff" },
        { name: "black", value: "#000000" },
      ],
    },
    layout: "padded",
  },
  decorators: [
    withThemeByClassName({
      themes: { light: "", dark: "dark" },
      defaultTheme: "dark",
    }),
  ],
  initialGlobals: {
    backgrounds: { value: "app" },
  },
};

export default preview;
