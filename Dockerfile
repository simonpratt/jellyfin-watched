# syntax=docker/dockerfile:1

# Build stage: compile a static binary. It runs on the build host's platform
# and cross-compiles for the target, so multi-arch builds need no emulation.
ARG GO_VERSION=1.26
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/jellyfin-watched .

# Runtime stage: distroless/static has CA certificates and tzdata, no shell,
# and a built-in non-root user (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/jellyfin-watched /jellyfin-watched
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/jellyfin-watched"]
