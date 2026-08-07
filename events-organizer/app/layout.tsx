import type { Metadata, Viewport } from 'next';
import './globals.css';
import { ThemeProvider, THEME_SCRIPT } from '../lib/theme';

export const metadata: Metadata = {
  title: 'Studio — TicketHawk',
  description: 'TicketHawk organizer dashboard',
};

// Tells the browser chrome (scrollbars, form controls, address bar on mobile)
// which palette is in play, per theme.
export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: dark)', color: '#08080C' },
    { media: '(prefers-color-scheme: light)', color: '#F4F5F9' },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    // data-theme is stamped by THEME_SCRIPT before paint, so the server HTML
    // and the pre-hydration DOM differ by that one attribute on purpose.
    // suppressHydrationWarning scopes the mismatch to <html> only.
    <html lang="en" data-theme="dark" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
      </head>
      <body>
        <ThemeProvider>{children}</ThemeProvider>
      </body>
    </html>
  );
}