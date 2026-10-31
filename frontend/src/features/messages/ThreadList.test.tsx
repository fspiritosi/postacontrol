import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListThreads: vi.fn(() => Promise.resolve([])),
}));

import type { MessageSummary, ThreadSummary } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { ThreadList } from "./ThreadList";

function summary(id: number, over: Partial<MessageSummary> = {}): MessageSummary {
  return {
    id, accountId: 7, folderId: 3, uid: id,
    from: { name: `Remitente ${id}`, email: "x@y.z" },
    subject: `Asunto ${id}`, snippet: "", date: "2026-10-30T10:00:00Z",
    seen: true, flagged: false, answered: false, hasAttachments: false,
    ...over,
  };
}

const threads: ThreadSummary[] = [
  {
    threadRoot: "<a@x>", latest: summary(1, { seen: false, flagged: true }),
    count: 3, unread: 1, participants: ["Ana", "Beto"],
  },
  { threadRoot: "<b@x>", latest: summary(2), count: 1, unread: 0, participants: ["Carla"] },
];

describe("ThreadList", () => {
  it("un hilo muestra participantes, contador y negrita si hay no leídos", async () => {
    vi.mocked(svc.ListThreads).mockResolvedValue(threads);
    const selectThread = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ThreadList
          folderId={3}
          unreadOnly={false}
          syncing={false}
          color="#f00"
          selected={null}
          onSelect={selectThread}
        />
      </QueryClientProvider>,
    );
    const ana = await screen.findByText("Ana, Beto");
    expect(ana.className).toContain("font-semibold");
    const carla = screen.getByText("Carla");
    expect(carla.className).not.toContain("font-semibold");

    const first = screen.getByRole("button", { name: /Asunto 1/ });
    expect(first.textContent).toContain("3");
    expect(first.querySelector('[aria-label="Destacado"]')).not.toBeNull();
    const second = screen.getByRole("button", { name: /Asunto 2/ });
    expect(second.querySelector('[data-testid="thread-count"]')).toBeNull();
    expect(second.querySelector('[aria-label="Destacado"]')).toBeNull();

    fireEvent.click(first);
    expect(selectThread).toHaveBeenCalledWith("<a@x>");
  });
});
