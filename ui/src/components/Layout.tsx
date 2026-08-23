import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import * as api from "../api";
import type { Me } from "../types";

const links = [
  { to: "/library", label: "Library" },
  { to: "/upload", label: "Upload" },
  { to: "/shares", label: "Shares" },
  { to: "/import", label: "Import" },
  { to: "/accounts", label: "Accounts" },
  { to: "/connections", label: "Connections" },
  { to: "/tokens", label: "App tokens" },
];

export default function Layout({
  me,
  onSignedOut,
  children,
}: {
  me: Me;
  onSignedOut: () => void;
  children: ReactNode;
}) {
  const signOut = async () => {
    await api.post("/api/v1/auth/logout").catch(() => {});
    onSignedOut();
  };

  return (
    <div className="shell">
      <nav className="sidebar">
        <div className="brand">
          <span className="brand-mark">g</span>
          <div>
            <strong>gpix</strong>
            <div className="muted" style={{ fontSize: 11 }}>
              Media server
            </div>
          </div>
        </div>
        {links.map((l) => (
          <NavLink
            key={l.to}
            to={l.to}
            className={({ isActive }) => "nav-link" + (isActive ? " active" : "")}
          >
            {l.label}
          </NavLink>
        ))}
        <div className="sidebar-foot">
          <button className="btn" style={{ width: "100%" }} onClick={signOut}>
            Sign out{me.user ? ` · ${me.user}` : ""}
          </button>
        </div>
      </nav>
      <main className="main">{children}</main>
    </div>
  );
}
