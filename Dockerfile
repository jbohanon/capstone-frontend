FROM golang:1.17-alpine AS builder
LABEL maintainer="Jacob Bohanon <jacobbohanon@gmail.com>"
WORKDIR /build/copy
COPY capstone/ ./capstone
WORKDIR /build
COPY go.mod .
COPY go.sum .
COPY *.go .
RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /build/copy/capstone-frontend
FROM scratch
WORKDIR /app
COPY --from=builder /build/copy/ /app/
EXPOSE 8080
ENTRYPOINT ["/app/capstone-frontend"]