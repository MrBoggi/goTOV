---
name: api-security
description: Implement secure API design patterns including authentication, authorization (Supabase RLS), input validation, and protection against common vulnerabilities. Use when designing New API routes, implementing Server Actions, or reviewing security policies.
---

# API Security Best Practices (Sourmate)

Ensure all API interactions (REST, Server Actions, Edge Functions) follow strict security patterns.

## Core Constraint
> [!IMPORTANT]
> **Project Constraint**: Sourmate uses `src/proxy.ts` instead of `middleware.ts`. When reviewing or building routing logic, ensure it aligns with the proxy implementation.

---

## 1. Authentication & Authorization (Supabase)

### Row Level Security (RLS)
Every table MUST have RLS enabled. NEVER allow operations without a specific policy.

```sql
-- Example: Allow users to view only their own bakes
CREATE POLICY "Users can view own bakes" ON bakes
FOR SELECT USING (auth.uid() = user_id);
```

### Server Actions
Always verify authentication at the start of every Server Action.

```typescript
// src/lib/actions.ts
import { createClient } from '@/utils/supabase/server';

export async function myAction(data: any) {
  const supabase = createClient();
  const { data: { user } } = await supabase.auth.getUser();
  
  if (!user) {
    throw new Error('Unauthorized');
  }
  
  // Proceed with authorized operation
}
```

---

## 2. Input Validation (Zod)

Use Zod for all incoming data to prevent injection and unexpected payloads.

```typescript
import { z } from 'zod';

const Schema = z.object({
  id: z.string().uuid(),
  count: z.number().min(1).max(100),
});

export async function myAction(input: unknown) {
  const validated = Schema.parse(input);
  // ...
}
```

---

## 3. Data Protection

### PII and Metadata
- Avoid returning sensitive user fields (email, phone) to the frontend unless necessary.
- Use `select()` in Supabase queries to limit returned columns.

### Error Messages
- Do NOT return internal database errors to the frontend.
- Use generic messages for authentication failures ("Invalid credentials") to prevent user enumeration.

---

## 4. Security Checklist

- [ ] RLS enabled on all new tables?
- [ ] `auth.getUser()` used in Server Actions?
- [ ] Input validated with Zod?
- [ ] Sensitive data filtered out of responses?
- [ ] Rate limiting considered for public endpoints?

## Related Files
- [bakesteps.md](file:///home/draftlogic/Documents/Repos/sourmate/docs/bakesteps.md) - Core logic validation.
- [src/proxy.ts](file:///home/draftlogic/Documents/Repos/sourmate/src/proxy.ts) - Routing security.
