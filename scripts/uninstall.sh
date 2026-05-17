#!/usr/bin/env bash
set -e

BINARY=treely
INSTALL_DIR=/usr/local/bin
TARGET="$INSTALL_DIR/$BINARY"

if [ ! -f "$TARGET" ]; then
  echo "$TARGET not found, nothing to uninstall."
  exit 0
fi

echo "Removing $TARGET..."
rm "$TARGET"

CONFIG_DIR="$HOME/.treely"
if [ -d "$CONFIG_DIR" ]; then
  echo "Removing $CONFIG_DIR..."
  rm -rf "$CONFIG_DIR"
fi

echo "Done."
