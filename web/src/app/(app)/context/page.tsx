import { redirect } from "next/navigation";

/** Legacy /context → /knowledge */
export default function ContextRedirect() {
  redirect("/knowledge");
}
