FROM golang:1.24 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build for x86_64 (amd64) instead of ARM64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bootstrap .

# Runtime - use x86_64 Lambda base image
FROM public.ecr.aws/lambda/provided:al2
# Copy to Lambda task root directory
COPY --from=builder /build/bootstrap ${LAMBDA_TASK_ROOT}/bootstrap
CMD ["bootstrap"]