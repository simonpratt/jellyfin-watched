# 03: Container image and CI pipeline

**What to build:** The app ships as a container image built by CI. A multi-stage Dockerfile builds a static binary and puts it in a minimal runtime image that runs as a non-root user and includes CA certificates. A GitHub Actions workflow tests every change and publishes multi-arch images to ghcr.io. An example `docker-compose.yml` shows how to run it.

**Blocked by:** 01 (Walking skeleton)

**Status:** ready-for-agent

- [ ] The Dockerfile is multi-stage, the final image is minimal (distroless/static or scratch plus CA certs), and it runs as non-root
- [ ] The image starts with only environment variables set
- [ ] The workflow runs `go vet` and `go test` on every push and pull request
- [ ] Pushes to the main branch publish `ghcr.io/...:latest` and `:<commit-sha>` for linux/amd64 and linux/arm64
- [ ] `v*` tags also publish the image under that version tag
- [ ] An example `docker-compose.yml` is committed, with every variable from REQUIREMENTS.md §8 shown or commented
- [ ] Note: the repo has no GitHub remote yet, so checking publication end to end needs one
