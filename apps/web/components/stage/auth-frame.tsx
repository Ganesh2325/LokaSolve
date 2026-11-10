import Link from "next/link";

export function AuthFrame({
  title,
  lede,
  children,
  footer,
}: {
  title: string;
  lede: string;
  children: React.ReactNode;
  footer: React.ReactNode;
}) {
  return (
    <main id="main" className="auth-screen">
      <section className="auth-card">
        <Link href="/" className="auth-brand">
          LokaSolve
        </Link>
        <h1>{title}</h1>
        <p className="auth-lede">{lede}</p>
        {children}
        <div className="auth-footer">{footer}</div>
      </section>
    </main>
  );
}
