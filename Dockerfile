# syntax=docker/dockerfile:1

ARG ONNXRUNTIME_VERSION=1.22.0

FROM node:22-bookworm AS frontend
WORKDIR /src/web/manager
COPY web/manager/package.json web/manager/package-lock.json ./
RUN npm ci
COPY web/manager/ ./
RUN npm run build

FROM golang:1.26-bookworm AS builder
ARG ONNXRUNTIME_VERSION
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl g++ libopus-dev libopusfile-dev portaudio19-dev \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}.tgz" \
    | tar -xz -C /opt
ENV CGO_CFLAGS="-I/opt/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/include"
ENV CGO_LDFLAGS="-L/opt/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/lib -lonnxruntime"
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /out/manager ./cmd/manager && go build -o /out/wsserver ./cmd/wsserver

FROM debian:bookworm-slim AS runtime
ARG ONNXRUNTIME_VERSION
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl libgomp1 libopus0 libopusfile0 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --create-home orion
COPY --from=builder /out/manager /usr/local/bin/manager
COPY --from=builder /out/wsserver /usr/local/bin/wsserver
COPY --from=builder /opt/onnxruntime-linux-x64-${ONNXRUNTIME_VERSION}/lib/libonnxruntime.so* /usr/local/lib/
RUN ldconfig
USER orion
CMD ["manager"]

FROM nginx:1.27-alpine AS nginx
COPY --from=frontend /src/web/manager/dist/ /usr/share/nginx/html/
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
