FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /napstarr .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /napstarr /usr/local/bin/napstarr
EXPOSE 8484
ENTRYPOINT ["napstarr"]
