"use client";

import { PrivateShell } from "@/components/shell";

export default function Layout({ children }: { children: React.ReactNode }) {
  return <PrivateShell>{children}</PrivateShell>;
}
