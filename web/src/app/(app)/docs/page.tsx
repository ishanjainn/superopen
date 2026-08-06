import { redirect } from "next/navigation";

/** Legacy /docs and /context → /knowledge */
export default function DocsRedirect() {
  redirect("/knowledge");
}
