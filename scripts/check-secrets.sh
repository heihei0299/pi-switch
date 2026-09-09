#!/usr/bin/env bash
set -euo pipefail

# Scan tracked source and configuration without treating generated artifacts as
# evidence. This intentionally checks high-signal credential formats only.
patterns=(
  'sk-[A-Za-z0-9]{20,}'
  'AKIA[0-9A-Z]{16}'
  '-----BEGIN (RSA|OPENSSH|EC|PRIVATE) KEY-----'
  'gh[pousr]_[A-Za-z0-9_]{20,}'
)
for pattern in "${patterns[@]}"; do
  if git grep -nE -e "$pattern" -- . \
      ':(exclude)package-lock.json' \
      ':(exclude)pnpm-lock.yaml' \
      ':(exclude)scripts/check-secrets.sh'; then
    echo "secret-like material matched: $pattern" >&2
    exit 1
  fi
done
