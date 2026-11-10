"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Card } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Referral = { id: string; issue_id: string; authority_name: string; status: string; channel: string; external_reference: string | null };

export default function CasesPage() {
  const { token } = useAuth();
  const [items, setItems] = useState<Referral[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    api<{ data: Referral[] }>("/api/v1/admin/referrals", { token })
      .then((response) => setItems(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load cases."));
  }, [token]);

  return (
    <div className="flex flex-col gap-3">
      <h1 className="font-serif text-3xl">Cases</h1>
      {error ? <Card><p>{error}</p></Card> : null}
      {items.length === 0 && !error ? <Card><p>No complaint packets yet.</p></Card> : null}
      <ul className="grid gap-3">
        {items.map((item) => (
          <li key={item.id}>
            <Link href={`/reports/${item.issue_id}`} className="block border border-black bg-card p-3">
              <p className="font-semibold">{item.authority_name}</p>
              <p className="mt-1 text-sm uppercase">{item.status.replaceAll("_", " ")} · {item.channel}</p>
              {item.external_reference ? <p className="mt-1 text-sm">Reference {item.external_reference}</p> : null}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
