# Frontend Design

Stack, component choices, and design system for the React/TypeScript dashboard in `frontend/`. Companion to [`architecture.md`](architecture.md), which owns the dashboard's functional scope (pod list view, pod detail view).

## Stack

| Concern | Choice | Why |
|---|---|---|
| Build tool | Vite | Fast dev server and HMR, minimal config. |
| Framework | React 19 + TypeScript | Matches the rest of the stack's preference for static typing; the dashboard reads a typed REST API from the Go agent. |
| Styling | Tailwind CSS v4 (`@tailwindcss/vite`) | Utility layer over a fully custom theme (see Design Tokens below) — deliberately *not* a themed component kit like MUI/Chakra/Ant, which would impose their own visual identity over PodSentinel's. |
| Fonts | `@fontsource/ibm-plex-mono`, `@fontsource/ibm-plex-sans` | Self-hosted (no external font CDN request). Same superfamily used for both display and body type — see Design Tokens. |
| Routing | React Router | Planned for pod list ↔ pod detail navigation. Not yet installed — the app is a single landing page today. |
| Data fetching | TanStack Query | Planned for polling the Go agent's REST API (pod list, live metrics, anomaly history) per the polling/refresh model in `architecture.md`. Not yet installed. |
| Charts | visx | Planned for the pod detail view's CPU/memory time-series and anomaly timeline. Chosen over a higher-level charting library (Recharts, Nivo) because the design calls for a custom oscilloscope-style trace rather than a stock chart look — visx gives D3-level control over the mark while staying React-idiomatic. Not yet installed. |
| Icons | Lucide React | Planned, unopinionated stroke icon set. Not yet installed — add when a view actually needs icons rather than carrying an unused dependency. |
| Pod list table | Hand-rolled (CSS grid/flex) | Single cluster, small dataset — a table library (e.g. TanStack Table) isn't earning its weight yet. Revisit if sorting/filtering needs grow. |
| Testing | Vitest + React Testing Library | Native Vite pairing, consistent with the Go/Python unit-testing approach in `architecture.md`. Not yet configured. |

Deliberately avoided: a full component-kit (MUI, Chakra, Ant, shadcn's whole system). They ship a default visual language that fights against a custom identity; PodSentinel's dashboard instead composes small, focused, mostly headless libraries on top of custom design tokens.

## Design Concept

The dashboard's core mechanism — a rolling per-pod baseline, flagged the moment a metric deviates — is structurally a **vital-signs monitor**: a patient monitor watches a baseline rhythm and alarms on deviation, the same shape as the z-score/EWMA detector in `architecture.md`. The visual language leans into that directly rather than a generic dashboard look: dark "monitor housing" surfaces, oscilloscope-style trace lines for metrics, and status color reserved for actual stable/alert states rather than decoration.

## Design Tokens

Defined in `frontend/src/index.css` via Tailwind v4's `@theme`.

**Color** — dark teal-charcoal housing (not pure black), warm phosphor amber as the primary signal color, coral reserved only for anomaly/alert states:

| Token | Hex | Use |
|---|---|---|
| `--color-scope-bg` | `#0f1a1c` | Page background ("monitor housing") |
| `--color-panel` | `#16262a` | Card/panel surface |
| `--color-panel-raised` | `#1c2e32` | Raised/hover surface |
| `--color-grid` | `#223639` | Borders, gridlines, dividers |
| `--color-trace` | `#f2a93b` | Primary accent — stable metric traces, brand mark |
| `--color-trace-dim` | `#8a6428` | Muted amber, low-emphasis accents |
| `--color-alert` | `#ff6452` | Anomaly/critical state only |
| `--color-stable` | `#59b896` | Stable/healthy status |
| `--color-ink` | `#ede9e2` | Primary text (warm off-white, not pure white) |
| `--color-ink-dim` | `#93a6a4` | Secondary text |
| `--color-ink-faint` | `#5c7371` | Tertiary/label text |

**Type** — IBM Plex Mono and IBM Plex Sans are the same superfamily (designed by IBM for technical/enterprise products), used for distinct roles rather than picked at random:

| Token | Family | Use |
|---|---|---|
| `--font-display` | IBM Plex Mono | Large headline type, set as a "readout" rather than a conventional display face |
| `--font-body` | IBM Plex Sans | Body copy, prose |
| `--font-mono` | IBM Plex Mono | Data readouts, labels, timestamps, numeric values |

**Signature element**: the `VitalTrace` component (`frontend/src/components/VitalTrace.tsx`) — an SVG sparkline with a dim static waveform plus a brighter looping "sweep" segment layered on top, mimicking an oscilloscope trace. Used per-pod in `MonitorCard` (`frontend/src/components/MonitorCard.tsx`) for CPU/memory, with `variant="alert"` switching the trace and status dot to coral. All animation respects `prefers-reduced-motion` (global override in `index.css`).

## Component Inventory

- `VitalTrace` — oscilloscope-style sparkline, `stable` | `alert` variants.
- `MonitorCard` — namespace/pod identity, status dot, `VitalTrace`, metric readout. Built for the landing page's sample monitor bank; expected to be reused (or adapted) for the real pod list view once the Go REST API exists.

## Not Yet Built

The pod list view and pod detail view described in `architecture.md` don't exist yet — the current app is a landing page introducing the project. Building them is blocked on the Go agent's REST API; until then, any dashboard work uses illustrative sample data (see `frontend/src/data/sampleTraces.ts`).
