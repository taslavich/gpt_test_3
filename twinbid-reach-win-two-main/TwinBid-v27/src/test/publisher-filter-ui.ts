import { fireEvent, screen, waitFor } from "@testing-library/react";
export async function selectPublisherFilter(label: string, values: string[]) {
  fireEvent.click(screen.getByLabelText(label));
  for (const value of values) fireEvent.click(await screen.findByRole("checkbox", { name: value }));
  const search = screen.queryByPlaceholderText("Поиск…");
  if (search) fireEvent.keyDown(search, { key: "Escape" });
  await waitFor(() => { if (screen.queryByPlaceholderText("Поиск…")) throw new Error("Filter is still open"); });
}
