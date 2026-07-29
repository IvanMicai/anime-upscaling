# site

The GitHub Pages site: <https://ivanmicai.github.io/anime-upscaling/>

A landing page plus the documentation, deployed by
[`.github/workflows/pages.yml`](../.github/workflows/pages.yml) on every push to
`main` that touches `site/`, `docs/`, `apps/api/README.md` or `CONTRIBUTING.md`.

## How it fits together

`build.mjs` renders `site/dist`:

- **Landing page** — `src/index.html`, hand-written. It reuses the dashboard's
  visual language: black canvas, zinc hairlines, monospace chips, and the four
  operation colors (upscale blue, interpolate purple, optimize green, integrity
  red). Color always encodes an operation; it is never decoration.
- **Docs pages** — rendered from the markdown that already lives in the
  repository, so there is one copy of each document and the site cannot drift
  from what shipped. The page list and sidebar order live in `PAGES` in
  `build.mjs`.

Relative links inside those markdown files are rewritten on the way out:
documents that are published here become site pages, and everything else points
at the file on GitHub. Heading anchors use GitHub's slug rules so the tables of
contents already written into the markdown keep resolving.

## Working on it

```bash
pnpm install
pnpm build
npx serve dist        # or: python3 -m http.server 8777 --directory dist
```

To publish another markdown file, add it to `PAGES` — and, if other documents
link to it, to `ROUTES` so those links resolve to the site instead of GitHub.
