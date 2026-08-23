export interface MediaItem {
  key: string;
  name: string;
  class: "photo" | "video" | "doc" | "file";
  encrypted: boolean;
  size: number;
  mtime: string;
  thumb: string;
  original: string;
}

export interface MediaPage {
  items: MediaItem[] | null;
  nextCursor: string;
}

export interface Me {
  user: string;
  signedIn: boolean;
  passwordLogin: boolean;
  logtoEnabled: boolean;
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
