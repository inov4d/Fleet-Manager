"use client";
import { Suspense, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { can, type Permission } from "@/lib/auth";
import { useAuthStore } from "@/stores/auth";
import { PageHeader, Spinner } from "@/components/ui/card";
import { Tabs } from "@/components/ui/tabs";
import { BatterySettingsForm, DispatcherSettingsForm, TrafficSettingsForm } from "@/components/settings/settings-forms";
import { SitesAdmin } from "@/components/settings/sites-admin";
import { UsersAdmin } from "@/components/settings/users-admin";
import { ApiKeysAdmin } from "@/components/settings/api-keys-admin";
import { AuditLog, SystemInfoCard } from "@/components/settings/audit-system";

type TabKey = "dispatcher" | "battery" | "traffic" | "sites" | "users" | "apikeys" | "audit" | "system";

interface TabDef {
  key: TabKey;
  label: string;
  /** Permission required to *see* the tab (undefined = everyone). */
  view?: Permission;
  /** Permission required to edit the content. */
  edit?: Permission;
}

const TABS: TabDef[] = [
  { key: "dispatcher", label: "Répartiteur", edit: "settings.dispatcher" },
  { key: "battery", label: "Batterie", edit: "settings.battery" },
  { key: "traffic", label: "Trafic", edit: "settings.traffic" },
  { key: "sites", label: "Sites", view: "sites.manage" },
  { key: "users", label: "Utilisateurs", view: "users.manage" },
  { key: "apikeys", label: "Clés API", view: "apikeys.manage" },
  { key: "audit", label: "Audit", view: "audit.read" },
  { key: "system", label: "Système" },
];

export default function SettingsPage() {
  return (
    <Suspense fallback={<Spinner />}>
      <SettingsTabs />
    </Suspense>
  );
}

function SettingsTabs() {
  const user = useAuthStore((s) => s.user);
  const router = useRouter();
  const search = useSearchParams();
  const visible = useMemo(() => TABS.filter((t) => !t.view || can(user, t.view)), [user]);
  const initial = (search.get("tab") as TabKey | null) ?? visible[0]?.key ?? "system";
  const [tab, setTab] = useState<TabKey>(initial);
  useEffect(() => {
    if (!visible.some((t) => t.key === tab)) setTab(visible[0]?.key ?? "system");
  }, [visible, tab]);

  const change = (k: TabKey) => {
    setTab(k);
    router.replace(`/settings?tab=${k}`);
  };
  const def = TABS.find((t) => t.key === tab);
  const canEdit = def?.edit ? can(user, def.edit) : false;

  return (
    <div className="flex flex-col gap-3">
      <PageHeader title="Paramètres" subtitle={def?.edit && !canEdit ? "Lecture seule pour votre rôle" : undefined} />
      <Tabs<TabKey> tabs={visible.map((t) => ({ key: t.key, label: t.label }))} value={tab} onChange={change} />
      {tab === "dispatcher" && <DispatcherSettingsForm canEdit={canEdit} />}
      {tab === "battery" && <BatterySettingsForm canEdit={canEdit} />}
      {tab === "traffic" && <TrafficSettingsForm canEdit={canEdit} />}
      {tab === "sites" && <SitesAdmin />}
      {tab === "users" && <UsersAdmin />}
      {tab === "apikeys" && <ApiKeysAdmin />}
      {tab === "audit" && <AuditLog />}
      {tab === "system" && <SystemInfoCard />}
    </div>
  );
}
