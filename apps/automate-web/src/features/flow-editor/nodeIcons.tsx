// Maps a NodeType.icon (a lucide-react icon name string) to its component.
// Both the palette and the custom canvas node render icons through this record
// so adding a node type only needs a new entry here if it uses a new icon.
import {
  CalendarClock,
  MousePointerClick,
  Database,
  GitBranch,
  Shuffle,
  FileSpreadsheet,
  Send,
  Mail,
  Download,
  Box,
  type LucideIcon,
} from 'lucide-react';

const ICONS: Record<string, LucideIcon> = {
  CalendarClock,
  MousePointerClick,
  Database,
  GitBranch,
  Shuffle,
  FileSpreadsheet,
  Send,
  Mail,
  Download,
};

/** Resolve a lucide icon name to a component, falling back to a neutral Box. */
export function nodeIcon(name: string): LucideIcon {
  return ICONS[name] ?? Box;
}
