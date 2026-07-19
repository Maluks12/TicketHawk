import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';
import Footer from '@/components/footer';
import ThemedMain from '@/components/themedMain';

export const metadata: Metadata = {
  title: 'TicketHawk',
  description: 'TicketHawk is South Africa\u2019s event ticket platform. Discover concerts, festivals, sports, nightlife, comedy shows, and more\u2014or create your own event and sell tickets with ease.',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <NavBar />

        <ThemedMain>{children}</ThemedMain>

        <Footer />

      </body>
    </html>
  );
}