import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { TopBar } from "./TopBar";

afterEach(() => vi.restoreAllMocks());

const admin = { id: 1, username: "owner", full_name: "Owner", role: "admin" as const };

function renderBar(onSignOut = () => {}) {
  render(<MemoryRouter><TopBar user={admin} onSignOut={onSignOut} /></MemoryRouter>);
}

describe("TopBar phone menu", () => {
  it("has a ☰ button that opens every page for the role, plus Sign out", async () => {
    renderBar();
    const button = screen.getByRole("button", { name: "Menu" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    const list = screen.getByRole("navigation", { name: "Phone menu" });
    expect(within(list).getAllByRole("link").map((a) => a.textContent)).toEqual(["Home", "Projects", "Officers", "Reports", "Admin"]);
    expect(within(list).getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });

  it("closes when a page is chosen, and on Escape", async () => {
    renderBar();
    await userEvent.click(screen.getByRole("button", { name: "Menu" }));
    await userEvent.click(within(screen.getByRole("navigation", { name: "Phone menu" })).getByRole("link", { name: "Reports" }));
    expect(screen.queryByRole("navigation", { name: "Phone menu" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Menu" }));
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("navigation", { name: "Phone menu" })).not.toBeInTheDocument();
  });

  it("keeps the desktop links, hidden on phones only", () => {
    renderBar();
    const main = screen.getByRole("navigation", { name: "Main" });
    expect(main.className).toMatch(/\bhidden\b/);
    expect(main.className).toMatch(/\bsm:flex\b/);
    expect(screen.getByRole("button", { name: "Menu" }).className).toMatch(/\bsm:hidden\b/);
  });
});
