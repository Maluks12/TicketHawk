import type { Metadata } from 'next';
import './global.css';
import NavBar from '@/components/navBar';

export const metadata: Metadata = {
  title: 'EventsApp',
  description: 'Discover and book events near you',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <NavBar />
        <main>{children}</main>
      </body>
    </html>
  );
}