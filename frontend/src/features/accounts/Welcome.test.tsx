import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  DetectSettings: vi.fn(),
  AddAccount: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { Welcome } from "./Welcome";

describe("Welcome", () => {
  it("muestra el logo de PostaControl", () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <Welcome />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("img", { name: "PostaControl" })).toHaveAttribute("src", "/logo.png");
  });
});
