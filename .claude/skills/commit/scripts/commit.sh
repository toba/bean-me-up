#!/usr/bin/env bash
# Unified commit workflow script
# Usage: commit.sh "subject" "description" [NEW_VERSION]
# Example: commit.sh "add user auth" "implements JWT-based auth flow" v1.2.0

set -euo pipefail

# === Arguments ===
SUBJECT="${1:-}"
DESCRIPTION="${2:-}"
NEW_VERSION="${3:-}"

if [ -z "$SUBJECT" ]; then
    echo "Usage: commit.sh \"subject\" \"description\" [NEW_VERSION]"
    echo "  subject     - Commit subject line (required)"
    echo "  description - Commit description (optional, use empty string if skipping)"
    echo "  NEW_VERSION - Version tag to create, e.g., v1.2.0 (optional)"
    exit 1
fi

# Patterns that suggest a file should be in .gitignore
GITIGNORE_PATTERNS=(
    '\.log$'
    '\.tmp$'
    '\.cache$'
    '\.exe$'
    '\.test$'
    '\.out$'
    '\.DS_Store$'
    '\.swp$'
    '\.swo$'
    '\.idea/'
    '\.vscode/'
    'vendor/'
    'coverage/'
    '\.env$'
    '\.env\.local$'
    'credentials\.'
    'secrets\.'
    '\.key$'
    '\.pem$'
)

# === Pre-commit checks ===
echo "=== Pre-commit checks ==="

echo "Running golangci-lint..."
if ! golangci-lint run; then
    echo "Lint failed"
    exit 1
fi

echo "Running tests..."
if ! go test ./...; then
    echo "Tests failed"
    exit 1
fi

# === Check .gitignore candidates ===
echo ""
echo "=== Checking for gitignore candidates ==="

UNTRACKED=$(git ls-files --others --exclude-standard)

if [ -n "$UNTRACKED" ]; then
    CANDIDATES=()
    while IFS= read -r file; do
        for pattern in "${GITIGNORE_PATTERNS[@]}"; do
            if echo "$file" | grep -qE "$pattern"; then
                CANDIDATES+=("$file")
                break
            fi
        done
    done <<< "$UNTRACKED"

    if [ ${#CANDIDATES[@]} -gt 0 ]; then
        echo "GITIGNORE_CANDIDATES:"
        printf '%s\n' "${CANDIDATES[@]}"
        echo ""
        echo "These untracked files may belong in .gitignore."
        exit 2
    fi
fi
echo "No gitignore candidates found."

# === Stage and show ===
echo ""
echo "=== Staging changes ==="
git add -A
git status --short

echo ""
echo "=== Staged diff ==="
git diff --staged --stat

# === Commit ===
echo ""
echo "=== Creating commit ==="

# Build commit message
if [ -n "$DESCRIPTION" ]; then
    COMMIT_MSG="$SUBJECT

$DESCRIPTION

Co-Authored-By: Claude <noreply@anthropic.com>"
else
    COMMIT_MSG="$SUBJECT

Co-Authored-By: Claude <noreply@anthropic.com>"
fi

git commit -m "$COMMIT_MSG"

# === Sync beans ===
echo ""
echo "=== Syncing beans ==="
if command -v beanup &> /dev/null && [ -f .beans.clickup.yml ]; then
    beanup sync || echo "Warning: beanup sync failed (continuing anyway)"
else
    echo "Skipping beanup sync (not configured)"
fi

# === Push commit ===
echo ""
echo "=== Pushing to remote ==="
git push

# === Version release ===
if [ -n "$NEW_VERSION" ]; then
    echo ""
    echo "=== Creating release $NEW_VERSION ==="
    git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"
    git push --tags
    gh release create "$NEW_VERSION" --title "$NEW_VERSION" --generate-notes
    echo "Release $NEW_VERSION created"
fi

echo ""
echo "=== Done ==="
