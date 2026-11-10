"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { text } from "@/lib/copy";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Issue = { id: string; description: string; category_label: string | null; status: string; locality_text: string | null };
type Notice = { id: string; read_at: string | null };

export default function HomePage() {
  const { user, token } = useAuth();
  const labels = text(user?.preferred_language ?? "en");
  const [reports, setReports] = useState<Issue[]>([]);
  const [unread, setUnread] = useState(0);

  useEffect(() => {
    if (!token) return;
    api<{ data: Issue[] }>("/api/v1/issues", { token }).then((response) => setReports(response.data.slice(0, 6))).catch(() => setReports([]));
    api<{ data: Notice[] }>("/api/v1/notifications", { token }).then((response) => setUnread(response.data.filter((item) => !item.read_at).length)).catch(() => setUnread(0));
  }, [token]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="font-serif text-3xl">Hello, {user?.display_name}</h1>
        <Link href="/reports/new" className="btn btn-gold shrink-0">{labels.reportProblem}</Link>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1.6fr)_240px]">
        <section className="border border-black bg-card">
          <div className="flex items-center justify-between border-b border-black px-3 py-2">
            <h2 className="font-semibold">Recent reports</h2>
            <Link href="/reports" className="text-sm font-semibold underline">All</Link>
          </div>
          {reports.length === 0 ? <p className="px-3 py-4 text-sm">No reports yet.</p> : null}
          <ul>
            {reports.map((report) => (
              <li key={report.id} className="border-b border-black last:border-b-0">
                <Link href={`/reports/${report.id}`} className="grid gap-1 px-3 py-3 sm:grid-cols-[1fr_auto] sm:items-center">
                  <span>
                    <span className="block font-semibold">{report.category_label || "Unlabeled problem"}</span>
                    <span className="mt-1 block text-sm">{report.locality_text || report.description}</span>
                  </span>
                  <span className="text-xs font-bold uppercase">{report.status.replaceAll("_", " ")}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
        <aside className="grid gap-3">
          <Link href="/notifications" className="border border-black bg-card px-3 py-3">
            <span className="block font-serif text-2xl">{unread}</span>
            <span className="text-sm font-semibold">Unread updates</span>
          </Link>
          <Link href="/nearby" className="border border-black bg-card px-3 py-3 text-sm font-semibold">Nearby</Link>
          <Link href="/work" className="border border-black bg-card px-3 py-3 text-sm font-semibold">Work</Link>
          <Link href="/profile" className="border border-black bg-card px-3 py-3 text-sm font-semibold">Profile</Link>
        </aside>
      </div>
    </div>
  );
}
