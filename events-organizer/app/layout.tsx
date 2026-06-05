import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Organizer — EventsProject',
  description: 'EventsProject organizer dashboard',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}