import { redirect } from "next/navigation";
import { deleteFile, fetchFileDetail } from "@/lib/api";

export default async function FileDetailPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;

  async function handleDelete() {
    "use server";
    const result = await deleteFile(uuid);
    if (result.error) {
      redirect(`/files/${uuid}?error=${encodeURIComponent(result.error)}`);
    }
    redirect("/files?deleted=1");
  }

  const result = await fetchFileDetail(uuid);

  return (
    <section className="card">
      <h2>/files/{uuid}</h2>
      {result.error ? (
        <p className="muted">{result.error}</p>
      ) : (
        <>
          <p>
            <strong>文件名：</strong>
            {result.item?.fileName}
          </p>
          <p>
            <strong>类型：</strong>
            {result.item?.contentType}
          </p>
          <p>
            <strong>同步状态：</strong>
            {result.item?.syncStatus || "pending"}
          </p>
          {result.item?.contentPreview ? (
            <pre style={{ whiteSpace: "pre-wrap", background: "#f3f4f6", padding: 12, borderRadius: 8 }}>
              {result.item.contentPreview}
            </pre>
          ) : null}

          <form action={handleDelete}>
            <button type="submit">删除（软删除）</button>
          </form>
        </>
      )}
    </section>
  );
}
