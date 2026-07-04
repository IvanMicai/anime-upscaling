# Changelog

## [1.11.1](https://github.com/IvanMicai/anime-upscaling/compare/v1.11.0...v1.11.1) (2026-07-04)

### Bug Fixes

* normalize anamorphic/interlaced sources to square pixels before video2x ([#57](https://github.com/IvanMicai/anime-upscaling/issues/57)) ([504283a](https://github.com/IvanMicai/anime-upscaling/commit/504283a6470bef79f7545f9c126c4f300c70e839))

## [1.11.0](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.7...v1.11.0) (2026-06-30)

### Features

* ship dashboard UX redesign ([#56](https://github.com/IvanMicai/anime-upscaling/issues/56)) ([59fe1cd](https://github.com/IvanMicai/anime-upscaling/commit/59fe1cde4cbc94d0667ec45326c34268b87e65df)), closes [#49](https://github.com/IvanMicai/anime-upscaling/issues/49)

## [1.10.7](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.6...v1.10.7) (2026-06-09)

### Bug Fixes

* **api:** check os.Remove error in optimize cleanup defer ([#40](https://github.com/IvanMicai/anime-upscaling/issues/40)) ([abb8462](https://github.com/IvanMicai/anime-upscaling/commit/abb8462a168ab13419503d94dcd7f2fee479bb9e))

## [1.10.6](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.5...v1.10.6) (2026-06-07)

### Bug Fixes

* two-phase optimize encode — keep sparse subtitle tracks out of the slow mux ([769a3d2](https://github.com/IvanMicai/anime-upscaling/commit/769a3d2d0267a592eedf282ee1be9a95dd74dd04))

## [1.10.5](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.4...v1.10.5) (2026-06-07)

### Bug Fixes

* **docker:** install pnpm via npm — corepack removed from node >= 25 ([5a26f00](https://github.com/IvanMicai/anime-upscaling/commit/5a26f00b81431f8c605382319dc97d9d46ca64ca)), closes [#27](https://github.com/IvanMicai/anime-upscaling/issues/27)

## [1.10.4](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.3...v1.10.4) (2026-06-07)

### Bug Fixes

* confine libx265 worker pool to per-stream thread budget ([b1bb339](https://github.com/IvanMicai/anime-upscaling/commit/b1bb339ebda2f1477670c0b98cb1e76045d57420))

## [1.10.3](https://github.com/IvanMicai/anime-upscaling/compare/v1.10.2...v1.10.3) (2026-06-04)

### Bug Fixes

* force A/V interleaving in optimize encode (-max_interleave_delta 0) ([#34](https://github.com/IvanMicai/anime-upscaling/issues/34)) ([10c9962](https://github.com/IvanMicai/anime-upscaling/commit/10c9962384fa1a320fae87313709a7c478da04ab))

This file is maintained automatically by
[semantic-release](https://github.com/semantic-release/semantic-release) from
[Conventional Commits](https://www.conventionalcommits.org/). New releases are
prepended above this note on each push to `main`; see
[docs/RELEASING.md](docs/RELEASING.md). Every release also appears on the
[GitHub Releases page](https://github.com/IvanMicai/anime-upscaling/releases).
