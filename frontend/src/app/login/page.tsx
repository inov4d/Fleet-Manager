"use client";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { Truck } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { ErrorBox } from "@/components/ui/card";

export default function LoginPage() {
  const router = useRouter();
  const setAuth = useAuthStore((s) => s.setAuth);
  const token = useAuthStore((s) => s.token);
  const hydrated = useAuthStore((s) => s.hydrated);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (hydrated && token) router.replace("/");
  }, [hydrated, token, router]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const res = await api.auth.login(username.trim(), password);
      setAuth(res.token, res.user, res.expiresAt);
      const siteStore = useSiteStore.getState();
      if (res.user.siteIds.length > 0 && !res.user.siteIds.includes(siteStore.siteId)) {
        siteStore.setSiteId(res.user.siteIds[0]);
      }
      router.replace("/");
    } catch (err) {
      setError(errorMessage(err) === "unauthorized" ? "Identifiants invalides" : errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="min-h-screen flex items-center justify-center bg-bg p-4">
      <form onSubmit={submit} className="w-full max-w-sm rounded-lg border border-border bg-surface p-6 flex flex-col gap-4">
        <div className="flex items-center gap-3">
          <div className="size-10 rounded-md bg-accent-soft text-accent flex items-center justify-center">
            <Truck className="size-5" aria-hidden />
          </div>
          <div>
            <h1 className="text-base font-semibold leading-tight">Fleet Manager</h1>
            <p className="text-xs text-fg-muted">Supervision de flotte AGV</p>
          </div>
        </div>
        {error && <ErrorBox message={error} />}
        <Field label="Nom d’utilisateur" htmlFor="username" required>
          <Input
            id="username"
            autoComplete="username"
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
          />
        </Field>
        <Field label="Mot de passe" htmlFor="password" required>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </Field>
        <Button type="submit" variant="primary" loading={loading} className="w-full">
          Se connecter
        </Button>
      </form>
    </main>
  );
}
