"use client";

import { useEffect, useState } from "react";

export type AuthorAvatar = { initials: string; avatar_url: string };

// The author avatar is a machine-level identity, so fetch /api/meta once and
// share the result across every avatar on the page.
let cache: AuthorAvatar | null = null;
let inflight: Promise<AuthorAvatar> | null = null;

async function loadAuthor(): Promise<AuthorAvatar> {
  if (cache) return cache;
  if (!inflight) {
    inflight = fetch("/api/meta")
      .then((r) => (r.ok ? r.json() : null))
      .then((body) => {
        const author = (body?.author as AuthorAvatar | undefined) || {
          initials: "U",
          avatar_url: "",
        };
        cache = author;
        return author;
      })
      .catch(() => ({ initials: "U", avatar_url: "" }))
      .finally(() => {
        inflight = null;
      });
  }
  return inflight;
}

export function useAuthorAvatar(): AuthorAvatar {
  const [author, setAuthor] = useState<AuthorAvatar>(
    cache || { initials: "U", avatar_url: "" },
  );
  useEffect(() => {
    let active = true;
    void loadAuthor().then((value) => {
      if (active) setAuthor(value);
    });
    return () => {
      active = false;
    };
  }, []);
  return author;
}
