import type { Metadata } from "next";
import { IBM_Plex_Mono, Sora, Source_Sans_3 } from "next/font/google";
import { Shell } from "@/components/layout/shell";
import "./globals.css";

const display = Sora({
  variable: "--font-display-face",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const body = Source_Sans_3({
  variable: "--font-body-face",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const mono = IBM_Plex_Mono({
  variable: "--font-mono-face",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  title: "NodeHarvest · 高质量节点观测台",
  description: "自动采集、智能测速、AI 站点可达筛选的代理节点控制台",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="zh-CN"
      className={`${display.variable} ${body.variable} ${mono.variable} h-full antialiased`}
    >
      <body className="min-h-full bg-background text-foreground">
        <Shell>{children}</Shell>
      </body>
    </html>
  );
}
