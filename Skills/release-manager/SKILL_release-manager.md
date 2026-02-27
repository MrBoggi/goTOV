---
name: release-manager
description: Automates the Sourmate release process. Use this when the user says "make a release", "bump version", or "prepare for deployment". This skill orchestrates version bumping (yyyy.mm.number), documentation updates via release-scribe, archiving of project history, and git tagging.
---

# Release Manager

This skill provides a robust workflow for releasing new versions of Sourmate. It supports two release modes: **Development Release** (internal sync) and **Production Release** (deployment).

## Release Modes

### 1. Development Release (Standard)
Use this for regular version bumps and updating release notes within the `development` branch.
- **Start Branch**: `development`
- **Target Branch**: `development`
- **Workflow**: `development` -> `release/[version]` -> Pull Request -> `development`

### 2. Production Release (Deployment)
Use this only when explicitly requested to "release to production".
- **Start Branch**: `development`
- **Target Branch**: `main`
- **Workflow**: `development` -> `release/[version]` -> Pull Request -> `main` -> Back-merge to `development`

## Core Rules
- **Version Format**: `yyyy.mm.number` (e.g., `2026.01.2`). No "v" prefix.
- **Git Tags**: Create a tag `[version]` only for **Production Releases** after merging to `main`.
- **Architecture**: `middleware.ts` is deprecated. Do not touch it. Use `proxy.ts`.

## Documentation Sync

> [!IMPORTANT]
> **Always keep documentation in sync.**
> If any logic or bake steps are modified in the code, the following documents MUST be updated to reflect the change:
> - [bakesteps.md](file:///home/draftlogic/Documents/Repos/sourmate/docs/bakesteps.md)
> - [bakeformulas.md](file:///home/draftlogic/Documents/Repos/sourmate/docs/bakeformulas.md)
> These two documents are the source of truth for the application's core logic.
>
> **FAQ Updates**
> Whenever adding or modifying functionality that affects the user journey (UI, logic, features), you MUST update the FAQ section:
> - [en.json](file:///home/draftlogic/Documents/Repos/sourmate/messages/en.json)
> - [nb.json](file:///home/draftlogic/Documents/Repos/sourmate/messages/nb.json)
> Ensure both languages are updated to maintain feature parity in documentation.

## Workflow

### 1. Preparation & Branching
1. **Analyze State**: Ensure the current branch is `development` and is clean (`git status`).
2. **Calculate Version**: Check `package.json` for the current version. Increment the `number` part of the `yyyy.mm.number` pattern. Do NOT rely on `git describe` as it only reflects production tags.
3. **Create Branch**: `git checkout -b release/[version]`.

### 2. Systematic Updates
Update the version string in these files (use `2026.01.2` format):
- `package.json` (`version` and `build` script fallback)
- `src/components/footer.tsx` (version fallback)
- `src/components/modals/ReleaseNotesModal.tsx` (version fallback). **Note**: The modal is configured to only show the 5 most recent releases to maintain a clean UI.

### 3. Release Notes & Documentation
1. **Generate Notes**: Call **@[skills/release-scribe/SKILL_release-scribe.md]** to analyze commits and update `docs/release-notes.md`.
2. **Archive History (CRITICAL)**: Move current implementation plans and walkthroughs to `docs/`:
   - `[date]-plan_[feature].md`
   - `[date]-walkthrough_[feature].md`

### 4. Push & PR
1. **Commit**: `chore(release): prepare [version] 🚀`.
2. **Push**: `git push origin release/[version]`.
3. **Notify**: Inform the user to open a PR:
   - For **Development**: PR against `development`.
   - For **Production**: PR against `main`.

### 5. Finalization (Production Only)
If it was a Production Release:
1. **Tag**: Create a tag `[version]` on `main`.
2. **Back-merge**: Merge `main` back into `development` to sync versioning and history.

## Automation Script
Use the bundled script for initialization:
- `skills/release-manager/scripts/prepare_release.sh`

## Example Execution
1. User: "Release current changes to dev"
2. Agent: Creates `release/2026.01.3`, updates files, pushes, and asks for PR to `development`.
3. User: "Launch 2026.02.0 to production"
4. Agent: Creates `release/2026.02.0`, updates files, pushes, and asks for PR to `main`.