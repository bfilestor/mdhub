import Link from "next/link";

export default function HomePage() {
  return (
    <section className="card">
      <h2>Welcome</h2>
      <p className="muted">管理后台骨架已就绪，先从文件管理开始。</p>
      <Link href="/files">进入 /files</Link>
    </section>
  );
}
