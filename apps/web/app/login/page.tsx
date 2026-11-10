"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { AuthFrame } from "@/components/stage/auth-frame";
import { Button, ErrorText, Field, controlClass } from "@/components/ui";
import { ApiError, api, type Session } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const { setSession } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const session = await api<Session>("/api/v1/auth/login", { method: "POST", body: { email, password } });
      await setSession(session);
      router.push("/home");
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "Could not sign in.");
    } finally {
      setPending(false);
    }
  }

  return (
    <AuthFrame
      title="Welcome back."
      lede="Sign in and the dashboard opens on the cases and places that belong to you."
      footer={
        <>
          New here? <Link href="/register">Create an account</Link>
        </>
      }
    >
      <form className="flex flex-col gap-4" onSubmit={submit}>
        {error ? <ErrorText error={error} /> : null}
        <Field label="Email">
          <input className={controlClass} type="email" autoComplete="username" required value={email} onChange={(event) => setEmail(event.target.value)} />
        </Field>
        <Field label="Password">
          <input className={controlClass} type="password" autoComplete="current-password" required value={password} onChange={(event) => setPassword(event.target.value)} />
        </Field>
        <Button type="submit" disabled={pending} className="w-full">
          {pending ? "Signing in…" : "Sign in"}
        </Button>
      </form>
    </AuthFrame>
  );
}
