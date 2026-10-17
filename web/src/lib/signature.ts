import type { SignatureScheme, SignatureStatus } from './types';

export interface SchemeInfo {
  name: string;
  /** What the scheme checks, shown under the picker. */
  hint: string;
  /** Signs a timestamp, so the tolerance applies. */
  timestamped: boolean;
  /** Takes a public key instead of a shared secret. */
  publicKey?: boolean;
}

export const SCHEMES: Record<SignatureScheme, SchemeInfo> = {
  github: {
    name: 'GitHub',
    hint: 'X-Hub-Signature-256: HMAC-SHA256 of the body, as sha256=<hex>. Use the webhook secret.',
    timestamped: false,
  },
  stripe: {
    name: 'Stripe',
    hint: 'Stripe-Signature: HMAC-SHA256 of <t>.<body>. Use the endpoint signing secret (whsec_…).',
    timestamped: true,
  },
  standard: {
    name: 'Standard Webhooks / Svix',
    hint: 'webhook-signature or svix-signature: HMAC-SHA256 of <id>.<timestamp>.<body>. The secret starts with whsec_.',
    timestamped: true,
  },
  shopify: {
    name: 'Shopify',
    hint: 'X-Shopify-Hmac-Sha256: base64 HMAC-SHA256 of the body. Use the app client secret.',
    timestamped: false,
  },
  slack: {
    name: 'Slack',
    hint: 'X-Slack-Signature: HMAC-SHA256 of v0:<timestamp>:<body>. Use the app signing secret.',
    timestamped: true,
  },
  malga: {
    name: 'Malga',
    hint: 'X-Plug-Signature: Ed25519 signature of X-Plug-Date, a line break and the body. Use the public key you got when you created the webhook.',
    timestamped: true,
    publicKey: true,
  },
  hmac: {
    name: 'Custom HMAC',
    hint: 'Any header that carries an HMAC of the body.',
    timestamped: false,
  },
};

export const SIGNATURE_STATUS: Record<SignatureStatus, { label: string; tone: 'ok' | 'bad' | 'warn' }> = {
  valid: { label: 'Signature valid', tone: 'ok' },
  invalid: { label: 'Signature invalid', tone: 'bad' },
  expired: { label: 'Signature expired', tone: 'bad' },
  missing: { label: 'No signature', tone: 'warn' },
  unverifiable: { label: 'Signature not checked', tone: 'warn' },
};
