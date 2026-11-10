"use client";

import { useEffect, useState } from "react";
import { Card } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Item = { id: string; category_label: string | null; locality_text: string | null; summary: string; status: string };

export default function NearbyPage() {
  const { token } = useAuth();
  const [items, setItems] = useState<Item[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    api<{ data: Item[] }>("/api/v1/nearby", { token })
      .then((response) => setItems(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load nearby reports."));
  }, [token]);

  return (
    <div className="flex h-full flex-col gap-3">
      <h1 className="font-serif text-3xl">Nearby</h1>
      {error ? <Card><p>{error}</p></Card> : null}
      {items.length === 0 && !error ? <Card><p>No public reports in view. Names and exact home locations are not shown here.</p></Card> : null}
      <ul className="grid gap-3">
        {items.map((item) => (
          <li key={item.id} className="border border-black bg-card p-3">
            <p className="font-semibold">{item.category_label || "Unlabeled problem"}</p>
            <p className="mt-1 text-sm">{item.locality_text || "Locality not written"} · {item.status.replaceAll("_", " ")}</p>
            <p className="mt-2 text-sm">{item.summary}</p>
          </li>
        ))}
      </ul>
    </div>
  );
}
