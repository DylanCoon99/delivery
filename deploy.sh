#!/bin/bash

# Configuration
AWS_REGION="us-east-2"
AWS_ACCOUNT_ID="120569606269"
ECR_REPO_NAME="lead-ship-lambda"
IMAGE_TAG="latest"

# Full ECR repository URI
ECR_URI="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPO_NAME}"

echo "🔐 Authenticating with ECR..."
aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

echo "🏗️  Building Docker image..."
docker build --platform linux/amd64 -t ${ECR_REPO_NAME}:${IMAGE_TAG} .

echo "🏷️  Tagging image..."
docker tag ${ECR_REPO_NAME}:${IMAGE_TAG} ${ECR_URI}:${IMAGE_TAG}

echo "⬆️  Pushing to ECR..."
docker push ${ECR_URI}:${IMAGE_TAG}

echo "✅ Successfully pushed ${ECR_URI}:${IMAGE_TAG}"
echo ""
echo "📝 Use this image URI in your ECS task definition:"
echo "${ECR_URI}:${IMAGE_TAG}"