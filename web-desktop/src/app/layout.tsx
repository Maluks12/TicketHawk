import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';
import Footer from '@/components/footer';
import ThemedMain from '@/components/themedMain';
import ThemeSync from '@/components/themesync';
import ReturnPathTracker from '@/components/returnPathTracker';

export const metadata: Metadata = {
  metadataBase: new URL('https://ticket-hawk.com'),
  title: 'TicketHawk',
  description: 'TicketHawk is South Africa\u2019s event ticket platform. Discover concerts, festivals, sports, nightlife, comedy shows, and more\u2014or create your own event and sell tickets with ease.',
  applicationName: 'TicketHawk',
  // NO alternates.canonical HERE — deliberately.
  //
  // Metadata set on the ROOT LAYOUT is inherited by every child route, so a
  // canonical here does not mean "this site lives at ticket-hawk.com". It
  // means EVERY page declares itself a duplicate of the homepage:
  //
  //   /search    canonical -> https://ticket-hawk.com
  //   /category  canonical -> https://ticket-hawk.com
  //   /merch     canonical -> https://ticket-hawk.com
  //   /support   canonical -> https://ticket-hawk.com   ...and so on
  //
  // A canonical is not a hint. It tells Google to drop that URL and index
  // the target instead, which is why Search Console reported "Alternate
  // page with proper canonical tag" — the pages were working exactly as
  // instructed. Event pages escaped only because events/[id]/layout.tsx
  // overrides this with a self-referencing canonical of its own.
  //
  // With no canonical declared, Google self-canonicalises each URL to
  // itself, which is the correct default for a site where every page is a
  // distinct document.
  //
  // The www vs non-www concern this was written for is real, but a
  // canonical is the wrong instrument: it is per-page, and it cannot stop
  // www.ticket-hawk.com from being crawled in the first place. That belongs
  // at the edge as a 301 redirect (Cloudflare Rules: www -> apex), which
  // resolves the duplicate for every URL at once and passes link equity.
  icons: {
    icon: [
      { url: '/favicon.ico', sizes: 'any' },
      { url: '/icon.png', type: 'image/png' },
    ],
    shortcut: ['/favicon.ico'],
    apple: ['/icon.png'],
  },
  openGraph: {
    type: 'website',
    siteName: 'TicketHawk',
    title: 'TicketHawk',
    description: 'TicketHawk is South Africa\u2019s event ticket platform. Discover concerts, festivals, sports, nightlife, comedy shows, and more\u2014or create your own event and sell tickets with ease.',
    url: 'https://ticket-hawk.com',
    images: ['/icon.png'],
  },
  twitter: {
    card: 'summary',
    title: 'TicketHawk',
    description: 'TicketHawk is South Africa\u2019s event ticket platform. Discover concerts, festivals, sports, nightlife, comedy shows, and more.',
    images: ['/icon.png'],
  },
};

// Organization structured data: the Google-recommended way to associate a
// site name and logo with the domain, so results show 'TicketHawk' + the mark
// rather than guessing from page content.
const orgJsonLd = {
  '@context': 'https://schema.org',
  '@type': 'Organization',
  name: 'TicketHawk',
  url: 'https://ticket-hawk.com',
  logo: 'https://ticket-hawk.com/icon.png',
};

const siteJsonLd = {
  '@context': 'https://schema.org',
  '@type': 'WebSite',
  name: 'TicketHawk',
  url: 'https://ticket-hawk.com',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <head>
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(orgJsonLd) }}
        />
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(siteJsonLd) }}
        />
      </head>
      <body>
        {/* Paints <html>/<body> and the browser chrome from the ACTIVE theme.
            global.css only has a prefers-color-scheme fallback, which tracks the
            OS rather than the theme picked in the app — so a light theme on a
            dark phone left black showing through wherever <main> did not reach. */}
        <ThemeSync />

        {/* Records the last non-auth page visited, so logging in returns the
            user there instead of router.back() landing them on whatever
            happened to precede the login page. */}
        <ReturnPathTracker />

        <NavBar />

        <ThemedMain>{children}</ThemedMain>

        <Footer />

      </body>
    </html>
  );
}