---
name: backend-development
description: Definitive guide for backend tasks in Sourmate. Use this skill when the user asks for database changes, API logic, Server Actions, or anything related to Supabase and data persistence. This skill enforces strict architectural patterns and "snake_case" DB naming conventions.
---

# Backend Development

This skill governs all backend engineering in the Sourmate project. It strictly enforces the architecture defined in `AGENTS.md` and provides specialized workflows for Supabase and Next.js Server Actions.

## Domain Boundaries

> [!IMPORTANT]
> **DO NOT** use this skill for:
> - **Releasing/Versioning**: Use `release-manager`.
> - **Frontend UI/Components**: Use general knowledge or `AGENTS.md` guidelines.
> - **Documentation/Release Notes**: Use `release-scribe` or `release-manager`.

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

## Tech Stack & Architecture

- **Framework**: Next.js 16 (App Router)
- **Language**: TypeScript (`.ts`, `.tsx`)
- **Database**: Supabase (PostgreSQL)
- **Data Access**: 
    - **Server Actions**: Primary method for mutations (POST/PUT/DELETE).
    - **Supabase SSR Client**: Used inside Server Actions and Server Components.
- **Styling**: None (Backend logic only).

## Core Rules

1.  **Strict Typing**: All database inputs/outputs must be typed. Use generated types from `database.types.ts` if available, or define interfaces.
2.  **Naming Conventions**:
    - **Database Tables/Columns**: `snake_case` (e.g., `last_fed_at`, `storage_preference`).
    - **TypeScript Variables**: `camelCase` (e.g., `lastFedAt`, `storagePreference`).
3.  **Security**:
    - **Row Level Security (RLS)**: MUST be enabled on all new tables.
    - **Validation**: Validate all inputs in Server Actions using `zod` or explicit checks before calling Supabase.

## Architecture Constraints

> [!WARNING]
> **DEPRECATED: middleware.ts**
> `middleware.ts` is strictly forbidden. All edge logic, including authentication, i18n, and routing, MUST be implemented in `src/proxy.ts`.
> - **NEVER** create or modify `middleware.ts`.
> - **ALWAYS** use `src/proxy.ts` for request interception and rewriting.


## Workflows

### 1. Creating/Modifying Database Schema

**Trigger**: "Add a new table", "Add column to starters", "Change database schema".

1.  **Plan**: Check the latest snapshot in `database-schema/`.
2.  **Update Schema**:
    - Use the Supabase MCP Server to execute changes or `apply_migration`.
3.  **Snapshot**:
    - **ALWAYS** generate a new dated snapshot folder in `database-schema/YYYY-MM-DD/`.
    - Include the full `complete-schema.sql`, `metadata.json`, and `ERD.md`.
4.  **Verify**: Ensure the new snapshot reflects all changes and can be used for a full restore.

### 2. Implementing Server Actions

**Trigger**: "Create an action to feed starter", "Backend logic for...", "API endpoint for...".

1.  **Location**: Place actions in `actions.ts` files collocated with the feature (e.g., `src/app/[locale]/dashboard/actions.ts`) or in `src/lib/actions/` if shared.
2.  **Pattern**:
    ```typescript
    'use server'

    import { createClient } from '@/lib/supabase'
    import { revalidatePath } from 'next/cache'

    export async function myAction(formData: FormData) {
      const supabase = await createClient()

      // 1. Auth Check
      const { data: { user }, error: authError } = await supabase.auth.getUser()
      if (authError || !user) throw new Error('Unauthorized')

      // 2. Extract & Validate Data
      const myValue = formData.get('my_value') as string

      // 3. Database Mutation
      const { error } = await supabase
        .from('my_table')
        .insert({ user_id: user.id, value: myValue })

      if (error) throw error

      // 4. Revalidate
      revalidatePath('/dashboard')
    }
    ```
3.  **Error Handling**: Throw errors for the UI to catch, or return `{ error: string }` if using a form state hook.

### 3. Migrating/Refactoring Backend Code

**Trigger**: "Refactor the starter feed logic", "Move logic to server action".

1.  Identify the `use client` component currently handling the logic.
2.  Extract logic to a `use server` function.
3.  Ensure no client-side specific code (like `window`, `local storage`) remains in the server action.
4.  Update the client component to call the Server Action.

## Common Snippets

### Supabase Client (Server)
```typescript
import { createClient } from '@/lib/supabase'
const supabase = await createClient()
```

### RLS Policy Example
```sql
alter table "public"."starters" enable row level security;

create policy "Users can only see their own starters"
on "public"."starters"
as permissive
for select
to authenticated
using (auth.uid() = user_id);
```

## Helper Scripts

- `scripts/create_migration.ps1`: Generates a correctly formatted migration file.
