FROM golang:1.23-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates tzdata && \
    apk upgrade

RUN adduser -D -s /bin/sh appuser

WORKDIR /app

COPY ContestManager/go.mod ContestManager/go.sum ./

RUN go mod download && go mod verify

COPY ContestManager/data ./data
COPY ContestManager/internal ./internal
COPY ContestManager/api ./api
COPY ContestManager/cmd ./cmd

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s' \
    -o server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s' \
    -o import-problems ./cmd/import-problems && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s' \
    -o delete-problem ./cmd/delete-problem

FROM alpine:latest

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /app

COPY --from=builder /app/server ./
COPY --from=builder /app/import-problems ./
COPY --from=builder /app/delete-problem ./

USER 65532:65532

EXPOSE 50051

CMD ["./server"]
