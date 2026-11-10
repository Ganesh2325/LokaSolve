"use client";

import type { ApiError } from "@/lib/api";

export function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-semibold">{label}</span>
      {children}
      {error ? <span className="mt-1 block text-sm text-rose">{error}</span> : null}
    </label>
  );
}

export const controlClass =
  "min-h-12 w-full rounded-lg border border-black bg-card px-3 text-base text-ink";

export function Button({
  children,
  type = "button",
  disabled,
  onClick,
  className = "",
}: {
  children: React.ReactNode;
  type?: "button" | "submit";
  disabled?: boolean;
  onClick?: () => void;
  className?: string;
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className={`inline-flex min-h-12 items-center justify-center rounded-lg border border-black bg-[#c4a15a] px-5 font-semibold text-black hover:bg-black hover:text-[#c4a15a] disabled:opacity-60 ${className}`}
    >
      {children}
    </button>
  );
}

export function SecondaryButton({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) {
  return (
    <button type="button" onClick={onClick} className="inline-flex min-h-11 items-center justify-center rounded-lg border border-black bg-card px-4 font-semibold text-black">
      {children}
    </button>
  );
}

export function Card({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <section className={`rounded-lg border border-black bg-card p-4 ${className}`.trim()}>{children}</section>;
}

export function Notice({ children }: { children: React.ReactNode }) {
  return <p className="rounded-lg border border-black border-l-4 border-l-[#c4a15a] bg-card px-3 py-2 text-sm">{children}</p>;
}

export function ErrorText({ error }: { error: string }) {
  return (
    <p role="alert" className="rounded-lg border border-black bg-card px-3 py-2 text-sm font-semibold">
      {error}
    </p>
  );
}

export function fieldErrors(error: ApiError | null) {
  const map: Record<string, string> = {};
  error?.details.forEach((detail) => {
    map[detail.field] = detail.message;
  });
  return map;
}
