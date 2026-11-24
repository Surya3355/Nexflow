import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProjectPicker } from "./ProjectPicker";
import type { Project } from "../api/types";

const projects: Project[] = [
  { id: 1, name: "NA", is_system: true, open_cases: 3, total_cases: 5, type: null, address: "", active_contacts: 0 },
  { id: 2, name: "Prestige Lakeside", is_system: false, open_cases: 1, total_cases: 1, type: null, address: "", active_contacts: 0 },
  { id: 3, name: "Zenith", is_system: false, open_cases: 0, total_cases: 0, type: null, address: "", active_contacts: 0 },
];

function options() {
  return screen.getAllByRole("option").map((o) => o.textContent);
}

describe("ProjectPicker", () => {
  it("filters by typed letters and keeps NA on top", async () => {
    render(<ProjectPicker projects={projects} value={null} onChange={() => {}} aria-label="Project" />);
    await userEvent.type(screen.getByRole("combobox", { name: "Project" }), "pre");
    expect(options()).toEqual(["NA", "Prestige Lakeside"]);
  });

  it("still shows NA when nothing matches", async () => {
    render(<ProjectPicker projects={projects} value={null} onChange={() => {}} aria-label="Project" />);
    await userEvent.type(screen.getByRole("combobox", { name: "Project" }), "zzz");
    expect(options()).toEqual(["NA"]);
  });

  it("calls onChange only when an item is picked", async () => {
    const onChange = vi.fn();
    render(<ProjectPicker projects={projects} value={null} onChange={onChange} aria-label="Project" />);
    const box = screen.getByRole("combobox", { name: "Project" });
    await userEvent.type(box, "Zen");
    expect(onChange).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("option", { name: "Zenith" }));
    expect(onChange).toHaveBeenCalledWith(3);
  });

  it("picks with the keyboard", async () => {
    const onChange = vi.fn();
    render(<ProjectPicker projects={projects} value={null} onChange={onChange} aria-label="Project" />);
    await userEvent.type(screen.getByRole("combobox", { name: "Project" }), "pre{ArrowDown}{Enter}");
    expect(onChange).toHaveBeenCalledWith(2);
  });

  it("shows the chosen project's name", () => {
    render(<ProjectPicker projects={projects} value={2} onChange={() => {}} aria-label="Project" />);
    expect(screen.getByRole("combobox", { name: "Project" })).toHaveValue("Prestige Lakeside");
  });

  it("offers All projects first when allowAll is set", async () => {
    const onChange = vi.fn();
    render(<ProjectPicker projects={projects} value={2} onChange={onChange} allowAll aria-label="Project" />);
    const box = screen.getByRole("combobox", { name: "Project" });
    await userEvent.clear(box);
    expect(options()).toEqual(["All projects", "NA", "Prestige Lakeside", "Zenith"]);
    await userEvent.click(screen.getByRole("option", { name: "All projects" }));
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it("starts a new search when a project is already chosen, and restores it on leaving", async () => {
    render(<><ProjectPicker projects={projects} value={2} onChange={() => {}} aria-label="Project" /><button>elsewhere</button></>);
    const box = screen.getByRole("combobox", { name: "Project" });
    await userEvent.type(box, "Zen");
    expect(options()).toEqual(["NA", "Zenith"]);
    await userEvent.click(screen.getByRole("button", { name: "elsewhere" }));
    expect(box).toHaveValue("Prestige Lakeside");
  });
});
