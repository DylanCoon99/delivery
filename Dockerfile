# Build stage
FROM golang:1.24 AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build for Lambda
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap main.go

# Runtime stage
FROM public.ecr.aws/lambda/provided:al2

# Copy the binary from builder
COPY --from=builder /build/bootstrap ${LAMBDA_TASK_ROOT}/bootstrap

# Set the CMD to your handler
CMD ["bootstrap"]