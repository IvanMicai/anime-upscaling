import { beforeAll } from "vitest";
import { setProjectAnnotations } from "@storybook/nextjs-vite";
import preview from "./preview";

const project = setProjectAnnotations([preview]);

beforeAll(project.beforeAll);
