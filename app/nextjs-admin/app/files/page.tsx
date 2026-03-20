import Link from "next/link";
import { fetchFiles } from "@/lib/api";

export default async function FilesPage({
  searchParams,
}: {
  searchParams?: Promise<{ type?: string; syncStatus?: string; deleted?: string }>;
}) {
  const sp = searchParams ? await searchParams : undefined;
  const type = sp?.type ?? "";
  const syncStatus = sp?.syncStatus ?? "";
  const deleted = sp?.deleted === "1";
  const result = await fetchFiles({ type, syncStatus });

  return (
    <section className="card">
      <h2>/files</h2>
      <form style={{ display: "flex", gap: 12, marginBottom: 12 }}>
        <input name="type" defaultValue={type} placeholder="type: markdown/image" />
        <input name="syncStatus" defaultValue={syncStatus} placeholder="syncStatus: pending/synced/failed" />
        <button type="submit">筛选</button>
      </form>

      {deleted ? <p className="muted">已删除（软删除）</p> : null}

      {result.error ? (
        <p className="muted">{result.error}</p>
      ) : result.items.length === 0 ? (
        <p className="muted">暂无数据</p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th align="left">文件名</th>
              <th align="left">类型</th>
              <th align="left">同步状态</th>
              <th align="left">操作</th>
            </tr>
          </thead>
          <tbody>
            {result.items.map((item) => (
              <tr key={item.uuid}>
                <td>{item.fileName}</td>
                <td>{item.type}</td>
                <td>{item.syncStatus || "pending"}</td>
                <td>
                  <Link href={`/files/${item.uuid}`}>查看</Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
