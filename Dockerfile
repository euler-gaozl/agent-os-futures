FROM node:24.19.0-alpine AS web
WORKDIR /source/web
RUN corepack enable && corepack prepare pnpm@11.25.0 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY web ./
RUN pnpm run build
FROM golang:1.27.2-alpine AS build
WORKDIR /source
ENV GOWORK=off GOPROXY=off CGO_ENABLED=0
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY futures ./futures
COPY internal ./internal
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/service ./cmd/futures && go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 agentos
WORKDIR /app
COPY --from=build /out/ /app/
COPY --from=web /source/web/dist /app/web
ENV LISTEN=0.0.0.0:8098 HEALTH_URL=http://127.0.0.1:8098/readyz FUTURES_WEB_DIR=/app/web
EXPOSE 8098
USER 10001
HEALTHCHECK --interval=15s --timeout=4s --start-period=10s CMD ["/app/healthcheck"]
ENTRYPOINT ["/app/service"]
CMD ["--config", "/run/service.json"]
