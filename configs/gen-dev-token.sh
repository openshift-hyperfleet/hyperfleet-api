#!/usr/bin/env bash
# Generates an RSA key pair, writes a JWKS file for the API, and outputs a signed JWT.
# Usage: ./configs/gen-dev-token.sh
# The token is valid for 8 hours and pairs with configs/dev.yaml.
#
# If a private key already exists at /tmp/hf-dev-key.pem, reuses it
# (pass --new-key to force regeneration).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KID="dev-key"
JWKS_PATH="${SCRIPT_DIR}/dev-jwks.json"
KEY_PATH="${XDG_RUNTIME_DIR:-/tmp}/hf-dev-key.pem"

# base64url encode stdin (no padding, URL-safe alphabet)
b64url() {
  base64 | tr '+/' '-_' | tr -d '=\n'
}

# Refuse symlinks at KEY_PATH to prevent symlink attacks
if [[ -L "$KEY_PATH" ]]; then
  echo "ERROR: $KEY_PATH is a symlink, refusing to use it" >&2
  exit 1
fi

# Reuse existing key unless --new-key is passed or no key exists
if [[ "${1:-}" == "--new-key" ]] || [[ ! -f "$KEY_PATH" ]] || [[ ! -f "$JWKS_PATH" ]]; then
  PRIV_KEY=$(openssl genrsa 2048 2>/dev/null)
  install -m 600 /dev/null "$KEY_PATH"
  echo "$PRIV_KEY" > "$KEY_PATH"
  REGEN_JWKS=true
else
  PRIV_KEY=$(cat "$KEY_PATH")
  REGEN_JWKS=false
fi

# Only regenerate JWKS when key changed
if [[ "$REGEN_JWKS" == "true" ]]; then
  N=$(echo "$PRIV_KEY" | openssl rsa -modulus -noout 2>/dev/null | cut -d= -f2 | xxd -r -p | b64url)
  E="AQAB"

  cat > "$JWKS_PATH" << EOF
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "${KID}",
      "alg": "RS256",
      "use": "sig",
      "n": "${N}",
      "e": "${E}"
    }
  ]
}
EOF
fi

# Build JWT claims
NOW=$(date +%s)
EXP=$((NOW + 28800))

HEADER=$(echo -n '{"alg":"RS256","kid":"'"${KID}"'","typ":"JWT"}' | b64url)
PAYLOAD=$(echo -n '{
  "iss":"https://test-issuer.example.com",
  "sub":"smoke-test-user",
  "email":"smoke@example.com",
  "iat":'"${NOW}"',
  "exp":'"${EXP}"'
}' | b64url)

# Sign header.payload with RSA-SHA256
SIGNATURE=$(echo -n "${HEADER}.${PAYLOAD}" \
  | openssl dgst -sha256 -sign <(echo "$PRIV_KEY") \
  | b64url)

echo "${HEADER}.${PAYLOAD}.${SIGNATURE}"
