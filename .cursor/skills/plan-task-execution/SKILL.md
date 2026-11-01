---
name: plan-task-execution
description: >-
  Execute a task from the "Senior Portfolio Upgrade" rebuild plan (task IDs
  T0.1-T14.3). Use whenever implementing, validating, or reviewing any plan
  task, especially when run by a fast/cheap model. Defines model-tier rules,
  validation-gate discipline, escalation, and evidence requirements.
---

# Plan Task Execution

The rebuild plan lives at `.cursor/plans/senior_portfolio_upgrade_1dc51438.plan.md` (or workspace `.cursor/plans/`). Every task there has: ID, Tier, Depends, Do, Validate.

## Before starting any task

1. Read the plan's "EXECUTOR PROTOCOL" section and the "Locked architecture decisions" table. Do not deviate from locked decisions for any reason.
2. Confirm the task's **Depends** tasks are complete (their Validate blocks passed).
3. Check the tier:
   - **[FAST]** — proceed. Follow the Do steps literally. Do not invent architecture, add features, or restructure anything the steps don't mention.
   - **[HIGH]** or **[REVIEW]** — these require a high-end model (Fable/Opus class). If you are a fast model, stop and report that this task needs escalation. HIGH tasks: T1.1, T2.1, T3.3, T3.4, T4.2, T5.1, T7.2, T7.3, T9.2, T10.2, T11.2, T12.1, T13.1, T14.1.

## While working

- Map every file you touch to the task. If you need to touch a file the task doesn't imply, stop and ask.
- Generated code (`gen/`, `frontend/src/realtime/gen/`, sqlc output) is never hand-edited. Fix `proto/` or `queries/*.sql` and regenerate (`task proto`, `sqlc generate`).
- `legacy-backend/` is read-only reference. Never extend or import it.
- Follow the karpathy-guidelines skill: surgical changes, minimum code, no speculative abstractions.

## Finishing a task (validation gate)

1. Run EVERY command in the task's **Validate** block. Paste real output as evidence.
2. Also run the global gates relevant to what you touched:
   - Go: `golangci-lint run ./...`, `go test ./... -race`, `go vet ./...`
   - Proto: `buf lint`, `buf breaking --against '.git#branch=main'`, `task proto && git diff --exit-code`
   - Frontend: `npx tsc --noEmit`, `npm run lint`, `npm run build`
3. A task is DONE only when every check passed with shown output. "Should work" is not done.
4. Commit with a conventional-commit message referencing the task ID, e.g. `feat(board): T5.2 jetstream consumer + grpc wiring`.

## Failure and escalation

- If a Validate command fails twice and the fix isn't obvious from the error, STOP. Write down: the task ID, the exact failing command, full error output, and what you tried. Escalate to a high-end model or the user.
- Never work around a failing validation by weakening the check, skipping the test, or changing the plan's design.
- If the plan's steps conflict with reality (e.g., a dependency version no longer exists), stop and flag it — do not silently substitute.

## Phase completion

Every phase ends with a [REVIEW] task: a high-end model reads the full phase diff, re-runs all phase validations, checks exit criteria, and either fixes issues or files them. The next phase must not start before the review passes.
