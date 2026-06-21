FROM golang:1.26.4-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o agentmemory ./cmd/agentmemory/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/agentmemory /usr/local/bin/agentmemory
COPY --from=builder /app/migrations /migrations
EXPOSE 8080
ENTRYPOINT ["agentmemory"]
CMD ["serve"]
