import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import * as api from "./api";
import type { Me } from "./types";
import Layout from "./components/Layout";
import Login from "./pages/Login";
import Library from "./pages/Library";
import Upload from "./pages/Upload";
import Shares from "./pages/Shares";
import Import from "./pages/Import";
import Connections from "./pages/Connections";
import Tokens from "./pages/Tokens";
import Accounts from "./pages/Accounts";

export default function App() {
  const [me, setMe] = useState<Me | null>(null);

  const refresh = () =>
    api
      .get<Me>("/api/v1/auth/me")
      .then(setMe)
      .catch(() =>
        setMe({ user: "", signedIn: false, passwordLogin: true, logtoEnabled: false }),
      );

  useEffect(() => {
    refresh();
  }, []);

  if (!me) {
    return (
      <div className="center">
        <div className="spinner" />
      </div>
    );
  }

  if (!me.signedIn) {
    return <Login me={me} onSignedIn={refresh} />;
  }

  return (
    <Layout me={me} onSignedOut={refresh}>
      <Routes>
        <Route path="/" element={<Library />} />
        <Route path="/library" element={<Library />} />
        <Route path="/upload" element={<Upload />} />
        <Route path="/shares" element={<Shares />} />
        <Route path="/import" element={<Import />} />
        <Route path="/connections" element={<Connections />} />
        <Route path="/tokens" element={<Tokens />} />
        <Route path="/accounts" element={<Accounts />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  );
}
