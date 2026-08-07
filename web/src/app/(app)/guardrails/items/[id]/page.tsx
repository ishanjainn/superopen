import Client from "./client";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  // Unwrap Next.js 15+ async dynamic APIs before rendering the client tree
  // (avoids sync-dynamic-apis warnings from prop enumeration).
  await params;
  await searchParams;
  return <Client />;
}
