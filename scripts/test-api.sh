#!/bin/bash
# Test script for HyperFleet API
# Usage: ./scripts/test-api.sh [API_URL]

set -e

API_URL="${1:-http://localhost:8000}"
API_BASE="$API_URL/api/hyperfleet/v1"

echo "=== HyperFleet API Test Script ==="
echo "API URL: $API_BASE"
echo ""

# Generate unique suffix for resource names
SUFFIX=$(date +%s | tail -c 6)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

success() { echo -e "${GREEN}✓ $1${NC}"; }
error() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo "jq is required but not installed. Install with: sudo dnf install jq"
    exit 1
fi

# 1. Create a Cluster
info "Creating cluster: test-cluster-$SUFFIX"
CLUSTER_RESPONSE=$(curl -s -X POST "$API_BASE/clusters" \
    -H "Content-Type: application/json" \
    -d "{
        \"apiVersion\": \"hyperfleet.io/v1\",
        \"kind\": \"Cluster\",
        \"name\": \"test-cluster-$SUFFIX\",
        \"spec\": {
            \"region\": \"us-east-1\",
            \"version\": \"4.14\",
            \"provider\": \"aws\"
        }
    }")

echo "$CLUSTER_RESPONSE" | jq .

CLUSTER_ID=$(echo "$CLUSTER_RESPONSE" | jq -r '.metadata.id // .id // empty')
if [ -z "$CLUSTER_ID" ]; then
    error "Failed to create cluster or extract ID"
fi
success "Cluster created with ID: $CLUSTER_ID"
echo ""

# 2. Get the Cluster
info "Fetching cluster..."
curl -s "$API_BASE/clusters/$CLUSTER_ID" | jq .
success "Cluster fetched"
echo ""

# 3. Create a NodePool
info "Creating nodepool: worker-pool-$SUFFIX"
NODEPOOL_RESPONSE=$(curl -s -X POST "$API_BASE/clusters/$CLUSTER_ID/nodepools" \
    -H "Content-Type: application/json" \
    -d "{
        \"apiVersion\": \"hyperfleet.io/v1\",
        \"kind\": \"NodePool\",
        \"name\": \"worker-pool-$SUFFIX\",
        \"spec\": {
            \"replicas\": 3,
            \"instanceType\": \"m5.xlarge\",
            \"autoScaling\": {
                \"enabled\": true,
                \"minReplicas\": 1,
                \"maxReplicas\": 10
            }
        }
    }")

echo "$NODEPOOL_RESPONSE" | jq .

NODEPOOL_ID=$(echo "$NODEPOOL_RESPONSE" | jq -r '.metadata.id // .id // empty')
if [ -z "$NODEPOOL_ID" ]; then
    error "Failed to create nodepool or extract ID"
fi
success "NodePool created with ID: $NODEPOOL_ID"
echo ""

# 4. Create an IDP
info "Creating IDP: corporate-sso-$SUFFIX"
IDP_RESPONSE=$(curl -s -X POST "$API_BASE/clusters/$CLUSTER_ID/idps" \
    -H "Content-Type: application/json" \
    -d "{
        \"apiVersion\": \"hyperfleet.io/v1\",
        \"kind\": \"IDP\",
        \"name\": \"corporate-sso-$SUFFIX\",
        \"spec\": {
            \"type\": \"OIDC\",
            \"issuerURL\": \"https://sso.example.com\",
            \"clientID\": \"hyperfleet-client\",
            \"clientSecret\": \"secret-placeholder\"
        }
    }")

echo "$IDP_RESPONSE" | jq .

IDP_ID=$(echo "$IDP_RESPONSE" | jq -r '.metadata.id // .id // empty')
if [ -z "$IDP_ID" ]; then
    error "Failed to create IDP or extract ID"
fi
success "IDP created with ID: $IDP_ID"
echo ""

# 5. List all resources
info "Listing all clusters..."
curl -s "$API_BASE/clusters" | jq .
echo ""

info "Listing nodepools for cluster $CLUSTER_ID..."
curl -s "$API_BASE/clusters/$CLUSTER_ID/nodepools" | jq .
echo ""

info "Listing IDPs for cluster $CLUSTER_ID..."
curl -s "$API_BASE/clusters/$CLUSTER_ID/idps" | jq .
echo ""

# 6. Summary
echo "=== Test Summary ==="
success "Cluster ID:  $CLUSTER_ID"
success "NodePool ID: $NODEPOOL_ID"
success "IDP ID:      $IDP_ID"
echo ""

# 7. Cleanup prompt
echo "To clean up test resources, run:"
echo "  curl -X DELETE $API_BASE/clusters/$CLUSTER_ID/idps/$IDP_ID"
echo "  curl -X DELETE $API_BASE/clusters/$CLUSTER_ID/nodepools/$NODEPOOL_ID"
echo "  curl -X DELETE $API_BASE/clusters/$CLUSTER_ID"
