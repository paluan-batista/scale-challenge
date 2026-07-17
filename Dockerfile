# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY db ./db
COPY internal ./internal
COPY testdata ./testdata
COPY tests ./tests
COPY Makefile ./
COPY docker-compose.yml ./

RUN for binary in api worker simulator migrate seed; do \
      CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/$binary ./cmd/$binary; \
    done

FROM alpine:3.22 AS runtime
RUN addgroup -S app && adduser -S -G app -u 10001 app
USER app

FROM runtime AS api
COPY --from=builder --chown=app:app /out/api /app/api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/app/api", "healthcheck"]
ENTRYPOINT ["/app/api"]

FROM runtime AS worker
COPY --from=builder --chown=app:app /out/worker /app/worker
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/app/worker", "healthcheck"]
ENTRYPOINT ["/app/worker"]

FROM runtime AS simulator
COPY --from=builder --chown=app:app /out/simulator /app/simulator
COPY --from=builder --chown=app:app /src/testdata /app/testdata
ENTRYPOINT ["/app/simulator"]

FROM runtime AS migrate
COPY --from=builder --chown=app:app /out/migrate /app/migrate
ENTRYPOINT ["/app/migrate"]

FROM runtime AS seed
COPY --from=builder --chown=app:app /out/seed /app/seed
ENTRYPOINT ["/app/seed"]

FROM builder AS test
RUN apk add --no-cache make gcc musl-dev && addgroup -S app && adduser -S -G app -u 10001 app
ENV CGO_ENABLED=1
CMD ["make", "verify"]
