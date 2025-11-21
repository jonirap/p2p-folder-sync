#!/bin/bash
# Install pre-commit hook for P2P Folder Sync

set -e

HOOK_SOURCE=".pre-commit-hook.sh"
HOOK_DEST=".git/hooks/pre-commit"

# Check if we're in a git repository
if [ ! -d ".git" ]; then
    echo "Error: Not in a git repository"
    exit 1
fi

# Check if hook source exists
if [ ! -f "$HOOK_SOURCE" ]; then
    echo "Error: $HOOK_SOURCE not found"
    exit 1
fi

# Backup existing hook if present
if [ -f "$HOOK_DEST" ]; then
    echo "Backing up existing pre-commit hook to $HOOK_DEST.backup"
    cp "$HOOK_DEST" "$HOOK_DEST.backup"
fi

# Copy and make executable
cp "$HOOK_SOURCE" "$HOOK_DEST"
chmod +x "$HOOK_DEST"

echo "✓ Pre-commit hook installed successfully"
echo ""
echo "The hook will run on every commit to:"
echo "  - Check code formatting"
echo "  - Run linter"
echo "  - Run unit tests"
echo "  - Check for common issues"
echo "  - Verify build"
echo ""
echo "To skip the hook for a specific commit, use: git commit --no-verify"
