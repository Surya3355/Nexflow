/** "DOVR officer", or "Bank officer" for the standard list's single Officer team. */
export function officerLabel(teamName: string): string {
  return teamName === "Officer" ? "Bank officer" : `${teamName} officer`;
}

/** How an officer is shown in a dropdown: "Name · mobile", or the email when there is no mobile. */
export function officerOption(o: { name: string; phone: string; email?: string }): string {
  return `${o.name} · ${o.phone || o.email || ""}`;
}

/** The client-side check that matches the server: a mobile or an email. */
export function contactError(phone: string, email: string): string | null {
  return phone.trim() || email.trim() ? null : "Enter a mobile number or an email.";
}
