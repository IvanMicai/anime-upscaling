import {
  inputFilesResponse,
  emptyFilesResponse,
  nestedFilesResponse,
  manyFilesResponse,
} from "../components/__fixtures__/files";
import { samplePipeline } from "../components/__fixtures__/pipelines";
import { sampleJobs } from "../components/__fixtures__/jobs";
import type { Settings } from "../lib/types";

const settings: Settings = {
  streams_per_gpu: 2,
  ffmpeg_streams: 1,
  gpu_count: 1,
  gpu_vendor: "nvidia",
};

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

const originalFetch =
  typeof window !== "undefined" ? window.fetch.bind(window) : undefined;

if (typeof window !== "undefined") {
  window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;

    if (url.startsWith("/api/jobs") && (!init || init.method === "GET" || !init.method)) {
      if (url === "/api/jobs") return jsonResponse(sampleJobs);
      const tail = url.replace("/api/jobs/", "");
      const [idAndExtra] = tail.split("?");
      const segments = idAndExtra.split("/");
      if (segments[1] === "logs") {
        const params = new URL(url, "http://localhost").searchParams;
        const since = parseInt(params.get("since") ?? "0", 10) || 0;
        const all = (await import("../components/__fixtures__/jobs")).sampleLogs;
        const entries = since < all.length ? all.slice(since) : [];
        return jsonResponse({ entries, total: all.length, running: false });
      }
      const job = sampleJobs.find((j) => j.id === segments[0]) ?? sampleJobs[0];
      return jsonResponse(job);
    }
    if (url.startsWith("/api/jobs") && init?.method === "DELETE") {
      return jsonResponse({}, { status: 204 });
    }
    if (url.startsWith("/api/jobs") && init?.method === "POST") {
      return jsonResponse(sampleJobs[0]);
    }

    if (url.startsWith("/api/files") && (!init || init.method === "GET" || !init.method)) {
      const params = new URL(url, "http://localhost").searchParams;
      const path = params.get("path") ?? "";
      if (path === "season-long" || path.startsWith("season-long/"))
        return jsonResponse(manyFilesResponse);
      if (path.includes("season")) return jsonResponse(nestedFilesResponse);
      return jsonResponse(inputFilesResponse);
    }
    if (url.startsWith("/api/files") && init?.method === "DELETE") {
      return jsonResponse({ deleted: 1, errors: [] });
    }

    if (url === "/api/pipelines" && (!init || init.method === "GET")) {
      return jsonResponse([samplePipeline]);
    }
    if (url.startsWith("/api/pipelines")) {
      return jsonResponse(samplePipeline);
    }

    if (url.startsWith("/api/merge/locate")) {
      const t = Number(new URL(url, "http://x").searchParams.get("t") ?? 0);
      // The first 100 s match with a lag; past that the mock "cannot find" the
      // moment, to show the warning state.
      const matched = t < 1000;
      return jsonResponse({
        location: { t_a: t, t_b: matched ? t - 10.5 : t, lag: matched ? -10.5 : 0, confidence: matched ? 41 : 3, matched },
        a: { width: 640, height: 480, bitrate: 1_200_000, duration: 1342 },
        b: { width: 1280, height: 960, bitrate: 1_900_000, duration: 1293 },
      });
    }
    if (url === "/api/merge/preview") {
      return jsonResponse({
        pairs: [
          {
            a: "Show/EN/ep01.en.mkv", b: "Show/PT/ep01.pt-br.mp4", output: "Show/ep01.mkv",
            lang_a: { tag: "en", iso3: "eng", title: "English" },
            lang_b: { tag: "pt-br", iso3: "por", title: "Português (BR)" },
          },
          {
            a: "Show/EN/ep02.en.mkv", b: "Show/PT/ep02.pt-br.mp4", output: "Show/ep02.mkv", exists: true,
            lang_a: { tag: "en", iso3: "eng", title: "English" },
            lang_b: { tag: "pt-br", iso3: "por", title: "Português (BR)" },
          },
        ],
        unpaired: [
          { file: "Show/EN/extra.en.mkv", reason: "no file with the same name in another language" },
        ],
      });
    }
    if (url === "/api/settings") return jsonResponse(settings);

    if (url === "/api/logout") return jsonResponse({}, { status: 204 });

    void emptyFilesResponse;
    return originalFetch
      ? originalFetch(input as RequestInfo, init)
      : new Response("Not mocked", { status: 404 });
  }) as typeof fetch;
}
