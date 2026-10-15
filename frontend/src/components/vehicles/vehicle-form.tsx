"use client";
import { useState, type FormEvent } from "react";
import type { Vehicle, VehicleModel, VehicleProtocol } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { ErrorBox } from "@/components/ui/card";

export interface VehicleFormValues {
  id: string;
  name: string;
  modelId: string;
  manufacturer: string;
  serialNumber: string;
  protocol: VehicleProtocol;
  enabled: boolean;
  safetyMarginM: number;
  localizationUncertaintyM: number;
  footprintEnabled: boolean;
  footprintLengthM: number;
  footprintWidthM: number;
  tags: string;
}

export function vehicleToValues(v?: Vehicle | null): VehicleFormValues {
  return {
    id: v?.id ?? "",
    name: v?.name ?? "",
    modelId: v?.modelId ?? "AGV20T",
    manufacturer: v?.manufacturer ?? "",
    serialNumber: v?.serialNumber ?? "",
    protocol: v?.protocol ?? "vda5050",
    enabled: v?.enabled ?? true,
    safetyMarginM: v?.safetyMarginM ?? 0.3,
    localizationUncertaintyM: v?.localizationUncertaintyM ?? 0.1,
    footprintEnabled: !!v?.footprint,
    footprintLengthM: v?.footprint?.lengthM ?? 6,
    footprintWidthM: v?.footprint?.widthM ?? 2.4,
    tags: v?.tags?.join(", ") ?? "",
  };
}

export function valuesToVehicle(values: VehicleFormValues, siteId: string): Partial<Vehicle> {
  return {
    ...(values.id ? { id: values.id } : {}),
    siteId,
    name: values.name.trim(),
    modelId: values.modelId,
    manufacturer: values.manufacturer.trim(),
    serialNumber: values.serialNumber.trim(),
    protocol: values.protocol,
    enabled: values.enabled,
    safetyMarginM: values.safetyMarginM,
    localizationUncertaintyM: values.localizationUncertaintyM,
    footprint: values.footprintEnabled ? { lengthM: values.footprintLengthM, widthM: values.footprintWidthM } : null,
    tags: values.tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean),
  };
}

export function VehicleForm({
  initial,
  models,
  onSubmit,
  submitLabel,
  loading,
  error,
  isNew,
}: {
  initial: VehicleFormValues;
  models: VehicleModel[];
  onSubmit: (v: VehicleFormValues) => void;
  submitLabel: string;
  loading?: boolean;
  error?: string | null;
  isNew?: boolean;
}) {
  const [v, setV] = useState<VehicleFormValues>(initial);
  const set = <K extends keyof VehicleFormValues>(k: K, val: VehicleFormValues[K]) => setV((p) => ({ ...p, [k]: val }));
  const num = (s: string) => (s === "" ? 0 : Number(s));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit(v);
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      {error && <ErrorBox message={error} />}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {isNew && (
          <Field label="Identifiant" htmlFor="v-id" hint="Optionnel, généré par le serveur si vide.">
            <Input id="v-id" value={v.id} onChange={(e) => set("id", e.target.value)} />
          </Field>
        )}
        <Field label="Nom" htmlFor="v-name" required>
          <Input id="v-name" value={v.name} onChange={(e) => set("name", e.target.value)} required />
        </Field>
        <Field label="Modèle" htmlFor="v-model" required>
          <Select
            id="v-model"
            value={v.modelId}
            onChange={(e) => set("modelId", e.target.value)}
            options={models.map((m) => ({ value: m.id, label: `${m.name} (${m.lengthM}×${m.widthM} m, ${m.maxPayloadKg / 1000} t)` }))}
          />
        </Field>
        <Field label="Protocole" htmlFor="v-protocol">
          <Select
            id="v-protocol"
            value={v.protocol}
            onChange={(e) => set("protocol", e.target.value as VehicleProtocol)}
            options={[
              { value: "vda5050", label: "VDA 5050" },
              { value: "simulated", label: "Simulé" },
            ]}
          />
        </Field>
        <Field label="Fabricant" htmlFor="v-manu">
          <Input id="v-manu" value={v.manufacturer} onChange={(e) => set("manufacturer", e.target.value)} />
        </Field>
        <Field label="Numéro de série" htmlFor="v-serial">
          <Input id="v-serial" value={v.serialNumber} onChange={(e) => set("serialNumber", e.target.value)} />
        </Field>
        <Field label="Marge de sécurité (m)" htmlFor="v-margin">
          <Input id="v-margin" type="number" step="0.05" min={0} value={v.safetyMarginM} onChange={(e) => set("safetyMarginM", num(e.target.value))} />
        </Field>
        <Field label="Incertitude de localisation (m)" htmlFor="v-loc">
          <Input id="v-loc" type="number" step="0.05" min={0} value={v.localizationUncertaintyM} onChange={(e) => set("localizationUncertaintyM", num(e.target.value))} />
        </Field>
        <Field label="Tags" htmlFor="v-tags" hint="Séparés par des virgules.">
          <Input id="v-tags" value={v.tags} onChange={(e) => set("tags", e.target.value)} />
        </Field>
        <div className="flex items-end pb-1">
          <Checkbox label="Véhicule activé" checked={v.enabled} onChange={(e) => set("enabled", e.target.checked)} />
        </div>
      </div>
      <fieldset className="rounded-md border border-border p-3">
        <legend className="text-xs text-fg-muted px-1">Empreinte (remplace la géométrie du modèle)</legend>
        <Checkbox label="Définir une empreinte spécifique" checked={v.footprintEnabled} onChange={(e) => set("footprintEnabled", e.target.checked)} />
        {v.footprintEnabled && (
          <div className="grid grid-cols-2 gap-3 mt-2">
            <Field label="Longueur (m)" htmlFor="v-fl">
              <Input id="v-fl" type="number" step="0.1" min={0.1} value={v.footprintLengthM} onChange={(e) => set("footprintLengthM", num(e.target.value))} />
            </Field>
            <Field label="Largeur (m)" htmlFor="v-fw">
              <Input id="v-fw" type="number" step="0.1" min={0.1} value={v.footprintWidthM} onChange={(e) => set("footprintWidthM", num(e.target.value))} />
            </Field>
          </div>
        )}
      </fieldset>
      <div className="flex justify-end">
        <Button type="submit" variant="primary" loading={loading}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}
