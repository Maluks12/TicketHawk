import type { Metadata } from 'next';
import './global.css'; // FIX: was './globals.css' — file is global.css (no 's')
import NavBar from '@/components/navBar'; // FIX: was '@/components/NavBar' — file is navBar.tsx (lowercase n)

export const metadata: Metadata = {
  title: 'EventsApp',
  description: 'Discover and book events near you',
};

export { default, metadata } from '../website/layout';

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