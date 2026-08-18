import {
  PluginApiClient,
  PluginQueryClientProvider,
} from "@paca-ai/plugin-sdk-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Mail, Send, Sparkles } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  type AdminConfig,
  getAdminConfig,
  getPluginErrorMessage,
  OPTIONAL_EVENT_OPTIONS,
  sendTestEmail,
  type UpdateAdminConfigInput,
  updateAdminConfig,
} from "./smtp-api";

// ── Primitives (mirrors the styling primitives every other plugin frontend
// in this workspace defines locally rather than depending on a host UI kit) ──

function cn(...classes: (string | undefined | null | false)[]): string {
  return classes.filter(Boolean).join(" ");
}

function Btn({
  className,
  variant = "default",
  size = "default",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "outline";
  size?: "default" | "sm";
}) {
  const variants: Record<string, string> = {
    default: "bg-primary text-primary-foreground hover:bg-primary/90",
    outline: "border border-input bg-background hover:bg-accent",
  };
  const sizes: Record<string, string> = {
    default: "h-10 px-4 py-2",
    sm: "h-8 px-3 text-xs",
  };
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:opacity-50 disabled:pointer-events-none",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
}

function Inp({
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground/80">{hint}</p> : null}
    </div>
  );
}

function Skeleton({ className }: { className?: string }) {
  return (
    <div className={cn("animate-pulse rounded-md bg-muted", className)} />
  );
}

/** Card wrapper every section below uses — header (icon + title + subtitle)
 * over a divider, then padded content. Matches the dashboard/time-tracking
 * plugins' page-shell conventions (icon+h1 header, bordered/muted sections)
 * applied at the section level instead of the whole page. */
function Section({
  icon: Icon,
  title,
  description,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-border/60 bg-card/30">
      <div className="flex items-start gap-2.5 border-b border-border/40 bg-muted/20 px-4 py-3">
        <Icon className="mt-0.5 size-4 shrink-0 text-primary" />
        <div>
          <h2 className="text-sm font-semibold">{title}</h2>
          {description ? (
            <p className="mt-0.5 text-xs text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
      </div>
      <div className="space-y-4 p-4">{children}</div>
    </section>
  );
}

// ── Form state ─────────────────────────────────────────────────────────────

interface FormState {
  host: string;
  port: string;
  username: string;
  password: string;
  fromAddress: string;
  fromName: string;
  useTLS: boolean;
  enabledEvents: string[];
}

function formStateFromConfig(cfg: AdminConfig): FormState {
  return {
    host: cfg.host,
    port: String(cfg.port),
    username: cfg.username,
    password: "",
    fromAddress: cfg.from_address,
    fromName: cfg.from_name,
    useTLS: cfg.use_tls,
    enabledEvents: cfg.enabled_events,
  };
}

/** True when the connection/sender fields have no unsaved changes relative
 * to the last-saved config — password is excluded from the "saved" side of
 * the comparison since it's write-only and always reads back blank; a
 * non-empty password always counts as a pending change. Notification events
 * are deliberately NOT part of this check: they save themselves immediately
 * on toggle (see toggleEvent/eventsMutation), so they're never "pending"
 * from this button's point of view. */
function formIsClean(form: FormState, cfg: AdminConfig): boolean {
  if (form.password !== "") return false;
  if (form.host !== cfg.host) return false;
  if (form.port !== String(cfg.port)) return false;
  if (form.username !== cfg.username) return false;
  if (form.fromAddress !== cfg.from_address) return false;
  if (form.fromName !== cfg.from_name) return false;
  if (form.useTLS !== cfg.use_tls) return false;
  return true;
}

const CONFIG_QUERY_KEY = ["smtp", "admin-config"];

function AdminSmtpSettingsInner({ api }: { api: PluginApiClient }) {
  const queryClient = useQueryClient();
  const { data: config, isLoading } = useQuery({
    queryKey: CONFIG_QUERY_KEY,
    queryFn: () => getAdminConfig(api),
  });

  const [form, setForm] = useState<FormState | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [testTo, setTestTo] = useState("");
  const [testResult, setTestResult] = useState<string | null>(null);

  useEffect(() => {
    if (config && !form) setForm(formStateFromConfig(config));
  }, [config, form]);

  const saveMutation = useMutation({
    mutationFn: (input: UpdateAdminConfigInput) => updateAdminConfig(api, input),
    onSuccess: (updated) => {
      queryClient.setQueryData(CONFIG_QUERY_KEY, updated);
      setForm(formStateFromConfig(updated));
      setSaveError(null);
    },
    onError: (err: unknown) => {
      setSaveError(
        getPluginErrorMessage(err) ?? "Failed to save settings. Please try again.",
      );
    },
  });

  // Saves just the event toggles, independent of saveMutation, so checking a
  // box takes effect immediately — no need to also press "Save settings",
  // which now only applies to the connection/sender fields. PATCHes with
  // config's last *saved* connection/sender values (not form's, which may
  // hold edits the admin hasn't submitted yet) so this can never clobber an
  // in-progress edit there.
  const eventsMutation = useMutation({
    mutationFn: (enabledEvents: string[]) => {
      if (!config) throw new Error("not configured");
      return updateAdminConfig(api, {
        host: config.host,
        port: config.port,
        username: config.username,
        from_address: config.from_address,
        from_name: config.from_name,
        use_tls: config.use_tls,
        enabled_events: enabledEvents,
      });
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(CONFIG_QUERY_KEY, updated);
      setEventsError(null);
    },
  });

  const testMutation = useMutation({
    mutationFn: () => sendTestEmail(api, testTo.trim()),
    onSuccess: () => setTestResult("Test email sent — check the inbox."),
    onError: (err: unknown) =>
      setTestResult(getPluginErrorMessage(err) ?? "Failed to send test email."),
  });

  // SMTP hasn't been configured and saved yet (fresh install with no
  // host/from_address on record) — there's nothing valid to PATCH onto, so
  // event toggles just update local state until the admin does an initial
  // "Save settings" with the connection fields filled in.
  const canAutoSaveEvents =
    !!config && config.host.trim() !== "" && config.from_address.trim() !== "";

  const toggleEvent = (topic: string) => {
    if (!form) return;
    const previous = form.enabledEvents;
    const next = previous.includes(topic)
      ? previous.filter((t) => t !== topic)
      : [...previous, topic];
    setForm({ ...form, enabledEvents: next }); // optimistic

    if (!canAutoSaveEvents) return;

    eventsMutation.mutate(next, {
      onError: (err: unknown) => {
        // Roll back the optimistic toggle since the save failed.
        setForm((f) => f && { ...f, enabledEvents: previous });
        setEventsError(
          getPluginErrorMessage(err) ?? "Failed to save. Please try again.",
        );
      },
    });
  };

  if (isLoading || !form) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-6 p-6">
        <div>
          <Skeleton className="h-6 w-40" />
          <Skeleton className="mt-2 h-4 w-72" />
        </div>
        <Skeleton className="h-56 rounded-xl" />
        <Skeleton className="h-40 rounded-xl" />
      </div>
    );
  }

  const portNumber = Number.parseInt(form.port, 10);
  const isValid =
    form.host.trim() !== "" &&
    Number.isFinite(portNumber) &&
    portNumber > 0 &&
    form.fromAddress.trim() !== "";
  const isDirty = !config || !formIsClean(form, config);
  const canSave = isValid && isDirty;

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-6 p-6">
      {/* Page header */}
      <div>
        <div className="flex items-center gap-2">
          <Mail className="size-5 text-primary" />
          <h1 className="text-xl font-semibold">Email (SMTP)</h1>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure an SMTP server to send account-invite and notification
          emails. The new-account welcome email is sent automatically once
          this is configured; each event below is optional and users can
          also opt out for themselves in their own account settings.
        </p>
      </div>

      {/* SMTP server */}
      <Section
        icon={Mail}
        title="SMTP server"
        description="Where and how outbound mail is sent."
      >
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="sm:col-span-2">
            <Field label="SMTP host">
              <Inp
                placeholder="smtp.example.com"
                value={form.host}
                onChange={(e) =>
                  setForm((f) => f && { ...f, host: e.target.value })
                }
              />
            </Field>
          </div>
          <Field label="Port">
            <Inp
              type="number"
              placeholder="587"
              value={form.port}
              onChange={(e) =>
                setForm((f) => f && { ...f, port: e.target.value })
              }
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Username">
            <Inp
              placeholder="smtp username"
              value={form.username}
              onChange={(e) =>
                setForm((f) => f && { ...f, username: e.target.value })
              }
              autoComplete="off"
            />
          </Field>
          <Field
            label={
              form.password === "" && config?.has_password
                ? "Password (leave blank to keep the current one)"
                : "Password"
            }
          >
            <Inp
              type="password"
              placeholder={config?.has_password ? "••••••••" : ""}
              value={form.password}
              onChange={(e) =>
                setForm((f) => f && { ...f, password: e.target.value })
              }
              autoComplete="new-password"
            />
          </Field>
        </div>

        <label className="flex items-center gap-2 text-sm cursor-pointer select-none">
          <input
            type="checkbox"
            className="size-3.5 rounded border-input"
            checked={form.useTLS}
            onChange={(e) =>
              setForm((f) => f && { ...f, useTLS: e.target.checked })
            }
          />
          Use TLS on connect (port 465 style)
        </label>
        <p className="-mt-2 text-xs text-muted-foreground/80">
          Leave unchecked for STARTTLS (port 587/25 style) — it's still used
          automatically when the server offers it.
        </p>

        <div className="h-px bg-border/40" />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="From address">
            <Inp
              type="email"
              placeholder="noreply@example.com"
              value={form.fromAddress}
              onChange={(e) =>
                setForm((f) => f && { ...f, fromAddress: e.target.value })
              }
            />
          </Field>
          <Field label="From name (optional)">
            <Inp
              placeholder="Paca"
              value={form.fromName}
              onChange={(e) =>
                setForm((f) => f && { ...f, fromName: e.target.value })
              }
            />
          </Field>
        </div>

        {saveError ? (
          <p className="text-xs text-destructive">{saveError}</p>
        ) : isDirty ? (
          <p className="text-xs text-muted-foreground">Unsaved changes</p>
        ) : null}

        <div className="flex justify-end">
          <Btn
            size="sm"
            disabled={!canSave || saveMutation.isPending}
            onClick={() =>
              saveMutation.mutate({
                host: form.host.trim(),
                port: portNumber,
                username: form.username.trim(),
                password: form.password || undefined,
                from_address: form.fromAddress.trim(),
                from_name: form.fromName.trim(),
                use_tls: form.useTLS,
                enabled_events: form.enabledEvents,
              })
            }
          >
            {saveMutation.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : null}
            Save settings
          </Btn>
        </div>
      </Section>

      {/* Notification events — each toggle saves itself immediately, no
          button here. */}
      <Section
        icon={Sparkles}
        title="Notification events"
        description="Which events send an email, in addition to the mandatory welcome and password-reset emails. Changes save immediately."
      >
        {!canAutoSaveEvents ? (
          <p className="text-xs text-muted-foreground">
            Save your SMTP connection settings above first — until then,
            changes here are kept locally and saved along with that.
          </p>
        ) : null}
        <div className="space-y-2">
          {OPTIONAL_EVENT_OPTIONS.map((opt) => {
            const checked = form.enabledEvents.includes(opt.topic);
            return (
              <label
                key={opt.topic}
                className={cn(
                  "flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors select-none",
                  checked
                    ? "border-primary/40 bg-primary/5"
                    : "border-border/40 hover:bg-accent/40",
                )}
              >
                <input
                  type="checkbox"
                  className="mt-0.5 size-3.5 shrink-0 rounded border-input"
                  checked={checked}
                  onChange={() => toggleEvent(opt.topic)}
                />
                <div className="min-w-0">
                  <p className="text-sm font-medium">{opt.label}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {opt.description}
                  </p>
                </div>
              </label>
            );
          })}
        </div>
        {eventsMutation.isPending ? (
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Loader2 className="size-3 animate-spin" />
            Saving…
          </p>
        ) : null}
        {eventsError ? (
          <p className="text-xs text-destructive">{eventsError}</p>
        ) : null}
      </Section>

      {/* Test email */}
      <Section
        icon={Send}
        title="Send a test email"
        description="Verify delivery and see how the branded template renders, using the settings saved above."
      >
        <div className="flex gap-2">
          <Inp
            type="email"
            placeholder="you@example.com"
            value={testTo}
            onChange={(e) => setTestTo(e.target.value)}
          />
          <Btn
            variant="outline"
            size="sm"
            className="shrink-0"
            disabled={!testTo.trim() || testMutation.isPending}
            onClick={() => {
              setTestResult(null);
              testMutation.mutate();
            }}
          >
            {testMutation.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <Send className="size-3.5" />
            )}
            Send test
          </Btn>
        </div>
        {testResult ? (
          <p className="text-xs text-muted-foreground">{testResult}</p>
        ) : null}
      </Section>
    </div>
  );
}

export default function AdminSmtpSettingsPage() {
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
      <AdminSmtpSettingsInner api={api} />
    </PluginQueryClientProvider>
  );
}
