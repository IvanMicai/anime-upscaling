# Releasing

Releases are created automatically from commits merged into `main`. The release
workflow uses semantic-release and Conventional Commits to decide the next
SemVer version, creates a GitHub release, then pushes the API and app images to
Docker Hub with matching tags.

## Docker Hub Setup

Create two Docker Hub repositories in your user or organization namespace:

- `anime-upscaling-api`
- `anime-upscaling-app`

Create a Docker Hub personal access token:

1. Sign in to Docker Hub.
2. Open Account settings.
3. Open Personal access tokens.
4. Generate a token with `Read & Write` access.
5. Copy the token immediately. Docker Hub does not show it again later.

In the GitHub repository, open Settings > Secrets and variables > Actions and
create:

| Type | Name | Value |
| --- | --- | --- |
| Repository variable | `DOCKERHUB_NAMESPACE` | Docker Hub username or organization, for example `my-user` |
| Repository secret | `DOCKERHUB_USERNAME` | Docker Hub username used to push images |
| Repository secret | `DOCKERHUB_TOKEN` | Docker Hub personal access token |

The workflow uses `GITHUB_TOKEN` for the GitHub release. Keep GitHub Actions
enabled with write access to repository contents so it can create tags and
releases.

## Version Rules

Use Conventional Commit messages:

| Commit message | Release |
| --- | --- |
| `fix: handle missing output folder` | Patch, for example `1.2.3` to `1.2.4` |
| `perf: reduce queue polling overhead` | Patch |
| `feat: add custom interpolation preset` | Minor, for example `1.2.3` to `1.3.0` |
| `feat!: change pipeline schema` | Major, for example `1.2.3` to `2.0.0` |
| Message with `BREAKING CHANGE:` footer | Major |
| `docs: update deployment guide` | No release by default |
| `chore: update tooling` | No release by default |

When using squash merge, make the pull request title follow the same convention
because GitHub uses it as the final commit title.

Add `[skip release]` or `[release skip]` to a commit message when a change should
be ignored by release analysis.

If the repository has no previous release tag, the first release starts at
`v1.0.0`.

## Published Docker Tags

Each release publishes both images:

- `${DOCKERHUB_NAMESPACE}/anime-upscaling-api`
- `${DOCKERHUB_NAMESPACE}/anime-upscaling-app`

For version `1.2.3`, the workflow publishes:

- `1.2.3`
- `1.2`
- `1`
- `latest`

Use exact tags such as `1.2.3` for stable deployments and rollback. Use
`latest` only when the server should always pull the newest release.

## Deploy From Docker Hub

Prepare `.env` as usual:

```bash
cp .env.example .env
mkdir -p data/input data/output data/optimized data/interpolated data/temp
```

Add the image namespace and desired release tag to `.env`:

```bash
DOCKERHUB_NAMESPACE=my-user
IMAGE_TAG=1.2.3
```

Start with published images:

```bash
docker compose -f docker-compose.hub.yml pull
docker compose -f docker-compose.hub.yml up -d
```

Start with the NVIDIA GPU overlay:

```bash
docker compose -f docker-compose.hub.yml -f docker-compose.nvidia.yml pull
docker compose -f docker-compose.hub.yml -f docker-compose.nvidia.yml up -d
```

To upgrade, change `IMAGE_TAG` and run `pull` and `up -d` again.

## Manual Image Push

The automated workflow is preferred, but a manual push is useful for testing a
Docker Hub repository:

```bash
docker login --username my-user

docker build -t my-user/anime-upscaling-api:0.0.0-test packages/api
docker push my-user/anime-upscaling-api:0.0.0-test

docker build -t my-user/anime-upscaling-app:0.0.0-test packages/app
docker push my-user/anime-upscaling-app:0.0.0-test
```
