# Authentication

This document describes authentication mechanisms for the HyperFleet API.

## Overview

HyperFleet API supports two authentication modes:

1. **Development Mode (No Auth)**: For local development and testing
2. **Production Mode (JWT Auth)**: JWT-based authentication with configurable issuer

## Development Mode (No Auth)

For local development and testing, authentication can be disabled.

### Usage

```bash
# Start service without authentication
make run-no-auth

# Access API without tokens
curl http://localhost:8000/api/hyperfleet/v1/clusters | jq
```

### Configuration

```bash
export HYPERFLEET_SERVER_JWT_ENABLED=false
./bin/hyperfleet-api serve
```

**Important**: Never disable authentication in production environments.

## Production Mode (JWT Auth)

Production deployments use JWT-based authentication with a configurable issuer.

### Usage

```bash
# Start service with authentication
make run

# Access API with a valid JWT
curl -H "Authorization: Bearer ${TOKEN}" \
  http://localhost:8000/api/hyperfleet/v1/clusters
```

### JWT Authentication

HyperFleet API validates JWT tokens using RS256 signature verification.

**Token validation checks:**
1. Signature - Token signed by trusted issuer
2. Issuer - Matches configured `HYPERFLEET_SERVER_JWT_ISSUER_URL`
3. Audience - Matches configured `HYPERFLEET_SERVER_JWT_AUDIENCE`
4. Expiration - Token not expired
5. Claims - Required claims present

**Token format:**
```text
Authorization: Bearer <jwt-token>
```

Example request:
```bash
curl -H "Authorization: Bearer ${TOKEN}" \
  http://localhost:8000/api/hyperfleet/v1/clusters
```

## Authorization

HyperFleet API implements resource-based authorization.

### Resource Ownership

Resources track ownership via `created_by` and `updated_by` fields:

```json
{
  "id": "cluster-123",
  "name": "my-cluster",
  "created_by": "user@example.com",
  "updated_by": "user@example.com"
}
```

### Access Control

- **Create**: Users can create resources
- **Read**: Users can read resources they created or have access to
- **Update**: Users can update resources they own
- **Delete**: Users can delete resources they own

Users within the same organization can access shared resources based on organizational membership.

## Configuration

### Environment Variables

```bash
# Development (no auth)
export HYPERFLEET_SERVER_JWT_ENABLED=false

# Production (with auth)
export HYPERFLEET_SERVER_JWT_ENABLED=true
export HYPERFLEET_SERVER_JWT_ISSUER_URL=https://sso.redhat.com/auth/realms/redhat-external
export HYPERFLEET_SERVER_JWT_AUDIENCE=https://api.openshift.com
```

See [Deployment](deployment.md) for complete configuration options.

### Kubernetes Deployment

Configure via Helm values:

```yaml
# values.yaml
auth:
  enabled: true
  jwtIssuer: https://sso.redhat.com/auth/realms/redhat-external
  jwtAudience: https://api.openshift.com
```

Deploy:
```bash
helm install hyperfleet-api ./charts/ --values values.yaml
```

## Troubleshooting

### Common Issues

**401 Unauthorized**
- Check token is valid and not expired
- Verify `HYPERFLEET_SERVER_JWT_ISSUER_URL` and `HYPERFLEET_SERVER_JWT_AUDIENCE` match token claims
- Ensure `Authorization` header is correctly formatted

**403 Forbidden**
- User authenticated but lacks permissions
- Check resource ownership
- Verify organizational membership

**Token debugging**
```bash
# Decode JWT token (header and payload only, not verified)
echo $TOKEN | cut -d. -f2 | base64 -d | jq

# Check token expiration
echo $TOKEN | cut -d. -f2 | base64 -d | jq '.exp | todate'
```

## Related Documentation

- [Deployment](deployment.md) - Authentication configuration and Kubernetes setup
