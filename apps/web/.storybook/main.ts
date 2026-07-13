import type { StorybookConfig } from "@storybook/nextjs-vite";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dirname = path.dirname(fileURLToPath(import.meta.url));

const config: StorybookConfig = {
  stories: ["../components/**/*.stories.@(ts|tsx)"],
  addons: ["@storybook/addon-themes", "@storybook/addon-vitest"],
  framework: {
    name: "@storybook/nextjs-vite",
    options: {},
  },
  staticDirs: ["../public"],
  typescript: {
    reactDocgen: "react-docgen-typescript",
  },
  viteFinal: async (vite) => {
    vite.plugins = [...(vite.plugins ?? []), tailwindcss()];
    vite.resolve = vite.resolve ?? {};
    vite.resolve.alias = {
      ...(vite.resolve.alias as Record<string, string> | undefined),
      "@": path.resolve(dirname, ".."),
    };
    return vite;
  },
};

export default config;
