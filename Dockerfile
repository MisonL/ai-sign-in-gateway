# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-builder
WORKDIR /build/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/ai-sign-in-gateway ./cmd/ai-sign-in-gateway

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /out/ai-sign-in-gateway /app/ai-sign-in-gateway
COPY --from=frontend-builder /build/frontend/dist /app/frontend/dist

RUN mkdir -p /app/data

ENV APP_NAME="爱签网关" \
    AI_SIGN_IN_GATEWAY_HOST=0.0.0.0 \
    AI_SIGN_IN_GATEWAY_PORT=8972 \
    AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false \
    AI_SIGN_IN_GATEWAY_CONFIG_DIR=/app/data \
    DATABASE_URL=sqlite:////app/data/ai-sign-in-gateway.db \
    CORS_ORIGINS=http://localhost:8972,http://127.0.0.1:8972

EXPOSE 8972

CMD ["/app/ai-sign-in-gateway"]
