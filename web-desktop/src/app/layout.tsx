import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import './styles/global.css';
import NavBar from '@/components/navBar';

export const metadata: Metadata = {
  title: 'EventsApp',
  description: 'Discover and book events near you',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <NavBar />

        <main className="pb-[60px] md:pb-0">{children}</main>
        
      </body>
    </html>
  );
}