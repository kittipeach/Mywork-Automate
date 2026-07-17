'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { LayoutDashboard, Workflow, History, Database, FolderOpen, Shield, Zap } from 'lucide-react';
import { cn } from '@/lib/cn';

const NAV = [
  { href: '/automate', label: 'Dashboard', icon: LayoutDashboard },
  { href: '/automate/flows', label: 'Flows', icon: Workflow },
  { href: '/automate/runs', label: 'Run History', icon: History },
  { href: '/automate/connections', label: 'Connections', icon: Database },
  { href: '/automate/files', label: 'My Files', icon: FolderOpen },
  { href: '/automate/admin', label: 'Admin & RBAC', icon: Shield },
];

export function Sidebar() {
  const pathname = usePathname();
  return (
    <aside className="flex w-60 shrink-0 flex-col bg-sidebar text-sidebar-fg">
      <div className="flex h-14 items-center gap-2 px-5">
        <div className="grid h-8 w-8 place-items-center rounded-md bg-brand text-brand-fg">
          <Zap className="h-4 w-4" />
        </div>
        <div className="leading-tight">
          <div className="text-sm font-semibold">MyWork</div>
          <div className="text-[11px] text-sidebar-muted">Automate</div>
        </div>
      </div>
      <nav className="flex-1 space-y-1 px-3 py-3">
        {NAV.map(({ href, label, icon: Icon }) => {
          const active = href === '/automate' ? pathname === href : pathname.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                active ? 'bg-sidebar-active text-white' : 'text-sidebar-muted hover:bg-white/5 hover:text-sidebar-fg',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </Link>
          );
        })}
      </nav>
      <div className="border-t border-white/10 px-5 py-3 text-[11px] text-sidebar-muted">
        Phase 1 · MVP
      </div>
    </aside>
  );
}
