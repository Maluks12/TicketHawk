# EventsWeb

The web frontend for EventsApp, split into two independently runnable projects:

| Folder | Target | Dev URL |
|--------|--------|---------|
| `desktop/` | Desktop browsers (≥768px) | http://localhost:3000 |
| `mobile/`  | Mobile browsers (<768px)  | http://localhost:3001 |

## Quick start

```bash
# Desktop
cd desktop && npm install && npm run dev

# Mobile (in a separate terminal)
cd mobile && npm install && npm run dev
```

## What's in each folder

Both projects are **self-contained** Next.js apps — they each contain their own
`components/`, `lib/`, `store/`, `themes/`, `src/`, and all config files.
This lets each be developed, linted, and built independently without any
cross-project imports.

Each project's `README.md` documents the files that currently mix mobile and
desktop layout code, so you know exactly where to start separating the concerns.

## Shared code

`lib/`, `store/`, and `themes/` are currently duplicated in both projects so
each is standalone. If you want a single source of truth for those, consider
extracting them into a workspace package (e.g. with npm workspaces or Turborepo)
— but keeping them separate is fine for independent development.
