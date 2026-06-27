# EventsWeb — Mobile Web

This is the **mobile web** version of the EventsApp web project, built with Next.js + Tailwind CSS.

## Getting started

```bash
npm install
npm run dev       # http://localhost:3001
```

> Tip: open Chrome DevTools → toggle device toolbar (Ctrl/Cmd+Shift+M) to emulate a phone viewport.

## Structure

```
mobile/
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

## Mobile-specific areas to focus on

The following files currently contain **both** mobile and desktop layouts — use
the mobile (non-prefixed / `md:hidden`) Tailwind branches and strip the desktop
`hidden md:flex` variants as you develop the mobile-only experience:

| File | Mobile/desktop split |
|------|---------------------|
| `components/navBar.tsx` | `md:hidden` hamburger menu is mobile; `hidden md:flex` nav is desktop |
| `src/app/page.tsx` | Dropdown category selector (`flex md:hidden`) is mobile |
| `src/app/events/[id]/page.tsx` | `.ed-mobile-cta` sticky bottom CTA (shown ≤860px) |
| `src/app/layout.tsx` | `pb-[60px]` bottom-nav padding is for mobile; `md:pb-0` is desktop |

## Dev port

`3001` — run alongside the desktop app on `3000`.
