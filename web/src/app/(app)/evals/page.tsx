"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

/** Legacy /evals → /evaluations */
export default function EvalsRedirectPage() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/evaluations");
  }, [router]);
  return (
    <p className="p-6 text-sm text-neutral-500">Redirecting to Evaluations…</p>
  );
}
