"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  FileText,
  MessageSquare,
  Upload,
  History,
} from "lucide-react";
import { cn } from "@/lib/utils";

const navItems = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  { href: "/documents", label: "Documents", icon: FileText },
  { href: "/documents/new", label: "Upload", icon: Upload },
  { href: "/chat", label: "Chat", icon: MessageSquare },
  { href: "/ingestion", label: "Ingestion", icon: History },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="w-56 bg-surface border-r border-border flex flex-col h-full shrink-0">
      <div className="p-4 border-b border-border">
        <Link href="/" className="flex items-center gap-2">
          <span className="text-accent font-bold text-lg">pro-rag</span>
          <span className="text-text-dim text-xs">v1</span>
        </Link>
      </div>
      <nav className="flex-1 p-2 space-y-1">
        {navItems.map((item) => {
          const isActive =
            item.href === "/"
              ? pathname === "/"
              : pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors",
                isActive
                  ? "bg-accent/10 text-accent"
                  : "text-text-dim hover:text-foreground hover:bg-surface-2"
              )}
            >
              <item.icon className="w-4 h-4" />
              {item.label}
            </Link>
          );
        })}
      </nav>
      <div className="p-4 border-t border-border text-xs text-text-dim">
        Tenant: <span className="font-mono">...0001</span>
      </div>
    </aside>
  );
}
