FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /goTOV ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /goTOV .

ENV GOTOV_SERVER_PORT=8085
EXPOSE ${GOTOV_SERVER_PORT}

ENTRYPOINT ["./goTOV"]
