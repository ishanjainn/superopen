import Client from "./client";

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  // Unwrap Next.js 15+ async searchParams before rendering the client tree
  // (avoids sync-dynamic-apis warnings from prop enumeration).
  await searchParams;
  return <Client />;
}
