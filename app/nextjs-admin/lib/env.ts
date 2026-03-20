export function getApiConfig() {
  const baseUrl = process.env.MDHUB_API_BASE_URL;
  const token = process.env.MDHUB_API_TOKEN;

  if (!baseUrl) {
    return { error: "Missing MDHUB_API_BASE_URL in environment." };
  }

  return {
    baseUrl,
    token,
  };
}
