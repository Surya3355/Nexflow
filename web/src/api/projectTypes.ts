import type { ProjectType } from "./types";

/** The fixed project types, in the owner's order (spec 2026-11-24). */
export const PROJECT_TYPES: { value: ProjectType; label: string }[] = [
  { value: "high_rise", label: "High-rise apartments" },
  { value: "apartments", label: "Apartments" },
  { value: "plots", label: "Plots" },
  { value: "villas", label: "Villas" },
];

export function projectTypeLabel(t: ProjectType | null | undefined): string {
  return PROJECT_TYPES.find((p) => p.value === t)?.label ?? "";
}
