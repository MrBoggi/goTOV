#!/bin/bash
# Sourmate Release Preparation Script
# Usage: ./prepare_release.sh <new_version>

NEW_VERSION=$1

if [ -z "$NEW_VERSION" ]; then
    echo "Error: Missing version number. Usage: ./prepare_release.sh yyyy.mm.number"
    exit 1
fi

echo "🚀 Preparing release $NEW_VERSION..."

# 1. Ensure we are on development
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "development" ]; then
    echo "Warning: You are not on the 'development' branch. Standard workflow requires starting from 'development'."
    # Optionally: git checkout development
fi

# 2. Create release branch
git checkout -b "release/$NEW_VERSION"

# 2. Update package.json
# Using sed for portability (macOS/Linux compatible)
sed -i "s/\"version\": \".*\"/\"version\": \"$NEW_VERSION\"/" package.json
sed -i "s/echo '.*') next build/echo '$NEW_VERSION') next build/" package.json

# 3. Update footer.tsx
sed -i "s/NEXT_PUBLIC_APP_VERSION || '.*'/NEXT_PUBLIC_APP_VERSION || '$NEW_VERSION'/" src/components/footer.tsx

# 4. Update ReleaseNotesModal.tsx
sed -i "s/NEXT_PUBLIC_APP_VERSION || \".*\"/NEXT_PUBLIC_APP_VERSION || \"$NEW_VERSION\"/" src/components/modals/ReleaseNotesModal.tsx

echo "✅ Files updated to $NEW_VERSION."
echo "👉 Now run: release-scribe skill to update release notes."
echo "👉 Then: Archive implementation plans/walkthroughs to docs/."
echo "👉 Finally: Commit and push the branch."
