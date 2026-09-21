import type { Metadata, Viewport } from 'next';
import './globals.css';
import { ThemeProvider, THEME_SCRIPT } from '../lib/theme';
import ServiceBanner from '../components/serviceBanner';
import { SITE_URL } from './site';

// ─────────────────────────────────────────────────────────────────────────────
// Root metadata
//
// This app is two different things wearing one shell:
//
//   /             a public marketing landing page that SHOULD rank
//   everything…   a signed-in dashboard that should not be indexed at all
//
// So the defaults here are written for the landing page — the only route a
// crawler can actually reach — and the dashboard stays out of the index via
// robots.txt, which already disallows every authenticated path.
//
// What was missing before, and why each one mattered:
//
//   • metadataBase   Without it, relative OG/Twitter image URLs never resolve
//                    to absolute ones, so every social preview was imageless.
//   • icons          There was NO favicon anywhere in the project. That is why
//                    the browser tab showed a generic globe.
//   • title template Child pages set a bare title and lost the brand entirely.
// ─────────────────────────────────────────────────────────────────────────────

export const metadata: Metadata = {
  // Absolute base for every relative URL below. Reads NEXT_PUBLIC_ORGANISER_URL
  // through site.ts, so a staging deploy doesn't claim production URLs.
  metadataBase: new URL(SITE_URL),

  title: {
    // Used by the landing page and anything that sets no title of its own.
    default: 'TicketHawk Studio — Sell tickets in South Africa',
    // Child pages supply the specific part; the brand is appended once, here,
    // rather than being retyped (and mistyped) on every page.
    template: '%s · TicketHawk Studio',
  },
  description:
    'Create events, sell tickets and scan at the door. TicketHawk Studio is the '
    + 'organiser dashboard for South Africa\u2019s event ticketing platform — with '
    + 'Home Affairs-verified identity, fast payouts and offline door scanning.',
  applicationName: 'TicketHawk Studio',

  // Next serves app/icon.png and app/apple-icon.png automatically; declaring
  // them gives the shortcut and apple slots a definite answer rather than
  // leaving each browser to guess.
  icons: {
    icon: [{ url: '/icon.png', type: 'image/png' }],
    shortcut: ['/icon.png'],
    apple: ['/apple-icon.png'],
  },

  openGraph: {
    type: 'website',
    siteName: 'TicketHawk Studio',
    title: 'TicketHawk Studio — Sell tickets in South Africa',
    description:
      'Create events, sell tickets and scan at the door. Built for South African organisers.',
    url: SITE_URL,
    images: ['/brand-logo.png'],
    locale: 'en_ZA',
  },
  twitter: {
    card: 'summary',
    title: 'TicketHawk Studio',
    description: 'Sell tickets, scan at the door, and get paid fast.',
    images: ['/brand-logo.png'],
  },

  // NOT a global noindex. The landing page is the one route worth ranking, and
  // a root-level noindex would suppress it along with the dashboard. The
  // authenticated routes are excluded by robots.txt instead — the right layer
  // for a path-based rule.
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      'max-image-preview': 'large',
      'max-snippet': -1,
    },
  },

  // Deliberately NO `alternates.canonical` here.
  //
  // A canonical on the root layout is inherited by every child route, which
  // would tell Google that every page is a duplicate of the landing page. (That
  // is exactly the bug on web-desktop and web-mobile.) The landing page sets
  // its own canonical in app/page.tsx, which is the correct scope for it.
};

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: dark)', color: '#08080C' },
    { media: '(prefers-color-scheme: light)', color: '#F4F5F9' },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    // data-theme is stamped by THEME_SCRIPT before paint, so the server HTML and
    // the pre-hydration DOM differ by that one attribute on purpose.
    // suppressHydrationWarning scopes the mismatch to <html> only.
    //
    // lang is en-ZA, not en: it tells search engines the regional variant, which
    // matters for a product whose whole market is one country.
    <html lang="en-ZA" data-theme="dark" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
      </head>
      <body>
        <ThemeProvider>
          {/* Maintenance / outage notice, on every route — dashboard, login
              and landing page alike. Renders nothing when healthy. It
              publishes its height as --service-banner-h, which the
              one-viewport-tall dashboard shell subtracts from 100dvh. */}
          <ServiceBanner />
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}