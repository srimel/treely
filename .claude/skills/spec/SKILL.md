---
name: spec
description: Interactively reduces ambiguity for a feature or bug, then writes an implementation plan to docs/plans/features/ or docs/plans/bugs/. Trigger with /spec followed by a brief description.
---

# Feature & Bug Planning Skill

When this skill is invoked, guide the user through a structured planning conversation that eliminates ambiguity before producing a written implementation plan.

## Step 1 — Classify and Gather Context

First, determine the **type** (feature or bug) and ask targeted clarifying questions. Do not generate a plan yet.

### For a **feature** request, ask:

1. **Goal**: What user problem does this solve? What does success look like?
2. **Scope**: What is explicitly out of scope?
3. **Entry points**: Which files/packages are likely touched?
4. **Constraints**: Any performance, API, or platform constraints to respect?
5. **Edge cases**: What failure modes or edge cases exist?

### For a **bug** report, ask:

1. **Reproduction**: What are the exact steps to reproduce? Is it consistent or flaky?
2. **Expected vs actual**: What should happen, and what happens instead?
3. **Scope**: Which component is responsible — daemon, TUI, IPC, or config?
4. **Impact**: Is there data loss, process leakage, or just a UX issue?
5. **Known constraints**: Any architectural constraints (e.g., no shared types package, platform-specific files) that the fix must respect?

Skip questions the user already answered in their initial description. Only ask what is genuinely unclear.

## Step 2 — Confirm Understanding

After the user answers, briefly summarize your understanding in 3–5 bullet points and ask: **"Does this capture it, or anything to correct?"**

Do not write the plan file until the user confirms.

## Step 3 — Write the Plan File

Once confirmed, produce the plan and write it to:

- `docs/plans/features/<kebab-case-title>.md` for features
- `docs/plans/bugs/<kebab-case-title>.md` for bugs

### Plan file format

Use this structure (adapt sections as needed — not all are required for every plan):

```markdown
# <Title>

## Context

<2–4 sentences. Why does this work need to happen? What is the current state?>

## Problem / Goal

<For bugs: what breaks and why. For features: what the user can do after this ships.>

## Constraints

<Architectural rules from CLAUDE.md or discovered during planning that this work must respect.>

## Approach

<How to solve it. Reference specific files and line numbers where known. Call out alternatives considered and why they were rejected.>

## Implementation Steps

### Step 1 — <Name>

<What to do and why. File paths and line numbers.>

### Step 2 — <Name>

...

## Testing

<How to verify correctness. Unit tests, integration tests, manual steps.>

## Out of Scope

<What this plan deliberately does not address.>
```

Tailor depth to complexity. A one-file bug fix doesn't need every section; a multi-package feature does. Always include **Context**, **Approach**, **Implementation Steps**, and **Testing**.

## Behavior Notes

- Do not implement anything — this skill only plans.
- Reference CLAUDE.md architectural constraints proactively (type duplication rule, process group lifecycle, no shared types package, etc.).
- If the user invokes `/spec` with no description, ask: "What are you speccing out — a feature or a bug fix? Give me a brief description to start."
- Keep questions conversational: one focused message, not a bulleted interrogation. Weave related questions together.
- After writing the file, print the path and a one-line summary of what was written.
