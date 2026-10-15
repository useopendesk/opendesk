import type { IconName } from '../../lib/icons';

/** One address on the contact page, and the draft it starts. */
export interface ContactChannel {
  id: string;
  icon: IconName;
  title: string;
  body: string;
  email: string;
  subject: string;
  hint: string;
  note?: string;
}
