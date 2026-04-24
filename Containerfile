# --- Build Stage ---
ARG GO_VERSION=1.25
ARG BUILD_TARGET=./cmd/agent

FROM docker.io/library/golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_TARGET
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app ${BUILD_TARGET}
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app-server ./cmd/server

# --- Runtime Stage ---
FROM docker.io/library/alpine:3.20
RUN apk add --no-cache git ca-certificates
COPY --from=build /app /usr/local/bin/gocoder
COPY --from=build /app-server /usr/local/bin/gocoder-server

# Server assets (prompts, templates, static files)
COPY prompts/ /app/prompts/
COPY web/templates/ /app/web/templates/
COPY web/static/ /app/web/static/

COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Default env vars pointing to baked-in asset paths
ENV ARDP_PROMPTS_DIR=/app/prompts
ENV ARDP_TEMPLATES_DIR=/app/web/templates
ENV ARDP_STATIC_DIR=/app/web/static
ENV ARDP_PORT=8080

WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
CMD ["sleep", "infinity"]
