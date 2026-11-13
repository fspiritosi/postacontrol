import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SnoozeFor: vi.fn(),
  Unsnooze: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SnoozeBanner } from "./SnoozeBanner";

function setup() {
  const onReturned = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SnoozeBanner messageId={4} onReturned={onReturned} />
    </QueryClientProvider>,
  );
  return { onReturned, user: userEvent.setup() };
}

describe("SnoozeBanner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("dice cuándo vuelve y lo devuelve ahora", async () => {
    const until = new Date();
    until.setDate(until.getDate() + 1);
    until.setHours(8, 0, 0, 0);
    vi.mocked(svc.SnoozeFor).mockResolvedValue({ id: 9, accountId: 1, messageId: "<a@x>", returnFolder: "INBOX", until: until.getTime() / 1000 });
    const { onReturned, user } = setup();
    expect(await screen.findByText(/vuelve mañana a las 8:00/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Devolver ahora" }));
    await waitFor(() => expect(svc.Unsnooze).toHaveBeenCalledWith(9));
    expect(onReturned).toHaveBeenCalled();
  });

  it("si no está pospuesto no muestra nada", async () => {
    vi.mocked(svc.SnoozeFor).mockResolvedValue(null);
    setup();
    await waitFor(() => expect(svc.SnoozeFor).toHaveBeenCalledWith(4));
    expect(screen.queryByRole("button", { name: "Devolver ahora" })).toBeNull();
  });
});
