# 三阶段：前端构建 → Go 构建（嵌入 dist）→ distroless 运行
FROM node:24-alpine AS web
WORKDIR /web
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build-only

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tf-console .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/tf-console /app/tf-console
EXPOSE 8301
ENTRYPOINT ["/app/tf-console"]
