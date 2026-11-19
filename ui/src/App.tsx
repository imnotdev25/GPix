import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import * as api from "./api";
import type { Me } from "./types";
import Layout from "./components/Layout";
import Landing from "./pages/Landing";
import Login from "./pages/Login";
import Library from "./pages/Library";
import Upload from "./pages/Upload";
import Shares from "./pages/Shares";
import Import from "./pages/Import";
import Connections from "./pages/Connections";
import Tokens from "./pages/Tokens";
import Accounts from "./pages/Accounts";
import Albums, { AlbumPage, TagPage } from "./pages/Albums";
import SharePublic from "./pages/SharePublic";

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const location = useLocation();
  const navigate = useNavigate();

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

  // Share links open for anyone, signed in or not, without the app around them.
  const shareToken = location.pathname.match(/^\/s\/([^/]+)\/?$/)?.[1];
  if (shareToken) return <SharePublic token={decodeURIComponent(shareToken)} />;

  if (!me) {
    return (
      <div className="center">
        <div className="spinner" />
      </div>
    );
  }

  // Signed out: the landing page at "/", sign-in at /signin, and any app page
  // sends you to sign in first, then back to where you were going.
  if (!me.signedIn) {
    return (
      <Routes>
        <Route path="/" element={<Landing setup={!!me.firstRun && !!me.signupOpen} />} />
        <Route path="/signin" element={<Login me={me} onSignedIn={refresh} />} />
        <Route path="*" element={<Navigate to="/signin" replace state={{ from: location.pathname }} />} />
      </Routes>
    );
  }

  const from = (location.state as { from?: string } | null)?.from;
  const back = from && from.startsWith("/") && !from.startsWith("//") && from !== "/signin" ? from : "/library";

  return (
    <Layout
      me={me}
      onSignedOut={() => {
        setMe({ ...me, user: "", signedIn: false });
        navigate("/", { replace: true });
        refresh(); // picks up whether sign-up is open now
      }}
    >
      <Routes>
        <Route path="/" element={<Library />} />
        <Route path="/library" element={<Library />} />
        <Route path="/albums" element={<Albums />} />
        <Route path="/albums/:id" element={<AlbumPage />} />
        <Route path="/tags/:tag" element={<TagPage />} />
        <Route path="/upload" element={<Upload />} />
        <Route path="/shares" element={<Shares />} />
        <Route path="/import" element={<Import />} />
        <Route path="/connections" element={<Connections />} />
        <Route path="/tokens" element={<Tokens />} />
        <Route path="/accounts" element={<Accounts />} />
        <Route path="/signin" element={<Navigate to={back} replace />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  );
}
