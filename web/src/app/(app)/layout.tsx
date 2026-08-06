import PlaygroundShell from "@/components/shell/playground-shell";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return <PlaygroundShell>{children}</PlaygroundShell>;
}
