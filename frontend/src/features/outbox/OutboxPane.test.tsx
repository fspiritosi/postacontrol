import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() =>
    Promise.resolve({
      unifiedInbox: false,
      disableNotifications: false, disableSpellcheck: false,
      threadedList: false,
      readingPane: "",
      background: "background-1.png",
    }),
  ),
  BackgroundImage: vi.fn(() => Promise.resolve("data:image/png;base64,AAAA")),
}));

import { OutboxPane } from "./OutboxPane";

describe("OutboxPane", () => {
  it("con fondo elegido lo pinta detrás del texto", async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <OutboxPane />
      </QueryClientProvider>,
    );
    const el = await screen.findByTestId("reader-backdrop");
    expect(el.style.backgroundImage).toContain("data:image/png;base64,AAAA");
    expect(el).toHaveTextContent("Abrí un correo de la Bandeja de salida para revisarlo, o reintentá el envío.");
  });
});
