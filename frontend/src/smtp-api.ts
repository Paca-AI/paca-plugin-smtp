import { type PluginApiClient } from "@paca-ai/plugin-sdk-react";
import { PLUGIN_ID } from "./constants";

export interface OptionalEventOption {
  topic: string;
  label: string;
  description: string;
}

/** The events an admin can enable and a user can individually opt out of.
 * user.created (the welcome/invite email) and user.password_reset (the
 * "your password was reset" email) are mandatory once SMTP is configured
 * and aren't listed here — see backend/plugin.go's optionalTopics.
 * All default to enabled (see the migration's enabled_events default). */
export const OPTIONAL_EVENT_OPTIONS: OptionalEventOption[] = [
  {
    topic: "notification.assigned",
    label: "Assigned to a task",
    description: "When someone assigns a task to you.",
  },
  {
    topic: "notification.mentioned",
    label: "Mentioned in a comment",
    description: "When someone @mentions you in a task comment.",
  },
  {
    topic: "notification.doc_mentioned",
    label: "Mentioned in a document",
    description: "When someone @mentions you in a document.",
  },
  {
    topic: "notification.task_description_mentioned",
    label: "Mentioned in a task description",
    description: "When someone @mentions you in a task's description.",
  },
];

export interface AdminConfig {
  host: string;
  port: number;
  username: string;
  from_address: string;
  from_name: string;
  use_tls: boolean;
  enabled_events: string[];
  has_password: boolean;
}

export interface UpdateAdminConfigInput {
  host: string;
  port: number;
  username: string;
  /** Omit or send "" to keep the currently-saved password unchanged. */
  password?: string;
  from_address: string;
  from_name: string;
  use_tls: boolean;
  enabled_events: string[];
}

export interface Preferences {
  available_events: string[];
  disabled_events: string[];
}

export function getAdminConfig(api: PluginApiClient): Promise<AdminConfig> {
  return api.pluginGet<AdminConfig>(PLUGIN_ID, "/admin/config");
}

export function updateAdminConfig(
  api: PluginApiClient,
  input: UpdateAdminConfigInput,
): Promise<AdminConfig> {
  return api.pluginPatch<AdminConfig>(PLUGIN_ID, "/admin/config", input);
}

export function sendTestEmail(
  api: PluginApiClient,
  to: string,
): Promise<void> {
  return api.pluginPost<void>(PLUGIN_ID, "/admin/test-email", { to });
}

export function getMyPreferences(api: PluginApiClient): Promise<Preferences> {
  return api.pluginGet<Preferences>(PLUGIN_ID, "/me/preferences");
}

export function updateMyPreferences(
  api: PluginApiClient,
  disabledEvents: string[],
): Promise<void> {
  return api.pluginPatch<void>(PLUGIN_ID, "/me/preferences", {
    disabled_events: disabledEvents,
  });
}

/**
 * Extracts a human-readable message from a PluginApiClient error.
 * The React SDK throws: `[PluginApiClient] METHOD URL → STATUS: BODY`
 * where BODY is `{"error":"..."}` from the plugin backend.
 */
export function getPluginErrorMessage(err: unknown): string | null {
  if (!(err instanceof Error)) return null;
  const arrowIdx = err.message.lastIndexOf("→ ");
  if (arrowIdx === -1) return null;
  const rest = err.message.slice(arrowIdx + 2);
  const colonIdx = rest.indexOf(": ");
  if (colonIdx === -1) return null;
  const maybeJson = rest.slice(colonIdx + 2);
  try {
    const body = JSON.parse(maybeJson) as { error?: string };
    return body.error ?? null;
  } catch {
    return null;
  }
}
