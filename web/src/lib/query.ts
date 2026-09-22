import { QueryClient } from "@tanstack/vue-query";
import { ApiError } from "./http";
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      refetchOnWindowFocus: false,
      retry: (count, error) =>
        count < 1 &&
        error instanceof ApiError &&
        (error.status === 0 || error.status >= 500),
    },
    mutations: { retry: false, networkMode: "always" },
  },
});
