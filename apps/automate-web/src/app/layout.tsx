import type { ReactNode } from 'react';

export const metadata = {
  title: 'MyWork Automate',
  description: 'Node-based workflow automation',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="th">
      <body>{children}</body>
    </html>
  );
}
