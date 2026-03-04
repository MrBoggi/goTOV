---
name: analytical-planning
description: Use when a user asks for a plan for a coding task. Enforces rigorous technical audit, skill selection, and architectural evaluation.
---

# Analytical Planning

## Goal

Transform a user request into a technically sound, rigorously analyzed implementation plan. Do not simply record the request; evaluate its impact, identify potential side effects, and select the most appropriate specialized skills.

## Workflow

### 1. Technical Audit & Skills Discovery
- Read the relevant codebase. Do not make assumptions.
- **Search the `Skills/` directory**. Evaluate every folder to see if its specific constraints or best practices apply to the task (e.g., `golang-pro` for Go logic, `postgres-best-practices` for DB changes).
- Identify architectural constraints (e.g., "Is this a blocking operation in an async loop?", "Will this cause a race condition?").

### 2. Critical Reasoning
- Don't just follow the user's instructions literally. If the proposed solution has a flaw (e.g., a `time.Sleep` in a main loop), point it out and propose a better pattern (e.g., state machines, async goroutines).
- Ask clarifying questions only if a technical decision cannot be made without them.

### 3. Generate Plan
Use the following structure:

- **Technical Analysis**: 2-4 sentences explaining the *why* and the *risks*. Address potential pitfalls like concurrency, performance, or hardware safety.
- **Task Assignments**: Explicitly map roles to specialized `Skills/` files detected in step 1.
- **Scope**: Define clear boundaries.
- **Action Items**: Atomic, ordered tasks. Use [Coder] and [Reviewer] prefixes to show who does what.
- **Validation**: Specific steps to prove the logic works (logs, tests, race detector).

## Plan Template

```markdown
# Plan: [Name]

## Technical Analysis
<Detailed reasoning about the implementation, risks identified, and architectural choices made.>

## Task Assignments

| Phase | Skill | Model | Skill File | Responsibility |
| :--- | :--- | :--- | :--- | :--- |
| **Planning** | Architect | `gemini-3.1-pro-high` | `Skills/planner/SKILL_planner.md` | Analysis and design |
| **Implementation** | Coder | `gemini-3-flash` | <Path to specific skill(s)> | Code execution |
| **Validation** | Reviewer | `claude-4.6-sonnet-thinking` | <Path to specific skill(s)> | Verification and review |

## Scope
- In:
- Out:

## Action Items
[ ] [Coder] <Step 1>
[ ] [Coder] <Step 2>
[ ] [Reviewer] <Validation Step>
```