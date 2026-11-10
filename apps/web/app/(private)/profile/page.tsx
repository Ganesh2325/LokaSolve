"use client";

import { useState } from "react";
import { Button, Card, ErrorText, Field, Notice, controlClass } from "@/components/ui";
import { ApiError, api, type SavedPlace, type User } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useEffect } from "react";

export default function ProfilePage() {
  const { user, token, refreshUser } = useAuth();
  const [name, setName] = useState(user?.display_name ?? "");
  const [language, setLanguage] = useState(user?.preferred_language ?? "en");
  const [whatsapp, setWhatsapp] = useState(Boolean(user?.notification_preferences.whatsapp));
  const [places, setPlaces] = useState<SavedPlace[]>([]);
  const [address, setAddress] = useState("");
  const [label, setLabel] = useState("Home");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    api<{ data: SavedPlace[] }>("/api/v1/me/locations", { token }).then((response) => setPlaces(response.data)).catch(() => setPlaces([]));
  }, [token]);

  async function saveProfile(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api<User>("/api/v1/me", {
        method: "PATCH",
        token,
        body: { display_name: name, preferred_language: language, notification_preferences: { whatsapp } },
      });
      await refreshUser();
      setMessage("Profile saved.");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not save the profile.");
    }
  }

  async function addPlace(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const place = await api<SavedPlace>("/api/v1/me/locations", {
        method: "POST",
        token,
        body: { source: "user_address", address_text: address, label },
      });
      setPlaces([place, ...places]);
      setAddress("");
      setMessage("Place saved. No coordinates were added.");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not save the place.");
    }
  }

  async function requestDeletion() {
    setError("");
    try {
      await api("/api/v1/me/deletion-request", { method: "POST", token, body: {} });
      setMessage("Deletion request submitted. The account stays usable until an operator completes it.");
      await refreshUser();
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not submit the request.");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-serif text-4xl">Profile</h1>
      {message ? <Notice>{message}</Notice> : null}
      {error ? <ErrorText error={error} /> : null}
      <Card>
        <form className="flex flex-col gap-4" onSubmit={saveProfile}>
          <Field label="Name">
            <input className={controlClass} value={name} onChange={(event) => setName(event.target.value)} />
          </Field>
          <Field label="Language">
            <select className={controlClass} value={language} onChange={(event) => setLanguage(event.target.value)}>
              <option value="en">English</option>
              <option value="hi">हिन्दी</option>
            </select>
          </Field>
          <label className="flex min-h-11 items-center gap-3 text-sm">
            <input type="checkbox" checked={whatsapp} onChange={(event) => setWhatsapp(event.target.checked)} />
            Allow WhatsApp messages. This build does not send them.
          </label>
          <Button type="submit">Save profile</Button>
        </form>
      </Card>
      <Card>
        <h2 className="font-serif text-2xl">Saved places</h2>
        <p className="mt-2 text-sm text-muted">An address is stored as you typed it. The service does not invent a map point.</p>
        <form className="mt-4 flex flex-col gap-3" onSubmit={addPlace}>
          <Field label="Label">
            <input className={controlClass} value={label} onChange={(event) => setLabel(event.target.value)} />
          </Field>
          <Field label="Address">
            <input className={controlClass} required value={address} onChange={(event) => setAddress(event.target.value)} />
          </Field>
          <Button type="submit">Save place</Button>
        </form>
        <ul className="mt-4 flex flex-col gap-2">
          {places.length === 0 ? <li className="text-sm text-muted">No saved places.</li> : null}
          {places.map((place) => (
            <li key={place.id} className="rounded-xl border border-line px-3 py-2 text-sm">
              <strong>{place.label ?? "Place"}</strong>
              <div>{place.address_text}</div>
              <div className="text-muted">{place.latitude == null ? "No coordinates stored" : `${place.latitude}, ${place.longitude}`}</div>
            </li>
          ))}
        </ul>
      </Card>
      <Card>
        <h2 className="font-serif text-2xl">Delete account</h2>
        <p className="mt-2 text-sm text-muted">This submits a request. It does not immediately erase the account.</p>
        <div className="mt-3">
          <Button onClick={requestDeletion}>Request deletion</Button>
        </div>
      </Card>
    </div>
  );
}
