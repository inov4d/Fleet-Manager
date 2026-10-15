"use client";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { useSiteStore } from "@/stores/site";
import { useApi } from "@/hooks/use-api";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardBody, Spinner } from "@/components/ui/card";
import { VehicleForm, valuesToVehicle, vehicleToValues } from "@/components/vehicles/vehicle-form";

export default function NewVehiclePage() {
  const router = useRouter();
  const siteId = useSiteStore((s) => s.siteId);
  const models = useApi(() => api.vehicleModels.list(), []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <div className="max-w-3xl">
      <PageHeader title="Nouveau véhicule" subtitle={`Site : ${siteId}`} />
      <Card>
        <CardBody>
          {models.data ? (
            <VehicleForm
              isNew
              initial={vehicleToValues(null)}
              models={models.data}
              submitLabel="Créer le véhicule"
              loading={loading}
              error={error}
              onSubmit={async (values) => {
                setLoading(true);
                setError(null);
                try {
                  const created = await api.vehicles.create(valuesToVehicle(values, siteId));
                  toast.success("Véhicule créé", created.name);
                  router.push(`/vehicles/${encodeURIComponent(created.id)}`);
                } catch (e) {
                  setError(errorMessage(e));
                } finally {
                  setLoading(false);
                }
              }}
            />
          ) : (
            <Spinner />
          )}
        </CardBody>
      </Card>
    </div>
  );
}
