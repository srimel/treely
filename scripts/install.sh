#!/usr/bin/env bash
set -e

BINARY=treely
INSTALL_DIR=/usr/local/bin

echo "Building $BINARY..."
go build -o "$BINARY" ./cmd/treely

echo "Installing to $INSTALL_DIR/$BINARY..."
mv "$BINARY" "$INSTALL_DIR/$BINARY"

echo "Done. Run 'treely' to get started."
