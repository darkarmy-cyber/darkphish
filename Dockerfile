# syntax=docker/dockerfile:1.7

FROM node:24.19.0-bookworm-slim AS frontend
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@11.19.0 --activate
COPY package.json pnpm-lock.yaml gulpfile.js webpack.config.js ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY static ./static
RUN pnpm run build

FROM golang:1.27.1-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/darkphish ./

FROM debian:13.1-slim AS runtime
RUN apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 65532 darkphish \
    && useradd --system --uid 65532 --gid darkphish --home-dir /opt/darkphish darkphish \
    && install -d -o darkphish -g darkphish /opt/darkphish /data /etc/darkphish

WORKDIR /opt/darkphish
COPY --from=backend --chown=65532:65532 /out/darkphish ./darkphish
COPY --from=backend --chown=65532:65532 /src/VERSION /src/LICENSE /src/NOTICE.md ./
COPY --from=backend --chown=65532:65532 /src/db ./db
COPY --from=backend --chown=65532:65532 /src/templates ./templates
COPY --from=backend --chown=65532:65532 /src/static/images ./static/images
COPY --from=backend --chown=65532:65532 /src/static/font ./static/font
COPY --from=backend --chown=65532:65532 /src/static/db ./static/db
COPY --from=backend --chown=65532:65532 /src/static/endpoint ./static/endpoint
COPY --from=backend --chown=65532:65532 /src/static/js/src/vendor/ckeditor ./static/js/src/vendor/ckeditor
COPY --from=frontend --chown=65532:65532 /src/static/js/dist ./static/js/dist
COPY --from=frontend --chown=65532:65532 /src/static/css/dist ./static/css/dist
COPY --chown=65532:65532 docker/config.json /etc/darkphish/config.json

USER 65532:65532
VOLUME ["/data"]
EXPOSE 3333 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["curl", "--fail", "--silent", "--show-error", "--insecure", "https://127.0.0.1:3333/healthz"]
ENTRYPOINT ["./darkphish"]
CMD ["--config", "/etc/darkphish/config.json"]
