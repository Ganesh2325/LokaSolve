"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Card } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Assignment = { id: string; issue_id: string; resolver_name: string; status: string; explanation: string };

export default function WorkPage() {
  const { token } = useAuth();
  const [items, setItems] = useState<Assignment[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    api<{ data: Assignment[] }>("/api/v1/assignments", { token })
      .then((response) => setItems(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load work."));
  }, [token]);

  return (
    <div className="flex flex-col gap-3">
      <h1 className="font-serif text-3xl">Work</h1>
      {error ? <Card><p>{error}</p></Card> : null}
      {items.length === 0 && !error ? <Card><p>Nothing is assigned to you.</p></Card> : null}
      <ul className="grid gap-3">
        {items.map((item) => (
          <li key={item.id}>
            <Link href={`/reports/${item.issue_id}`} className="block border border-black bg-card p-3">
              <p className="font-semibold">{item.resolver_name}</p>
              <p className="mt-1 text-sm uppercase">{item.status.replaceAll("_", " ")}</p>
              <p className="mt-2 text-sm">{item.explanation}</p>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
