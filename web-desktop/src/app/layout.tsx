import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';
import Footer from '@/components/footer';
import ThemedMain from '@/components/themedMain';

export const metadata: Metadata = {
  metadataBase: new URL('https://ticket-hawk.com'),
  title: 'TicketHawk',
  description: 'TicketHawk is South Africa\u2019s event ticket platform. Discover concerts, festivals, sports, nightlife, comedy shows, and more\u2014or create your own event and sell tickets with ease.',
  applicationName: 'TicketHawk',
  // Canonical to the bare domain so search engines don't treat
  // ticket-hawk.com and www.ticket-hawk.com as two competing results.
  alternates: { canonical: 'https://ticket-hawk.com' },
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
        <NavBar />

        <ThemedMain>{children}</ThemedMain>

        <Footer />

      </body>
    </html>
  );
}