#!/usr/bin/env bash
set -e

echo "🔧 Installing Go Git hooks..."

HOOK_DIR=".git/hooks"

mkdir -p "$HOOK_DIR"

install_hook() {
    local name=$1
    local file=$2

    echo "➡️  Installing $name..."
    cp "$file" "$HOOK_DIR/$name"
    chmod +x "$HOOK_DIR/$name"
}

install_hook "pre-commit" "scripts/hooks/pre-commit"
install_hook "pre-push"   "scripts/hooks/pre-push"
install_hook "pre-merge"  "scripts/hooks/pre-merge"

echo "✅ Hooks installed."
