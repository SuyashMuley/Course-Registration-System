# syntax=docker/dockerfile:1
# Go registration service. Build from the repository root:
#   docker build -f deploy/docker/Dockerfile -t crs-app .
# Owner: Santosh.

FROM golang:1.27-alpine AS build
WORKDIR /src

# Download modules first so this layer is cached when only source code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# Minimal runtime image: static binary only, no shell, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
