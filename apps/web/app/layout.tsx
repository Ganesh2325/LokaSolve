import type { Metadata, Viewport } from "next";
import { Figtree, Fraunces } from "next/font/google";
import { AuthProvider } from "@/lib/auth";
import "./globals.css";

const figtree = Figtree({ subsets: ["latin"], variable: "--font-figtree" });
const fraunces = Fraunces({ subsets: ["latin"], variable: "--font-fraunces" });

export const metadata: Metadata = {
  title: "LokaSolve",
  description: "Tell LokaSolve what is wrong around you. Coordinate the next action and follow verified outcomes.",
  applicationName: "LokaSolve",
  manifest: "/manifest.webmanifest",
};

export const viewport: Viewport = {
  themeColor: "#f3eee3",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${figtree.variable} ${fraunces.variable} font-sans antialiased`}>
        <a className="skip-link" href="#main">Skip to content</a>
        <AuthProvider>{children}</AuthProvider>
      </body>
    </html>
  );
}
