"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { text } from "@/lib/copy";
import { useAuth } from "@/lib/auth";

const appLinks = [
  { href: "/home", key: "dashboard" as const },
  { href: "/nearby", key: "nearby" as const },
  { href: "/reports", key: "reports" as const },
  { href: "/work", key: "work" as const },
  { href: "/notifications", key: "notifications" as const },
  { href: "/profile", key: "profile" as const },
] as const;

export function PrivateShell({ children }: { children: React.ReactNode }) {
  const { ready, user, signOut } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const [mounted, setMounted] = useState(false);
  const labels = text(user?.preferred_language ?? "en");

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (mounted && ready && !user) router.replace("/login");
  }, [mounted, ready, user, router]);

  useEffect(() => {
    if (user?.preferred_language) document.documentElement.lang = user.preferred_language;
  }, [user?.preferred_language]);

  if (!mounted || !ready || !user) {
    return <p className="p-6 text-muted" suppressHydrationWarning>Loading your account…</p>;
  }

  const operations =
    user.roles.includes("super_admin") || user.roles.includes("support_agent")
      ? [
          { href: "/admin", label: labels.operations },
          { href: "/authorities", label: "Authorities" },
          { href: "/cases", label: "Cases" },
        ]
      : [];

  return (
    <div className="h-dvh overflow-hidden bg-paper md:grid md:grid-cols-[220px_minmax(0,1fr)]">
      <aside className="hidden h-dvh bg-black text-white md:flex md:flex-col md:p-4">
        <Brand tone="light" />
        <nav aria-label="Primary" className="mt-8 flex flex-col gap-1">
          {appLinks.map((link) => (
            <NavLink key={link.href} href={link.href} active={pathname === link.href || pathname.startsWith(`${link.href}/`)}>
              {labels[link.key]}
            </NavLink>
          ))}
          <NavLink href="/reports/new" active={pathname === "/reports/new"}>
            {labels.reportProblem}
          </NavLink>
          <NavLink href="/incidents" active={pathname === "/incidents" || pathname.startsWith("/incidents/")}>
            Incidents
          </NavLink>
          {operations.map((link) => (
            <NavLink key={link.href} href={link.href} active={pathname === link.href}>
              {link.label}
            </NavLink>
          ))}
        </nav>
        <button type="button" className="mt-auto min-h-11 text-left text-sm font-semibold" onClick={() => signOut().then(() => router.push("/"))}>
          {labels.signOut}
        </button>
      </aside>
      <div className="flex h-full min-h-0 min-w-0 flex-col">
        <header className="flex shrink-0 items-center justify-between border-b border-line bg-paper px-4 py-3 md:hidden">
          <Brand tone="dark" />
          <Link href="/incidents" className="min-h-11 text-sm font-semibold">Incidents</Link>
          <button type="button" className="min-h-11 text-sm font-semibold" onClick={() => signOut().then(() => router.push("/"))}>
            {labels.signOut}
          </button>
        </header>
        {operations.length > 0 ? (
          <nav aria-label="More" className="flex gap-3 overflow-x-auto border-b border-line bg-paper px-4 py-2 md:hidden">
            {operations.map((link) => (
              <Link key={link.href} href={link.href} className={`min-h-11 shrink-0 text-sm font-semibold ${pathname === link.href ? "text-teal" : "text-muted"}`}>
                {link.label}
              </Link>
            ))}
          </nav>
        ) : null}
        {user.is_demo ? <p className="border-b border-black bg-paper px-4 py-2 text-sm">This is a labeled demo account.</p> : null}
        <main id="main" className="mx-auto min-h-0 w-full max-w-5xl flex-1 overflow-y-auto px-4 py-4 pb-20 md:px-6 md:py-5 md:pb-5">
          {children}
        </main>
        <nav aria-label="Mobile" className="fixed inset-x-0 bottom-0 grid grid-cols-6 border-t border-line bg-paper md:hidden">
          {appLinks.map((link) => (
            <Link key={link.href} href={link.href} className={`flex min-h-14 items-center justify-center border-t-2 px-1 text-center text-xs font-semibold ${pathname === link.href ? "border-[#c4a15a] text-black" : "border-transparent text-muted"}`}>
              {labels[link.key]}
            </Link>
          ))}
        </nav>
      </div>
    </div>
  );
}

function Brand({ tone }: { tone: "light" | "dark" }) {
  return (
    <Link href="/home" className={`font-serif text-xl font-semibold ${tone === "light" ? "text-white" : "text-black"}`}>
      LokaSolve
    </Link>
  );
}

function NavLink({ href, active, children }: { href: string; active: boolean; children: React.ReactNode }) {
  return (
    <Link href={href} className={`rounded-lg px-3 py-2 text-sm font-semibold ${active ? "bg-[#c4a15a] text-black" : "text-white"}`}>
      {children}
    </Link>
  );
}
