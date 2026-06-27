import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';

export const metadata: Metadata = {
  title: 'EventsApp',
  description: 'Discover and book events near you',
};

// FIX: viewport-fit=cover lets the bottom tab bar extend edge-to-edge on
// notched/home-indicator devices, and env(safe-area-inset-bottom) then
// correctly pads the bar's content above the home indicator.
export const viewport: Viewport = {
  viewportFit: 'cover',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <NavBar />

        {/* pb-[60px] on mobile reserves space for the fixed bottom tab bar.
            md:pb-0 removes it on desktop where the top nav is used instead. */}
        <main className="pb-[60px] md:pb-0">{children}</main>
      </body>
    </html>
  );
}