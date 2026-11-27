import { useEffect, useId, useState } from "react";
import type { Project } from "../api/types";
import { input } from "./ui";

type Props = {
  projects: Project[];
  value: number | null;
  onChange: (id: number | null) => void;
  /** Adds "All projects" (value null) before NA, for filters. */
  allowAll?: boolean;
  placeholder?: string;
  "aria-label"?: string;
  className?: string;
};

type Option = { id: number | null; name: string };

/**
 * A searchable project box (spec 2026-11-21 §4.3). Typing shows the projects
 * whose name contains the letters; NA is always first. Only an item from the
 * list can be chosen, so typing alone never changes the value.
 */
export function ProjectPicker({ projects, value, onChange, allowAll, placeholder, className, ...rest }: Props) {
  const listId = useId();
  const selectedName = value === null ? (allowAll ? "All projects" : "") : projects.find((p) => p.id === value)?.name ?? "";
  const [text, setText] = useState(selectedName);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);

  useEffect(() => setText(selectedName), [selectedName]);

  const q = text.trim().toLowerCase();
  const typing = q !== "" && q !== selectedName.toLowerCase();
  const system = projects.filter((p) => p.is_system);
  const others = projects.filter((p) => !p.is_system && (!typing || p.name.toLowerCase().includes(q)));
  const options: Option[] = [
    ...(allowAll ? [{ id: null, name: "All projects" }] : []),
    ...system,
    ...others,
  ];

  function pick(o: Option) {
    onChange(o.id);
    setText(o.name);
    setOpen(false);
  }

  return (
    <div className="relative">
      <input
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-label={rest["aria-label"]}
        className={className ?? input}
        placeholder={selectedName || placeholder || "Type to find a project"}
        value={text}
        // Focus clears the box so typing starts a new search; leaving without
        // picking puts the chosen name back.
        onFocus={() => { setText(""); setOpen(true); setActive(0); }}
        onBlur={() => { setOpen(false); setText(selectedName); }}
        onChange={(e) => { setText(e.target.value); setOpen(true); setActive(0); }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") { e.preventDefault(); setOpen(true); setActive((a) => Math.min(a + 1, options.length - 1)); }
          else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
          else if (e.key === "Enter" && open && options[active]) { e.preventDefault(); pick(options[active]); }
          else if (e.key === "Escape") { setOpen(false); setText(selectedName); }
        }}
      />
      {open && (
        <ul id={listId} role="listbox" className="absolute z-30 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-slate-200 bg-white py-1 text-sm shadow-lg">
          {options.map((o, i) => (
            <li
              key={o.id ?? "all"}
              role="option"
              aria-selected={o.id === value}
              className={`cursor-pointer px-3 py-2 ${i === active ? "bg-blue-50 text-blue-800" : "text-slate-800"}`}
              onMouseDown={(e) => { e.preventDefault(); pick(o); }}
              onMouseEnter={() => setActive(i)}
            >
              {o.name}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
