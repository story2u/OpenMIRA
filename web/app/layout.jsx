import "./globals.css";

export const metadata = {
  title: "IM Slim",
  description: "Message and SOP console",
};

export default function RootLayout({ children }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
