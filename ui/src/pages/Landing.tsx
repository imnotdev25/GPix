import { Link } from "react-router-dom";
import Icon from "../components/Icon";
import type { IconName } from "../components/Icon";

const features: { icon: IconName; title: string; text: string }[] = [
  {
    icon: "photos",
    title: "Photo library",
    text: "A fast grid grouped by day, a full-screen viewer with zoom and swipe, video streaming, and bulk select to share, download or delete.",
  },
  {
    icon: "album",
    title: "Albums, tags and a timeline",
    text: "Photos are dated from their camera metadata. Jump through years and months with the scrubber, and sort things into albums and tags.",
  },
  {
    icon: "upload",
    title: "Upload anything",
    text: "Drag in photos, videos or any other file. Documents and archives are stored too, and come back byte-for-byte when you download them.",
  },
  {
    icon: "lock",
    title: "Optional encryption",
    text: "Encrypt every upload with a key only you hold. Google stores an unreadable clip; gpix still shows you the photo.",
  },
  {
    icon: "share",
    title: "Share links",
    text: "Public links to one item or a whole gallery, with an optional password, expiry date and download limit.",
  },
  {
    icon: "plug",
    title: "S3 and WebDAV",
    text: "Point rclone, aws, Finder or Windows Explorer at your library. Keys and app passwords are managed in the web UI.",
  },
  {
    icon: "monitor",
    title: "Desktop and phone apps",
    text: "Apps for macOS, Windows, Linux, Android and iOS, including photo backup from your phone.",
  },
  {
    icon: "import",
    title: "Import from Immich",
    text: "Copy an existing Immich library in with live progress. Runs resume where they left off and skip duplicates.",
  },
  {
    icon: "accounts",
    title: "Accounts and sign-in",
    text: "Link several Google accounts and switch between them. Sign in with a password or single sign-on (Logto / OIDC).",
  },
];

/** Public page shown to signed-out visitors at "/". */
export default function Landing({ setup = false }: { setup?: boolean }) {
  // On a fresh server the first visitor creates the owner account.
  const cta = setup ? "Create your account" : "Sign in to your library";
  return (
    <div className="landing">
      <header className="landing-wrap landing-top">
        <img className="brand-logo" src="/gpix.svg" alt="" />
        <strong>gpix</strong>
        <div className="spacer" />
        <Link className="btn" to="/signin">
          {setup ? "Set up" : "Sign in"}
        </Link>
      </header>

      <main>
        <section className="landing-wrap landing-hero">
          <div className="landing-hero-text">
            <h1>Your Google Photos library, on your terms.</h1>
            <p className="landing-lede">
              gpix is a self-hosted client for Google Photos. Browse, upload, encrypt and share your
              library from the browser, the desktop and phone apps, or any S3 and WebDAV tool.
            </p>
            <div className="landing-cta">
              <Link className="btn btn-primary" to="/signin">
                {cta}
              </Link>
              <a className="text-link" href="#features">
                See what it does
              </a>
            </div>
          </div>
          <img className="landing-mark" src="/gpix.svg" alt="" />
        </section>

        <section className="landing-wrap landing-section" aria-labelledby="about-h">
          <h2 id="about-h">What is gpix?</h2>
          <div>
            <p>
              gpix is one program you run on your own computer or server. It connects to your Google
              Photos account the same way the Android app does, so your photos stay in your Google
              account while gpix gives you more ways to reach them.
            </p>
            <p>
              There's no third-party service in between: gpix, its keys and its settings live on
              hardware you control.
            </p>
          </div>
        </section>

        <section className="landing-wrap landing-section" id="features" aria-labelledby="features-h">
          <h2 id="features-h">Features</h2>
          <ul className="features">
            {features.map((f) => (
              <li key={f.title}>
                <h3>
                  <Icon name={f.icon} size={18} />
                  {f.title}
                </h3>
                <p>{f.text}</p>
              </li>
            ))}
          </ul>
        </section>
      </main>

      <footer className="landing-wrap landing-foot">
        <span>gpix · self-hosted Google Photos client</span>
        <div className="spacer" />
        <Link className="text-link" to="/signin">
          Sign in
        </Link>
      </footer>
    </div>
  );
}
