"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { AuthFrame } from "@/components/stage/auth-frame";
import { Button, ErrorText, Field, controlClass, fieldErrors } from "@/components/ui";
import { ApiError, api, type Session } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function RegisterPage() {
  const router = useRouter();
  const { setSession } = useAuth();
  const [form, setForm] = useState({
    display_name: "",
    email: "",
    password: "",
    preferred_language: "en",
  });
  const [error, setError] = useState<ApiError | null>(null);
  const [pending, setPending] = useState(false);
  const fields = fieldErrors(error);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      const session = await api<Session>("/api/v1/auth/register", { method: "POST", body: form });
      await setSession(session);
      router.push("/home");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught : new ApiError(0, "network", "Could not create the account."));
    } finally {
      setPending(false);
    }
  }

  return (
    <AuthFrame
      title="Join the street."
      lede="One account for reporting and tracking local problems. Email verification is not turned on yet — you can sign in right after you register."
      footer={
        <>
          Already registered? <Link href="/login">Sign in</Link>
        </>
      }
    >
      <form className="flex flex-col gap-4" onSubmit={submit}>
        {error && Object.keys(fields).length === 0 ? <ErrorText error={error.message} /> : null}
        <Field label="Name" error={fields.display_name}>
          <input className={controlClass} required value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} />
        </Field>
        <Field label="Email" error={fields.email}>
          <input className={controlClass} type="email" autoComplete="email" required value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
        </Field>
        <Field label="Password" error={fields.password}>
          <input
            className={controlClass}
            type="password"
            autoComplete="new-password"
            required
            minLength={10}
            value={form.password}
            onChange={(event) => setForm({ ...form, password: event.target.value })}
          />
        </Field>
        <Field label="Language">
          <select className={controlClass} value={form.preferred_language} onChange={(event) => setForm({ ...form, preferred_language: event.target.value })}>
            <option value="en">English</option>
            <option value="hi">हिन्दी</option>
          </select>
        </Field>
        <Button type="submit" disabled={pending} className="w-full">
          {pending ? "Creating account…" : "Create account"}
        </Button>
      </form>
    </AuthFrame>
  );
}
