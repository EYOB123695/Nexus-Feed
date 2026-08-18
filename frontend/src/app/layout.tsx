import type { Metadata } from 'next';
import { Inter, JetBrains_Mono } from 'next/font/google';
import QueryProvider from '@/lib/query-provider';
import './globals.css';

const inter = Inter({
  variable: '--font-inter',
  subsets: ['latin'],
});

const jetbrainsMono = JetBrains_Mono({
  variable: '--font-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'Nexus-Feed | High-Frequency Market Terminal',
  description: 'Cross-exchange consolidated order book and real-time arbitrage detection engine',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body className={`${inter.variable} ${jetbrainsMono.variable} antialiased bg-[#080B11] text-gray-100 min-h-screen`}>
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
