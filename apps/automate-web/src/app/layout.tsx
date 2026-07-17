import type { ReactNode } from 'react';
import './globals.css';
import { Providers } from '@/components/Providers';

export const metadata = {
  title: 'MyWork Automate',
  description: 'Node-based workflow automation for HR operations',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="th">
      <body className="font-sans">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
