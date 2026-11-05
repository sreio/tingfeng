FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.10-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
ARG VERSION=1.0.0
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /tingfeng ./cmd/tingfeng \
    && mkdir /data && chown 65532:65532 /data && chmod 700 /data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /tingfeng /tingfeng
COPY --from=build --chown=65532:65532 /data /data
USER 65532:65532
ENV TINGFENG_LISTEN=0.0.0.0:8080 TINGFENG_DATA_DIR=/data GOMEMLIMIT=96MiB GOGC=75 GOMAXPROCS=1
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/tingfeng", "healthcheck"]
ENTRYPOINT ["/tingfeng"]
CMD ["serve"]
