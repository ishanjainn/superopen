import type { Metadata } from "next";
import Script from "next/script";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ThemeProvider } from "@/components/shell/theme-provider";
import "./globals.css";

export const metadata: Metadata = {
  title: "Superopen",
  description: "Open source Agent Harness Engineering",
};

/** Apply theme class before paint. Default dark when no preference stored. */
const themeBootScript = `(function(){try{var t=localStorage.getItem('so-theme')||'dark';var d=t==='dark'||(t==='system'&&window.matchMedia('(prefers-color-scheme: dark)').matches);var r=document.documentElement;r.classList.toggle('dark',d);r.style.colorScheme=d?'dark':'light';}catch(e){}})();`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="bg-stone-50 text-stone-900 antialiased">
        <Script id="so-theme-boot" strategy="beforeInteractive">
          {themeBootScript}
        </Script>
        <ThemeProvider>
          <TooltipProvider>{children}</TooltipProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
