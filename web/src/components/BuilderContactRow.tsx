import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { BuilderContact, Project } from "../api/types";

type Props = {
  contact: BuilderContact | null | undefined;
  projectId: number;
  /** Shows "change" (open leads and cases that are not dropped). */
  onChange?: () => void;
};

/**
 * The "Builder contact" row of a Details list (spec 2026-11-24 §4.3): label ·
 * name, a call link and email. With no contact it shows "Not set" only when
 * the project has contacts to pick from, so NA and old projects show nothing.
 */
export function BuilderContactRow({ contact, projectId, onChange }: Props) {
  const [pickable, setPickable] = useState(false);

  useEffect(() => {
    if (contact) return;
    let live = true;
    api.request<Project>("GET", `/api/config/projects/${projectId}`)
      .then((p) => live && setPickable(!p.is_system && (p.contacts?.length ?? 0) > 0))
      .catch(() => live && setPickable(false));
    return () => { live = false; };
  }, [contact, projectId]);

  if (!contact && !(pickable && onChange)) return null;
  const change = onChange && (
    <button type="button" aria-label="Change builder contact" className="ml-2 text-sm font-normal text-blue-700" onClick={onChange}>change</button>
  );
  return (
    <>
      <dt className="text-slate-500">Builder contact</dt>
      <dd className="min-w-0 font-semibold [overflow-wrap:anywhere]">
        {contact ? (
          <>
            {[contact.label, contact.name].filter(Boolean).join(" · ")}
            {contact.removed && <span className="ml-1 font-normal text-amber-700">(removed)</span>}
            {change}
            <span className="block font-normal">
              <a href={`tel:+91${contact.phone}`} className="text-blue-700">{contact.phone}</a>
              {contact.email && <> · <a href={`mailto:${contact.email}`} className="text-blue-700">{contact.email}</a></>}
            </span>
          </>
        ) : (
          <><span className="font-normal text-slate-500">Not set</span>{change}</>
        )}
      </dd>
    </>
  );
}
