# EventsWeb — Desktop

This is the **desktop web** version of the EventsApp web project, built with Next.js + Tailwind CSS.

## Getting started

```bash
npm install
npm run dev       # http://localhost:3000
```

## Structure

```
desktop/
├── components/       # UI components (NavBar, modals, etc.)
├── lib/              # API client (api.ts) + image helpers
├── store/            # Zustand state (auth, events, filter, theme)
├── themes/           # Color tokens (colors.ts)
├── src/
│   └── app/          # Next.js App Router pages
│       ├── page.tsx              # Home / discover
│       ├── events/[id]/          # Event detail + purchase
│       ├── merch/                # Merchandise
│       ├── cart/                 # Cart + checkout
│       ├── tickets/              # My tickets
│       ├── search/               # Search
│       ├── profile/              # Profile + following
│       ├── booking/[id]/         # Booking
│       ├── payment/[id]/         # Payment confirmation
│       ├── category/[name]/      # Category browse
│       ├── section/[title]/      # Section listing
│       └── auth/login/           # Login
├── next.config.ts
├── postcss.config.mjs
└── tsconfig.json
```

## Desktop-specific areas to focus on

The following files currently contain **both** mobile and desktop layouts — use
`md:` Tailwind breakpoints to keep desktop layout and strip or ignore the mobile
branches as you develop the desktop-only experience:

| File | Mobile/desktop split |
|------|---------------------|
| `components/navBar.tsx` | `hidden md:flex` (desktop nav) vs `md:hidden` (hamburger) |
| `src/app/page.tsx` | Category pill-tabs (`hidden md:flex`) vs dropdown (`flex md:hidden`) |
| `src/app/events/[id]/page.tsx` | `.ed-mobile-cta` via `@media (max-width: 860px)` |
| `src/app/layout.tsx` | `pb-[60px] md:pb-0` bottom-nav padding |

## Dev port

`3000` — run alongside the mobile app on `3001`.
