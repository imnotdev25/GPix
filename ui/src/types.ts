export interface MediaItem {
  key: string;
  name: string;
  class: "photo" | "video" | "doc" | "file";
  encrypted: boolean;
  size: number;
  mtime: string;
  thumb: string;
  original: string;
  thumbPending?: boolean;
  tags?: string[];
}

export interface MediaPage {
  items: MediaItem[] | null;
  nextCursor: string;
  prevCursor?: string;
  offset?: number;
  total?: number;
}

export interface Me {
  user: string;
  signedIn: boolean;
  passwordLogin: boolean;
  logtoEnabled: boolean;
  /** A new account can be created now (GPIX_MAX_USERS not reached). */
  signupOpen?: boolean;
  /** No account exists yet: the first sign-up owns the server. */
  firstRun?: boolean;
}

export interface Account {
  id: number;
  label: string;
  addedAt: string;
  active: boolean;
}

export interface Share {
  token: string;
  title: string;
  url: string;
  hasPassword: boolean;
  expiresAt?: string;
  maxDownloads: number;
  downloads: number;
  allowOriginal: boolean;
  createdAt: string;
  count: number;
  /** Starts the download directly (a .zip for several items). */
  directUrl?: string;
}

export interface ImportProgress {
  running: boolean;
  server: string;
  workers: number;
  total: number;
  done: number;
  skipped: number;
  failed: number;
  current: string;
  error: string;
}

export interface ApiToken {
  id: number;
  label: string;
  preview: string;
  createdAt: string;
  lastUsed: string;
}

export interface Gateways {
  S3Enabled: boolean;
  S3Endpoint: string;
  S3Region: string;
  S3Bucket: string;
  S3AccessKey: string;
  S3SecretKey: string;
  HasS3Keys: boolean;
  WebDAVEnabled: boolean;
  WebDAVEndpoint: string;
  WebDAVUsername: string;
  WebDAVPassword: string;
  HasWebDAVPass: boolean;
  EncryptionAvailable: boolean;
  EncryptionEnabled: boolean;
  EncryptionFingerprint: string;
  EncryptionKeyCount: number;
  EncryptionKeys: { Fingerprint: string; Active: boolean }[] | null;
  JustGenerated: string;
  Notice: string;
}

/** What "Delete gpix uploads" would remove (GET /api/v1/gpix-uploads/summary). */
export interface GpixUploadSummary {
  accountId: number;
  accountLabel: string;
  total: number;
  photos: number;
  videos: number;
  docs: number;
  files: number;
  bytes: number;
  tracked: number;
  disguised: number;
  trackingEnabled: boolean;
  trackingSince: string;
  sample: string[];
}

/** A gpix album (stored by gpix, not synced with Google Photos). */
export interface Album {
  id: number;
  name: string;
  count: number;
  cover?: MediaItem;
  createdAt: string;
}

export interface TagSummary {
  tag: string;
  count: number;
}

export interface TimelineMonth {
  year: number;
  month: number; // 1-12
  count: number;
}
