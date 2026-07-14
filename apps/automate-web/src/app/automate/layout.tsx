import type { ReactNode } from 'react';
import { Sidebar } from '@/components/shell/Sidebar';

export default function AutomateLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">{children}</div>
    </div>
  );
}
