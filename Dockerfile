FROM golang:1.24 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bootstrap .

# Runtime
FROM public.ecr.aws/lambda/provided:al2-arm64

# Copy directly to /var/runtime
COPY --from=builder /build/bootstrap /var/runtime/bootstrap

CMD ["bootstrap"]