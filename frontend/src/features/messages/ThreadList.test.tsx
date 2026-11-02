import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListThreads: vi.fn(() => Promise.resolve([])),
}));

import type {
  MessageSummary,
  ThreadSummary,
} from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { NO_MARKS, type Marks } from "@/lib/selection";
import { ThreadList } from "./ThreadList";

function summary(
  id: number,
  over: Partial<MessageSummary> = {},
): MessageSummary {
  return {
    id,
    accountId: 7,
    folderId: 3,
    uid: id,
    from: { name: `Remitente ${id}`, email: "x@y.z" },
    subject: `Asunto ${id}`,
    snippet: "",
    date: "2026-10-30T10:00:00Z",
    seen: true,
    flagged: false,
    answered: false,
    hasAttachments: false,
    ...over,
  };
}

const threads: ThreadSummary[] = [
  {
    threadRoot: "<a@x>",
    latest: summary(1, { seen: false, flagged: true }),
    count: 3,
    unread: 1,
    participants: ["Ana", "Beto"],
  },
  {
    threadRoot: "<b@x>",
    latest: summary(2),
    count: 1,
    unread: 0,
    participants: ["Carla"],
  },
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
          marked={NO_MARKS}
          onMark={vi.fn()}
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

describe("ThreadList no leídos", () => {
  it("una conversación con no leídos tiene fondo y remitente resaltados", async () => {
    vi.mocked(svc.ListThreads).mockResolvedValue([
      { ...threads[0], unread: 2 },
      threads[1],
    ]);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ThreadList
          folderId={3}
          unreadOnly={false}
          syncing={false}
          selected={null}
          onSelect={vi.fn()}
          marked={NO_MARKS}
          onMark={vi.fn()}
        />
      </QueryClientProvider>,
    );
    const unread = await screen.findByRole("button", { name: /Asunto 1/ });
    const read = screen.getByRole("button", { name: /Asunto 2/ });
    expect(unread.closest("li")!.getAttribute("data-unread")).toBe("true");
    expect(unread.className).toContain("bg-unread");
    expect(screen.getByText("Ana, Beto").className).toContain(
      "text-unread-foreground",
    );
    expect(read.closest("li")!.hasAttribute("data-unread")).toBe(false);
    expect(read.className).not.toContain("bg-unread");
    expect(screen.getByText("Carla").className).not.toContain(
      "text-unread-foreground",
    );
  });
});

describe("ThreadList selección múltiple", () => {
  // Hace de MailShell: abrir un hilo suelta lo marcado.
  function Stateful() {
    const [marked, setMarked] = useState<Marks<string>>(NO_MARKS);
    const [open, setOpen] = useState<string | null>(null);
    return (
      <>
        <p data-testid="open">{open ?? ""}</p>
        <ThreadList
          folderId={3}
          unreadOnly={false}
          syncing={false}
          selected={open}
          onSelect={(r) => {
            setOpen(r);
            setMarked(NO_MARKS);
          }}
          marked={marked}
          onMark={setMarked}
        />
        <p data-testid="count">{marked.keys.length}</p>
      </>
    );
  }

  it("Ctrl + clic en dos hilos los marca; un clic común abre uno y suelta", async () => {
    vi.mocked(svc.ListThreads).mockResolvedValue(threads);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <Stateful />
      </QueryClientProvider>,
    );
    const first = await screen.findByRole("button", { name: /Asunto 1/ });
    const second = screen.getByRole("button", { name: /Asunto 2/ });
    fireEvent.click(first, { ctrlKey: true });
    // Con uno solo marcado y nada abierto se abre ese.
    expect(screen.getByTestId("open").textContent).toBe("<a@x>");
    fireEvent.click(second, { ctrlKey: true });
    expect(screen.getByTestId("count").textContent).toBe("2");
    expect(second.closest("li")!.className).toContain("bg-primary/10");
    fireEvent.keyDown(second, { key: "Escape" });
    expect(screen.getByTestId("count").textContent).toBe("0");
    fireEvent.click(second, { ctrlKey: true });
    fireEvent.click(first);
    expect(screen.getByTestId("count").textContent).toBe("0");
    expect(screen.getByTestId("open").textContent).toBe("<a@x>");
  });

  it("la casilla de un hilo lo marca sin abrirlo", async () => {
    vi.mocked(svc.ListThreads).mockResolvedValue(threads);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <Stateful />
      </QueryClientProvider>,
    );
    const first = await screen.findByRole("button", { name: /Asunto 1/ });
    fireEvent.click(first);
    const box = screen
      .getByRole("button", { name: /Asunto 2/ })
      .closest("li")!
      .querySelector("input")!;
    fireEvent.click(box);
    expect(screen.getByTestId("count").textContent).toBe("2");
    expect(screen.getByTestId("open").textContent).toBe("<a@x>");
  });
});
