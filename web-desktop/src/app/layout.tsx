import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';
import Footer from '@/components/footer';
import ThemedMain from '@/components/themedMain';

export const metadata: Metadata = {
  title: 'TicketHawk',
  description: 'TicketHawk, a product of Self Hawk Tech Pty Ltd, is South Africa\'s home for discovering, booking, and managing live events.',
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