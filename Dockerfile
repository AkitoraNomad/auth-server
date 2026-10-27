# Build the management SPA first so it can be embedded into the Go binary.
FROM node:22-alpine AS web
WORKDIR /src
ARG NPM_REGISTRY=https://registry.npmjs.org
RUN corepack enable \
 && COREPACK_NPM_REGISTRY="$NPM_REGISTRY" corepack prepare pnpm@11.3.0 --activate \
 && pnpm --version
COPY auth-server-webui/package.json auth-server-webui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile --registry="$NPM_REGISTRY"
COPY auth-server-webui/ ./
RUN pnpm build

# Build the Go server with the SPA assets from the web stage.
FROM golang:1.26 AS builder
ENV GO111MODULE=on
ENV GOPROXY="https://goproxy.io,direct"
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/dist ./auth-server-webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -tags embedspa -a -installsuffix cgo -o auth-server .

FROM alpine:latest

COPY --from=builder /app/auth-server .
COPY --from=builder /app/template ./template/
COPY --from=builder /app/assets ./assets/
COPY placeholder.jpg .

ENV GIN_MODE=release
EXPOSE 8080
CMD ["./auth-server"]
