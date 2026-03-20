import "./globals.css";
import { NavBar } from "@/components/NavBar";

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>
        <main className="container">
          <h1>mdhub Admin</h1>
          <NavBar />
          {children}
        </main>
      </body>
    </html>
  );
}
