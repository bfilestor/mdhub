import Link from "next/link";

const links = [
  { href: "/files", label: "Files" },
  { href: "/upload", label: "Upload" },
  { href: "/sync", label: "Sync" },
];

export function NavBar() {
  return (
    <nav className="nav">
      {links.map((item) => (
        <Link key={item.href} href={item.href}>
          {item.label}
        </Link>
      ))}
    </nav>
  );
}
