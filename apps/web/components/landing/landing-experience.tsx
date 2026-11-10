"use client";

import Link from "next/link";

const steps = [
  { n: "01", t: "Create an account", d: "Name, email, and a password. One door for everyone who reports." },
  { n: "02", t: "Sign in", d: "Your session comes back with you. The dashboard is the next screen." },
  { n: "03", t: "Work the case", d: "Report, follow what is public nearby, and confirm when the outcome is true." },
];

const modes = [
  { t: "Digital", d: "Guidance, a prepared request, or an approved submission, each with its own delivery state." },
  { t: "On the ground", d: "A verified person who can reach the place: field staff, a technician, or a contractor." },
  { t: "Both", d: "The request goes out and someone still comes to look. Progress stays on one timeline." },
];

export function LandingExperience() {
  return (
    <div className="landing">
      <header className="topbar">
        <Link href="/" className="wordmark">
          LokaSolve
        </Link>
        <nav className="top-actions">
          <Link href="/login" className="btn btn-line">
            Sign in
          </Link>
          <Link href="/register" className="btn btn-gold">
            Create account
          </Link>
        </nav>
      </header>

      <section className="hero">
        <div>
          <p className="kicker">Local problems, carried to a verified outcome</p>
          <h1>Something is wrong on your street. We take it from there.</h1>
          <p className="lede">
            Tell LokaSolve what you see. We find who can act — an office, a utility, a technician, or a community responder — and we stay with the case until the result is real.
          </p>
          <div className="hero-actions">
            <Link href="/register" className="btn btn-gold btn-lg">
              Start your account
            </Link>
            <Link href="/login" className="btn btn-line btn-lg">
              I already signed up
            </Link>
          </div>
          <dl className="hero-metrics">
            <div>
              <dt>Any problem</dt>
              <dd>Not locked to one category</dd>
            </div>
            <div>
              <dt>One timeline</dt>
              <dd>Digital, field, or both</dd>
            </div>
            <div>
              <dt>Honest status</dt>
              <dd>No invented “accepted”</dd>
            </div>
          </dl>
        </div>
        <figure className="photo-frame">
          <img src="/photos/street.jpg" alt="A wide city avenue in daylight, with crossings, traffic, and buildings on both sides." />
          <figcaption>A real street. Reports start from a place like this, described by the person who is there.</figcaption>
        </figure>
      </section>

      <section className="split">
        <img src="/photos/road.jpg" alt="An aerial photograph of a road interchange surrounded by neighborhoods." />
        <div className="section-copy">
          <p className="kicker">What can be reported</p>
          <h2>Roads, water, power, and anything else local.</h2>
          <p>
            Categories stay open. A dark street, water on the road, an open drain, a failed light, a civic service. You describe it. LokaSolve does not decide the department for you in advance, and it never invents a coordinate from an address alone.
          </p>
        </div>
      </section>

      <section className="steps">
        <div className="section-copy">
          <p className="kicker">The path you take</p>
          <h2>Sign up. Sign in. Then the street has a desk.</h2>
        </div>
        <div className="step-grid">
          {steps.map((step) => (
            <article key={step.n} className={step.n === "02" ? "step-live" : undefined}>
              <span>{step.n}</span>
              <h3>{step.t}</h3>
              <p>{step.d}</p>
            </article>
          ))}
        </div>
        <div className="mode-row">
          {modes.map((mode) => (
            <article key={mode.t}>
              <span>Resolution</span>
              <h3>{mode.t}</h3>
              <p>{mode.d}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="close">
        <div className="close-inner">
          <p className="kicker">Open the account</p>
          <h2>The street can wait. The account should not.</h2>
          <Link href="/register" className="btn btn-gold btn-lg">
            Create account
          </Link>
        </div>
      </section>
    </div>
  );
}
