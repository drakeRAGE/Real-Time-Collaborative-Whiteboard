---
name: frontend-standards
description: >-
  Frontend standards for the React 19 + strict TypeScript rebuild. Use when
  writing or reviewing anything under frontend/: realtime client usage,
  providers, component structure, accessibility, and quality gates.
---

# Frontend Standards

## TypeScript

- `strict: true` + `noUncheckedIndexedAccess`. No new `any` — during the migration window (Phase 9) existing `any` must carry `// TODO(types)` and the total count must only go down (hard cap 20).
- All `VITE_*` env vars typed in `vite-env.d.ts`. No untyped `import.meta.env` access.
- WS message types come ONLY from `frontend/src/realtime/gen/` (ts-proto output). Never hand-write a type that mirrors a proto message.

## Realtime

- Exactly ONE WebSocket connection app-wide, owned by `SocketProvider` wrapping `RealtimeClient` (`frontend/src/realtime/client.ts`). Components never call `new WebSocket`/`io(...)` directly — they use `useSocket()` / `useRoom()`.
- Stroke and cursor emissions go through `useStrokeBatcher` (rAF-coalesced strokes, cursors throttled to 30/s). No raw per-mousemove sends.
- Client-generated ULIDs on strokes/chat for idempotent replay; offline queue and reconnect/backoff live in RealtimeClient only — do not duplicate reconnect logic in components.

## Components

- Whiteboard is split: `CanvasBoard` (render + pointer input), `Toolbar`, hooks for logic. Keep orchestration components < 250 lines; extract hooks instead of growing files.
- Canvas replay logic lives in `canvasUtils.ts` only — history load and live events use the same function.
- Error handling: route-level error boundaries (`react-error-boundary`); user-facing failures show toasts or inline UI — `alert()` is forbidden. RealtimeClient connection state drives a visible banner (reconnecting/offline + retry).

## Accessibility (review blockers)

- Every icon-only button has `aria-label`. Modals: `role="dialog"`, `aria-modal`, focus trap, Esc closes. Chat message list has `aria-live="polite"`. Forms use real `<label htmlFor>` and `type="submit"`.

## Done means

`npx tsc --noEmit` clean, `npm run lint` clean, `npm run build` succeeds, relevant Vitest suites green, and the plan task's Validate block passed with shown output.
