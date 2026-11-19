import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../bindings/postacontrol/app/mailservice", () => ({
  InviteForMessage: vi.fn(() => Promise.resolve({ found: false })),
}));

import * as svc from "../../bindings/postacontrol/app/mailservice";
import { useMessageInvite } from "./queries";

describe("useMessageInvite", () => {
  it("no vuelve a pedir la invitación al remontar dentro de 60 s", async () => {
    const client = new QueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const first = renderHook(() => useMessageInvite(5), { wrapper });
    await waitFor(() => expect(first.result.current.isSuccess).toBe(true));
    first.unmount();
    renderHook(() => useMessageInvite(5), { wrapper });
    expect(svc.InviteForMessage).toHaveBeenCalledTimes(1);
  });
});
