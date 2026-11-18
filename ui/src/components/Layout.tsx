import { useState } from "react";
import type { ReactNode } from "react";
import { NavLink, useLocation } from "react-router-dom";
import * as api from "../api";
import type { Me } from "../types";
import Icon from "./Icon";
import type { IconName } from "./Icon";
import { Modal } from "./feedback";
import { getThemePref, setThemePref } from "../theme";
import type { ThemePref } from "../theme";

type NavItem = { to: string; label: string; icon: IconName };

const primary: NavItem[] = [
  { to: "/library", label: "Library", icon: "photos" },
  { to: "/albums", label: "Albums", icon: "album" },
  { to: "/upload", label: "Upload", icon: "upload" },
  { to: "/shares", label: "Shares", icon: "share" },
  { to: "/import", label: "Import", icon: "import" },
];

const settings: NavItem[] = [
  { to: "/accounts", label: "Accounts", icon: "accounts" },
  { to: "/connections", label: "Connections", icon: "plug" },
  { to: "/tokens", label: "App tokens", icon: "key" },
];

const themes: { value: ThemePref; label: string; icon: IconName }[] = [
  { value: "system", label: "System", icon: "monitor" },
  { value: "light", label: "Light", icon: "sun" },
  { value: "dark", label: "Dark", icon: "moon" },
];

function ThemeSwitch({ labels = false }: { labels?: boolean }) {
  const [pref, setPref] = useState<ThemePref>(getThemePref);
  return (
    <div className={"segmented" + (labels ? " full" : "")} role="radiogroup" aria-label="Theme">
      {themes.map((t) => (
        <button
          key={t.value}
          type="button"
          role="radio"
          aria-checked={pref === t.value}
          aria-label={`${t.label} theme`}
          title={`${t.label} theme`}
          className={pref === t.value ? "on" : ""}
          onClick={() => {
            setThemePref(t.value);
            setPref(t.value);
          }}
        >
          <Icon name={t.icon} size={15} />
          {labels && <span>{t.label}</span>}
        </button>
      ))}
    </div>
  );
}

export default function Layout({
  me,
  onSignedOut,
  children,
}: {
  me: Me;
  onSignedOut: () => void;
  children: ReactNode;
}) {
  const [more, setMore] = useState(false);
  const { pathname } = useLocation();
  // "/" also shows the library.
  // Album and tag pages count as Albums.
  const isActive = (to: string) =>
    pathname === to ||
    (to === "/library" && pathname === "/") ||
    (to === "/albums" && (pathname.startsWith("/albums/") || pathname.startsWith("/tags/")));
  // The phone tab bar has room for four; the rest go in the More sheet.
  const tabs = primary.slice(0, 4);
  const overflow = [...primary.slice(4), ...settings];

  const signOut = async () => {
    await api.post("/api/v1/auth/logout").catch(() => {});
    onSignedOut();
  };

  const navLink = (l: NavItem) => (
    <NavLink key={l.to} to={l.to} className={"nav-link" + (isActive(l.to) ? " active" : "")}>
      <Icon name={l.icon} />
      <span>{l.label}</span>
    </NavLink>
  );
  const signOutLabel = "Sign out" + (me.user ? ` · ${me.user}` : "");

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Main">
        <div className="brand">
          <img className="brand-logo" src="/gpix.svg" alt="" />
          <div>
            <strong>gpix</strong>
            <div className="muted small">Media server</div>
          </div>
        </div>
        {primary.map(navLink)}
        <div className="nav-heading">Settings</div>
        {settings.map(navLink)}
        <div className="sidebar-foot">
          <ThemeSwitch />
          <button className="nav-link" onClick={signOut} title={signOutLabel}>
            <Icon name="logout" />
            <span>{signOutLabel}</span>
          </button>
        </div>
      </nav>

      <header className="topbar">
        <img className="brand-logo" src="/gpix.svg" alt="" />
        <strong>gpix</strong>
      </header>

      <main className="main">{children}</main>

      <nav className="tabbar" aria-label="Main">
        {tabs.map((l) => (
          <NavLink key={l.to} to={l.to} className={"tab" + (isActive(l.to) ? " active" : "")}>
            <Icon name={l.icon} size={20} />
            <span>{l.label}</span>
          </NavLink>
        ))}
        <button
          className={"tab" + (overflow.some((s) => isActive(s.to)) ? " active" : "")}
          onClick={() => setMore(true)}
        >
          <Icon name="menu" size={20} />
          <span>More</span>
        </button>
      </nav>

      {more && (
        <Modal title="More" sheet onClose={() => setMore(false)}>
          <div className="more-list" onClick={() => setMore(false)}>
            {overflow.map(navLink)}
          </div>
          <div className="field">
            <span>Theme</span>
            <ThemeSwitch labels />
          </div>
          <button className="btn wide" onClick={signOut}>
            <Icon name="logout" size={16} />
            {signOutLabel}
          </button>
        </Modal>
      )}
    </div>
  );
}
