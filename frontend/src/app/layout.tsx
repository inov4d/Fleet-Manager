import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Fleet Manager — Supervision AGV",
  description: "Système de gestion de flotte d’AGV omnidirectionnels 20 t / 40 t",
};

// Applies the persisted theme before first paint to avoid a flash.
const themeScript = `(function(){try{var t=localStorage.getItem('fm-theme');if(t==='light'||t==='dark'){document.documentElement.setAttribute('data-theme',t);}}catch(e){}})();`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="fr" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full">{children}</body>
    </html>
  );
}
