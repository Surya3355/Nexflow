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

/** The unit number's label for a project type (spec 2026-11-24 unit label). */
export function unitLabel(t: ProjectType | null | undefined): string {
  switch (t) {
    case "high_rise":
    case "apartments":
      return "Flat no.";
    case "villas":
      return "Villa no.";
    case "plots":
      return "Plot no.";
    default:
      return "Unit no.";
  }
}

/** Shown in every unit number box; block letters are allowed. */
export const unitPlaceholder = "e.g. A-1012";
