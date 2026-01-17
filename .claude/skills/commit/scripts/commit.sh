#!/usr/bin/env bash
# Stage all changes and show status for review before committing

set -euo pipefail

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

# Get untracked files
UNTRACKED=$(git ls-files --others --exclude-standard)

# Check untracked files for gitignore candidates BEFORE staging
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

# No gitignore candidates - stage all changes
git add -A
echo "Staged changes:"
git status --short
