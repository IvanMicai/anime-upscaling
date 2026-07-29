// Builds the GitHub Pages site into site/dist.
//
// The landing page is hand-written HTML; the docs pages are rendered from the
// markdown that already lives in the repo, so there is exactly one copy of the
// documentation and it cannot drift from what ships in the tree.

import { readFile, writeFile, mkdir, rm, cp } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Marked } from "marked";

const SITE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(SITE, "..");
const SRC = path.join(SITE, "src");
const DIST = path.join(SITE, "dist");

const REPO = "https://github.com/IvanMicai/anime-upscaling";
const BLOB = `${REPO}/blob/main`;

/** Docs pages, in sidebar order. `src` is relative to the repository root. */
const PAGES = [
  { slug: "index", title: "Overview", src: "site/src/docs-index.md" },
  { slug: "architecture", title: "Architecture", src: "docs/ARCHITECTURE.md" },
  { slug: "deployment", title: "Deployment", src: "docs/DEPLOYMENT.md" },
  { slug: "api", title: "API reference", src: "apps/api/README.md" },
  { slug: "releasing", title: "Releasing", src: "docs/RELEASING.md" },
  { slug: "contributing", title: "Contributing", src: "CONTRIBUTING.md" },
];

/** Repo-relative markdown path -> page on this site. */
const ROUTES = {
  "README.md": "../",
  "CONTRIBUTING.md": "contributing.html",
  "docs/ARCHITECTURE.md": "architecture.html",
  "docs/DEPLOYMENT.md": "deployment.html",
  "docs/RELEASING.md": "releasing.html",
  "apps/api/README.md": "api.html",
  "apps/web/README.md": `${BLOB}/apps/web/README.md`,
};

const ASSETS = [
  ["docs/screenshots/pipeline-editor.png", "pipeline-editor.png"],
  ["docs/screenshots/file-picker.png", "file-picker.png"],
  ["docs/screenshots/job-detail.png", "job-detail.png"],
  ["apps/web/public/logo.png", "logo.png"],
  ["apps/web/public/favicon/favicon-96x96.png", "favicon.png"],
];

/**
 * GitHub-compatible heading slugs, so the tables of contents already written
 * into the markdown keep resolving. GitHub turns every single space into a
 * hyphen rather than collapsing runs, so "Why Go + Next.js" becomes
 * "why-go--nextjs" with two hyphens — matching that exactly matters.
 */
function slugify(text) {
  return text
    .trim()
    .toLowerCase()
    .replace(/[^\w\- ]+/g, "")
    .replace(/ /g, "-");
}

/**
 * Rewrites a relative markdown link so it resolves on the published site:
 * links to documents we publish become site pages, everything else points at
 * the file on GitHub.
 */
function rewriteHref(href, srcDir) {
  if (!href) return href;
  if (/^(https?:|mailto:|#|\/\/)/.test(href)) return href;

  const [target, hash = ""] = href.split("#");
  if (!target) return href;

  // Already a page on this site (docs-index.md links to its siblings that way).
  if (target.endsWith(".html")) return href;

  const repoPath = path
    .normalize(path.join(srcDir, target))
    .replace(/\\/g, "/")
    .replace(/^\.\//, "");

  const route = ROUTES[repoPath];
  if (route) return hash ? `${route}#${hash}` : route;

  return `${BLOB}/${repoPath}${hash ? `#${hash}` : ""}`;
}

function renderNav(currentSlug) {
  const items = PAGES.map((p) => {
    const href = p.slug === "index" ? "./" : `./${p.slug}.html`;
    const current = p.slug === currentSlug ? ' aria-current="page"' : "";
    return `<li><a href="${href}"${current}>${p.title}</a></li>`;
  }).join("\n            ");

  return `<nav class="docnav" aria-label="Documentation">
          <h4>Documentation</h4>
          <ul>
            ${items}
          </ul>
          <h4>Elsewhere</h4>
          <ul>
            <li><a href="../">Home</a></li>
            <li><a href="${REPO}">GitHub</a></li>
            <li><a href="https://hub.docker.com/r/ivanmicai/anime-upscaling-app">Docker Hub</a></li>
          </ul>
        </nav>`;
}

function shell({ title, description, nav, body, editPath }) {
  const edit = editPath
    ? `<a href="${BLOB}/${editPath}">Edit this page on GitHub</a>`
    : "";
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${title} — Anime Upscaling</title>
    <meta name="description" content="${description}" />
    <link rel="icon" href="../assets/favicon.png" type="image/png" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link
      href="https://fonts.googleapis.com/css2?family=Archivo:wdth,wght@62..125,400..700&family=JetBrains+Mono:wght@400;500&display=swap"
      rel="stylesheet"
    />
    <link rel="stylesheet" href="../styles.css" />
  </head>
  <body>
    <a class="skip-link" href="#main">Skip to content</a>
    <header class="masthead">
      <div class="masthead__inner">
        <a class="brand" href="../">
          <img src="../assets/logo.png" alt="" />
          <span>
            <span class="brand__name">Anime Upscaling</span>
            <span class="brand__sub">video restoration</span>
          </span>
        </a>
        <nav>
          <a href="./">Docs</a>
          <a class="is-secondary" href="../#quick-start">Quick start</a>
          <a href="${REPO}">GitHub</a>
        </nav>
      </div>
    </header>

    <div class="docs">
      ${nav}
      <main class="prose" id="main">
${body}
        <p class="doc-foot">${edit}</p>
      </main>
    </div>
  </body>
</html>
`;
}

async function renderMarkdown(md, srcDir) {
  // A fresh instance per page. `marked.use()` on the shared singleton would
  // stack one walkTokens hook per page, and every later page would then rewrite
  // its links once per hook — each pass resolving an already-rewritten href
  // against the wrong source directory.
  const md2html = new Marked({
    gfm: true,
    walkTokens(token) {
      if (token.type === "link" || token.type === "image") {
        token.href = rewriteHref(token.href, srcDir);
      }
    },
  });

  let html = await md2html.parse(md);

  // Anchor ids on headings, matching GitHub's slugs so existing tables of
  // contents keep resolving.
  const seen = new Map();
  html = html.replace(
    /<h([1-4])>([\s\S]*?)<\/h\1>/g,
    (_m, level, inner) => {
      const base = slugify(inner.replace(/<[^>]*>/g, ""));
      const n = seen.get(base) ?? 0;
      seen.set(base, n + 1);
      const id = n === 0 ? base : `${base}-${n}`;
      return `<h${level} id="${id}">${inner}</h${level}>`;
    },
  );

  // Tables scroll inside their own container instead of pushing the page wide.
  html = html
    .replace(/<table>/g, '<div class="tablewrap"><table>')
    .replace(/<\/table>/g, "</table></div>");

  return html;
}

function firstParagraph(md) {
  const line = md
    .split("\n")
    .map((l) => l.trim())
    .find((l) => l && !l.startsWith("#") && !l.startsWith("<") && !l.startsWith("["));
  return (line ?? "Anime Upscaling documentation")
    .replace(/[[\]`*]/g, "")
    .replace(/\(([^)]*)\)/g, "")
    .replace(/"/g, "&quot;")
    .slice(0, 180);
}

async function build() {
  await rm(DIST, { recursive: true, force: true });
  await mkdir(path.join(DIST, "docs"), { recursive: true });
  await mkdir(path.join(DIST, "assets"), { recursive: true });

  // Landing page + stylesheet.
  await cp(path.join(SRC, "index.html"), path.join(DIST, "index.html"));
  await cp(path.join(SRC, "styles.css"), path.join(DIST, "styles.css"));

  for (const [from, to] of ASSETS) {
    await cp(path.join(ROOT, from), path.join(DIST, "assets", to));
  }

  for (const page of PAGES) {
    const abs = path.join(ROOT, page.src);
    const md = await readFile(abs, "utf8");
    const srcDir = path.dirname(page.src);
    const body = await renderMarkdown(md, srcDir);

    const html = shell({
      title: page.title,
      description: firstParagraph(md),
      nav: renderNav(page.slug),
      body,
      editPath: page.src.startsWith("site/") ? null : page.src,
    });

    const out = page.slug === "index" ? "index.html" : `${page.slug}.html`;
    await writeFile(path.join(DIST, "docs", out), html);
  }

  // Pages is served by the deploy action, not Jekyll.
  await writeFile(path.join(DIST, ".nojekyll"), "");

  await checkLinks();

  console.log(`Built ${PAGES.length + 1} pages into ${path.relative(ROOT, DIST)}`);
}

/**
 * Fails the build on links that resolve to nothing. External URLs cannot be
 * checked offline, but the two ways link rewriting has actually broken here can:
 * a published page leaking out as a GitHub blob URL, and an internal target that
 * does not exist on disk.
 */
async function checkLinks() {
  const pages = [
    ["index.html", path.join(DIST, "index.html")],
    ...PAGES.map((p) => {
      const name = p.slug === "index" ? "index.html" : `${p.slug}.html`;
      return [`docs/${name}`, path.join(DIST, "docs", name)];
    }),
  ];

  const problems = [];

  for (const [label, file] of pages) {
    const html = await readFile(file, "utf8");
    const dir = path.dirname(file);
    const ids = new Set(
      [...html.matchAll(/\bid="([^"]+)"/g)].map((m) => m[1]),
    );

    for (const [, href] of html.matchAll(/href="([^"]+)"/g)) {
      // A doc we publish must never be linked as a file on GitHub.
      if (href.startsWith(BLOB) && href.endsWith(".html")) {
        problems.push(`${label}: publishes as a GitHub blob link — ${href}`);
        continue;
      }
      if (/^(https?:|mailto:|\/\/)/.test(href)) continue;

      if (href.startsWith("#")) {
        if (!ids.has(href.slice(1))) {
          problems.push(`${label}: anchor has no matching id — ${href}`);
        }
        continue;
      }

      const [target, hash] = href.split("#");
      if (!target) continue;
      const resolved = path.resolve(
        dir,
        target.endsWith("/") ? `${target}index.html` : target,
      );
      if (!existsSync(resolved)) {
        problems.push(`${label}: target does not exist — ${href}`);
        continue;
      }
      if (hash && resolved.endsWith(".html")) {
        const targetHtml = await readFile(resolved, "utf8");
        if (!new RegExp(`\\bid="${hash.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}"`).test(targetHtml)) {
          problems.push(`${label}: anchor has no matching id — ${href}`);
        }
      }
    }
  }

  if (problems.length) {
    throw new Error(`Broken links:\n  ${problems.join("\n  ")}`);
  }
  console.log(`Link check passed across ${pages.length} pages.`);
}

build().catch((err) => {
  console.error(err);
  process.exit(1);
});
