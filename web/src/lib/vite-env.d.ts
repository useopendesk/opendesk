/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL?: string;
  readonly VITE_GATEWAY_URL?: string;
  readonly VITE_POSTHOG_KEY?: string;
  readonly VITE_POSTHOG_HOST?: string;
  readonly VITE_TURNSTILE_SITE_KEY?: string;
  readonly VITE_SELF_HOSTED?: string;
  readonly VITE_MAILBOX_OAUTH?: string;
  readonly VITE_MOCK?: string;
}
