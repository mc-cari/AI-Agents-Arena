FROM golang:1.23-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates tzdata protoc protobuf-dev && \
    apk upgrade

RUN adduser -D -s /bin/sh appuser

WORKDIR /app

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY proto/ proto/

RUN mkdir -p api/grpc && \
    PATH=$PATH:$(go env GOPATH)/bin protoc \
        --go_out=api/grpc \
        --go_opt=paths=source_relative \
        --go-grpc_out=api/grpc \
        --go-grpc_opt=paths=source_relative \
        proto/contest.proto

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s' \
    -o server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s' \
    -o import-problems ./cmd/import-problems && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
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
