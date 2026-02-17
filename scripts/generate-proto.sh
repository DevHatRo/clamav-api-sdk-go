#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Generating protobuf code..."

protoc \
  --proto_path="$PROJECT_ROOT/grpc/proto" \
  --go_out="$PROJECT_ROOT/grpc/proto" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$PROJECT_ROOT/grpc/proto" \
  --go-grpc_opt=paths=source_relative \
  "$PROJECT_ROOT/grpc/proto/clamav.proto"

echo "Done."
