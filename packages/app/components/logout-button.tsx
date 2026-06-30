"use client";

import { useRouter } from "next/navigation";
import { LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";

export function LogoutButton() {
  const router = useRouter();

  async function handleLogout() {
    await fetch("/api/logout", { method: "POST" });
    router.push("/login");
    router.refresh();
  }

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={handleLogout}
      aria-label="Log out"
      title="Log out"
      className="ml-1 text-muted-foreground"
    >
      <LogOut className="size-4" />
    </Button>
  );
}
