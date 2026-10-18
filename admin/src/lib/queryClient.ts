import { QueryClient } from "@tanstack/react-query";
import { isApiError } from "./api/client";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 20_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      retry: (count, e) => {
        if (isApiError(e) && e.status >= 400 && e.status < 500 && e.status !== 429) return false;
        return count < 2;
      },
    },
    mutations: { retry: false },
  },
});
