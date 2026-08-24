import Client from "./client";

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  await searchParams;
  return <Client />;
}
