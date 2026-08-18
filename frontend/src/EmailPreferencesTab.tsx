import {
  PluginApiClient,
  PluginQueryClientProvider,
} from "@paca-ai/plugin-sdk-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { useMemo, useState } from "react";
import {
  getMyPreferences,
  getPluginErrorMessage,
  OPTIONAL_EVENT_OPTIONS,
  updateMyPreferences,
} from "./smtp-api";

// ── Card primitives — reproduce apps/web/src/components/ui/card.tsx's exact
// classes (Card/CardHeader/CardContent) and separator.tsx's, rather than
// importing them, since a plugin frontend is a separate build with no
// access to the host app's component source. This keeps this section
// visually identical to the Profile and Change Password cards above it on
// the same page. ────────────────────────────────────────────────────────

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-4 overflow-hidden rounded-xl bg-card py-4 text-sm text-card-foreground ring-1 ring-foreground/10">
      {children}
    </div>
  );
}

function CardHeader({ children }: { children: React.ReactNode }) {
  return <div className="px-4">{children}</div>;
}

function CardContent({ children }: { children: React.ReactNode }) {
  return <div className="px-4 pt-5">{children}</div>;
}

function Separator() {
  return <div className="h-px w-full shrink-0 bg-border" />;
}

const PREFERENCES_QUERY_KEY = ["smtp", "my-preferences"];

function EmailPreferencesInner({ api }: { api: PluginApiClient }) {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: PREFERENCES_QUERY_KEY,
    queryFn: () => getMyPreferences(api),
  });
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (disabledEvents: string[]) =>
      updateMyPreferences(api, disabledEvents),
    onSuccess: (_void, disabledEvents) => {
      queryClient.setQueryData(PREFERENCES_QUERY_KEY, (old: typeof data) =>
        old ? { ...old, disabled_events: disabledEvents } : old,
      );
      setError(null);
    },
    onError: (err: unknown) => {
      setError(
        getPluginErrorMessage(err) ?? "Failed to save. Please try again.",
      );
    },
  });

  const toggle = (topic: string) => {
    if (!data) return;
    const next = data.disabled_events.includes(topic)
      ? data.disabled_events.filter((t) => t !== topic)
      : [...data.disabled_events, topic];
    mutation.mutate(next);
  };

  if (isLoading || !data) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="size-8 animate-pulse rounded-lg bg-muted" />
            <div className="space-y-1.5">
              <div className="h-4 w-40 animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-56 animate-pulse rounded bg-muted" />
            </div>
          </div>
        </CardHeader>
      </Card>
    );
  }

  const availableOptions = OPTIONAL_EVENT_OPTIONS.filter((opt) =>
    data.available_events.includes(opt.topic),
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 items-center justify-center rounded-lg bg-muted">
            <Mail className="size-4 text-muted-foreground" />
          </div>
          <div>
            <div className="text-base leading-snug font-medium">
              Email notifications
            </div>
            <div className="mt-0.5 text-sm text-muted-foreground">
              Choose which events email you. Your admin controls which are
              available.
            </div>
          </div>
        </div>
      </CardHeader>

      <Separator />

      <CardContent>
        {availableOptions.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Your admin hasn't enabled any optional email notifications yet.
          </p>
        ) : (
          <div className="flex max-w-sm flex-col gap-4">
            {availableOptions.map((opt) => (
              <label
                key={opt.topic}
                className="flex items-start gap-2.5 cursor-pointer select-none"
              >
                <input
                  type="checkbox"
                  className="mt-0.5 size-3.5 shrink-0 rounded border-input"
                  checked={!data.disabled_events.includes(opt.topic)}
                  disabled={mutation.isPending}
                  onChange={() => toggle(opt.topic)}
                />
                <div className="min-w-0">
                  <p className="text-sm">{opt.label}</p>
                  <p className="text-xs text-muted-foreground">
                    {opt.description}
                  </p>
                </div>
              </label>
            ))}
          </div>
        )}
        {error ? (
          <p className="mt-4 text-sm text-destructive">{error}</p>
        ) : null}
      </CardContent>
    </Card>
  );
}

export default function EmailPreferencesTab() {
  const api = useMemo(
    () =>
      new PluginApiClient({
        baseUrl: `${window.location.origin}/api/v1`,
        fetch: (url, init) =>
          window.fetch(url, { ...init, credentials: "include" }),
      }),
    [],
  );

  return (
    <PluginQueryClientProvider>
      <EmailPreferencesInner api={api} />
    </PluginQueryClientProvider>
  );
}
