#!/bin/bash
# Push Docker image to AWS ECR repositories

set -e

ACCOUNT_ID=$1
REGION=${2:-us-east-1}
IMAGE_NAME="descheduler"
IMAGE_TAG=$3

if [ -z "$ACCOUNT_ID" ] || [ -z "$IMAGE_TAG" ]; then
    echo "Usage: $0 <account-id> [region] <image-tag>"
    echo "Example: $0 928990672971 us-east-1 v20251208--amd64"
    exit 1
fi

ECR_REPO="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com/${IMAGE_NAME}"
ECR_URI="${ECR_REPO}:${IMAGE_TAG}"

echo "Pushing to ECR: ${ECR_URI}"

# Find AWS profile for the account ID
# First try to find profiles with sso_account_id matching
PROFILE=$(grep -B 2 "sso_account_id = $ACCOUNT_ID" ~/.aws/config 2>/dev/null | grep "^\[profile" | head -1 | sed 's/\[profile \(.*\)\]/\1/')

if [ -z "$PROFILE" ]; then
    # Fallback: look for Administrator role profiles, or ReadOnly for specific account
    if [ "$ACCOUNT_ID" = "484922885923" ]; then
        PROFILE=$(grep -B 5 "sso_account_id = $ACCOUNT_ID" ~/.aws/config 2>/dev/null | grep "ReadOnly" | head -1 | sed 's/.*\[profile \(.*\)\].*/\1/')
    else
        PROFILE=$(grep -B 5 "sso_account_id = $ACCOUNT_ID" ~/.aws/config 2>/dev/null | grep "Administrator" | head -1 | sed 's/.*\[profile \(.*\)\].*/\1/')
    fi
fi

if [ -z "$PROFILE" ]; then
    echo "Warning: Could not find AWS profile for account $ACCOUNT_ID"
    echo "Available profiles for this account:"
    grep -B 2 "sso_account_id = $ACCOUNT_ID\|aws_account_id.*$ACCOUNT_ID" ~/.aws/config 2>/dev/null | grep "^\[profile" || echo "  None found"
    echo ""
    echo "Using default profile"
    PROFILE="default"
fi

echo "Using AWS profile: $PROFILE"

# Logout from previous ECR registries to avoid credential conflicts
echo "Logging out from previous ECR sessions..."
docker logout "$ECR_REPO" 2>/dev/null || true

# Login to ECR
echo "Logging in to ECR ($ECR_REPO)..."
aws ecr get-login-password --region "$REGION" --profile "$PROFILE" | docker login --username AWS --password-stdin "$ECR_REPO"

if [ $? -ne 0 ]; then
    echo "Error: Failed to login to ECR"
    exit 1
fi

# Tag and push the image
LOCAL_IMAGE="descheduler:${IMAGE_TAG}"
echo "Tagging image: $LOCAL_IMAGE -> $ECR_URI"
docker tag "$LOCAL_IMAGE" "$ECR_URI"

if [ $? -ne 0 ]; then
    echo "Error: Failed to tag image"
    exit 1
fi

echo "Pushing image to ECR..."
docker push "$ECR_URI"

if [ $? -ne 0 ]; then
    echo "Error: Failed to push image"
    exit 1
fi

echo "Successfully pushed $ECR_URI"
