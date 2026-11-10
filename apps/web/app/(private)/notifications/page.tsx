"use client";

import { useEffect, useState } from "react";
import { Button, Card } from "@/components/ui";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

type Notice = { id: string; title: string; body: string; read_at: string | null; created_at: string };

export default function NotificationsPage() {
  const { token } = useAuth();
  const [items, setItems] = useState<Notice[]>([]);
  const [error, setError] = useState("");

  function load() {
    if (!token) return;
    api<{ data: Notice[] }>("/api/v1/notifications", { token })
      .then((response) => setItems(response.data))
      .catch((caught) => setError(caught instanceof ApiError ? caught.message : "Could not load notifications."));
  }

  useEffect(() => {
    load();
  }, [token]);

  async function mark(id: string) {
    await api(`/api/v1/notifications/${id}/read`, { method: "POST", token });
    load();
  }

  return (
    <div className="flex flex-col gap-3">
      <h1 className="font-serif text-3xl">Notifications</h1>
      {error ? <Card><p>{error}</p></Card> : null}
      {items.length === 0 && !error ? <Card><p>No updates yet. Email, SMS, and WhatsApp are not connected.</p></Card> : null}
      <ul className="flex flex-col gap-3">
        {items.map((item) => (
          <li key={item.id} className="border border-black bg-card p-3">
            <p className="font-semibold">{item.title}</p>
            <p className="mt-1 text-sm">{item.body}</p>
            {item.read_at ? null : <Button className="mt-3" onClick={() => mark(item.id)}>Mark read</Button>}
          </li>
        ))}
      </ul>
    </div>
  );
}
