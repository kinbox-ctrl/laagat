import type { Metadata } from "next";
import "./globals.css";
import { SessionProvider } from "@/lib/session";
import { TopBar } from "@/components/TopBar";

export const metadata: Metadata = {
  title: "BuildCost Intelligence",
  description: "Independent construction cost assessment: contractor estimate audits and drawing-based cost estimates.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <SessionProvider>
          <TopBar />
          <main>{children}</main>
        </SessionProvider>
      </body>
    </html>
  );
}
